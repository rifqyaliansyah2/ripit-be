package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	"ripit-be/internal/domain"

	"github.com/google/uuid"
)

type roomService struct {
	roomRepo      domain.RoomRepository
	userRepo      domain.UserRepository
	trackRepo     domain.TrackRepository
	playbackCache domain.PlaybackCache
}

func NewRoomService(roomRepo domain.RoomRepository, userRepo domain.UserRepository, trackRepo domain.TrackRepository, playbackCache domain.PlaybackCache) domain.RoomService {
	return &roomService{
		roomRepo:      roomRepo,
		userRepo:      userRepo,
		trackRepo:     trackRepo,
		playbackCache: playbackCache,
	}
}

func (s *roomService) generateRoomCode() (string, error) {
	const charset = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // Exclude confusing chars like 0/O, 1/I
	for attempts := 0; attempts < 10; attempts++ {
		b := make([]byte, 6)
		for i := range b {
			idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
			if err != nil {
				return "", err
			}
			b[i] = charset[idx.Int64()]
		}
		code := string(b)
		exists, err := s.roomRepo.IsCodeExists(code)
		if err != nil {
			return "", err
		}
		if !exists {
			return code, nil
		}
	}
	return "", errors.New("unable to generate unique room code")
}

func (s *roomService) CreateRoom(userID string, req *domain.CreateRoomRequest) (*domain.Room, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	roomCode, err := s.generateRoomCode()
	if err != nil {
		return nil, err
	}

	room := &domain.Room{
		ID:                 uuid.New().String(),
		RoomCode:           roomCode,
		Name:               req.Name,
		HostID:             userID,
		PlaybackState:      domain.PlaybackStatePaused,
		PlaybackPositionMS: 0,
		LastSyncTimestamp:  time.Now(),
		CreatedAt:          time.Now(),
	}

	if err := s.roomRepo.Create(room); err != nil {
		return nil, err
	}

	// Add host as member
	member := &domain.RoomMember{
		RoomID:   room.ID,
		UserID:   userID,
		Role:     domain.RoomRoleHost,
		JoinedAt: time.Now(),
	}
	if err := s.roomRepo.AddMember(member); err != nil {
		return nil, err
	}

	return s.roomRepo.GetByID(room.ID)
}

func (s *roomService) GetRoomByCode(code string) (*domain.Room, error) {
	cleanCode := strings.ToUpper(strings.TrimSpace(code))
	room, err := s.roomRepo.GetByCode(cleanCode)
	if err != nil {
		return nil, err
	}
	if room == nil {
		return nil, errors.New("room not found")
	}
	s.overlayPlaybackCache(room)
	return room, nil
}

func (s *roomService) GetRoomByID(roomID string) (*domain.Room, error) {
	room, err := s.roomRepo.GetByID(roomID)
	if err != nil {
		return nil, err
	}
	if room == nil {
		return nil, errors.New("room not found")
	}
	s.overlayPlaybackCache(room)
	return room, nil
}

// overlayPlaybackCache patches a freshly-loaded (MySQL) room with the live
// playback state from Redis, if any is cached. This matters because
// UpdatePlayback returns immediately after writing Redis and only persists
// to MySQL in the background, so MySQL's copy can briefly lag behind. If
// Redis has nothing (cache expired, or never written), the room just keeps
// whatever MySQL had — no error, no crash.
func (s *roomService) overlayPlaybackCache(room *domain.Room) {
	snapshot, err := s.playbackCache.GetPlaybackState(room.ID)
	if err != nil || snapshot == nil {
		return
	}
	room.PlaybackState = snapshot.PlaybackState
	room.PlaybackPositionMS = snapshot.PlaybackPositionMS
	if snapshot.CurrentTrackID != nil {
		room.CurrentTrackID = snapshot.CurrentTrackID
	}
}

func (s *roomService) JoinRoom(userID, roomCode string) (*JoinRoomResponse, error) {
	user, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New("user not found")
	}

	room, err := s.GetRoomByCode(roomCode)
	if err != nil {
		return nil, err
	}

	role := domain.RoomRoleListener
	if room.HostID == userID {
		role = domain.RoomRoleHost
	}

	member := &domain.RoomMember{
		RoomID:   room.ID,
		UserID:   userID,
		Role:     role,
		JoinedAt: time.Now(),
	}

	if err := s.roomRepo.AddMember(member); err != nil {
		return nil, err
	}

	member.User = user

	return &JoinRoomResponse{
		Room:   room,
		Member: member,
	}, nil
}

func (s *roomService) LeaveRoom(userID, roomID string) error {
	return s.roomRepo.RemoveMember(roomID, userID)
}

func (s *roomService) UpdatePlayback(userID, roomID string, req *domain.SyncPlaybackRequest) (*domain.Room, error) {
	room, err := s.roomRepo.GetByID(roomID)
	if err != nil {
		return nil, err
	}
	if room == nil {
		return nil, errors.New("room not found")
	}

	if room.HostID != userID {
		return nil, errors.New("only room host can update playback state")
	}

	if req.CurrentTrackID != nil && *req.CurrentTrackID != "" {
		track, err := s.trackRepo.GetByID(*req.CurrentTrackID)
		if err != nil {
			return nil, err
		}
		if track == nil || track.RoomID != roomID {
			return nil, errors.New("current track not found in this room")
		}
	}

	positionMS := room.PlaybackPositionMS
	if req.PlaybackPositionMS != nil {
		positionMS = *req.PlaybackPositionMS
	}
	currentTrackID := room.CurrentTrackID
	if req.CurrentTrackID != nil {
		currentTrackID = req.CurrentTrackID
	}

	// Fast path: this is what the response and every listener's next read
	// actually sees. Redis write latency is low enough it doesn't matter
	// that we're on the request's hot path here.
	if err := s.playbackCache.SetPlaybackState(roomID, req.PlaybackState, positionMS, currentTrackID); err != nil {
		return nil, err
	}

	// Slow path: MySQL stays the durable copy (survives Redis restarts/
	// evictions), but writing it happens off the request path so a slow
	// query never delays the response or blocks the next heartbeat.
	go func() {
		_ = s.roomRepo.UpdatePlaybackState(roomID, req.PlaybackState, req.PlaybackPositionMS, req.CurrentTrackID)
	}()

	room.PlaybackState = req.PlaybackState
	room.PlaybackPositionMS = positionMS
	room.CurrentTrackID = currentTrackID
	room.LastSyncTimestamp = time.Now()

	return room, nil
}

func (s *roomService) GetRoomMembers(roomID string) ([]domain.RoomMember, error) {
	return s.roomRepo.GetMembers(roomID)
}

// Ensure alias type matching domain response
type JoinRoomResponse = domain.JoinRoomResponse