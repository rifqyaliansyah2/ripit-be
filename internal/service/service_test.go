package service_test

import (
	"testing"
	"time"

	"ripit-be/internal/domain"
	"ripit-be/internal/service"
	"ripit-be/pkg/jwt"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// In-memory mock repositories for testing
type mockUserRepo struct {
	users map[string]*domain.User
}

func (m *mockUserRepo) Create(u *domain.User) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepo) GetByID(id string) (*domain.User, error) {
	return m.users[id], nil
}

func (m *mockUserRepo) GetByUsername(name string) (*domain.User, error) {
	for _, u := range m.users {
		if u.Username == name {
			return u, nil
		}
	}
	return nil, nil
}

type mockRoomRepo struct {
	rooms   map[string]*domain.Room
	members map[string][]domain.RoomMember
}

func (m *mockRoomRepo) Create(r *domain.Room) error {
	m.rooms[r.ID] = r
	return nil
}

func (m *mockRoomRepo) GetByID(id string) (*domain.Room, error) {
	return m.rooms[id], nil
}

func (m *mockRoomRepo) GetByCode(code string) (*domain.Room, error) {
	for _, r := range m.rooms {
		if r.RoomCode == code {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockRoomRepo) Update(r *domain.Room) error {
	m.rooms[r.ID] = r
	return nil
}

func (m *mockRoomRepo) UpdatePlaybackState(roomID string, state domain.PlaybackState, pos int, trackID *string) error {
	if r, ok := m.rooms[roomID]; ok {
		r.PlaybackState = state
		r.PlaybackPositionMS = pos
		if trackID != nil {
			r.CurrentTrackID = trackID
		}
	}
	return nil
}

func (m *mockRoomRepo) AddMember(member *domain.RoomMember) error {
	m.members[member.RoomID] = append(m.members[member.RoomID], *member)
	return nil
}

func (m *mockRoomRepo) RemoveMember(roomID, userID string) error {
	return nil
}

func (m *mockRoomRepo) GetMembers(roomID string) ([]domain.RoomMember, error) {
	return m.members[roomID], nil
}

func (m *mockRoomRepo) GetMember(roomID, userID string) (*domain.RoomMember, error) {
	for _, member := range m.members[roomID] {
		if member.UserID == userID {
			return &member, nil
		}
	}
	return nil, nil
}

func (m *mockRoomRepo) IsCodeExists(code string) (bool, error) {
	for _, r := range m.rooms {
		if r.RoomCode == code {
			return true, nil
		}
	}
	return false, nil
}

type mockTrackRepo struct {
	tracks map[string]*domain.Track
}

func (m *mockTrackRepo) Create(t *domain.Track) error {
	m.tracks[t.ID] = t
	return nil
}

func (m *mockTrackRepo) GetByID(id string) (*domain.Track, error) {
	return m.tracks[id], nil
}

func (m *mockTrackRepo) GetByRoomID(roomID string) ([]domain.Track, error) {
	var list []domain.Track
	for _, t := range m.tracks {
		if t.RoomID == roomID {
			list = append(list, *t)
		}
	}
	return list, nil
}

func (m *mockTrackRepo) Update(t *domain.Track) error {
	m.tracks[t.ID] = t
	return nil
}

func (m *mockTrackRepo) Delete(id string) error {
	delete(m.tracks, id)
	return nil
}

func (m *mockTrackRepo) GetMaxSortOrder(roomID string) (int, error) {
	max := -1
	for _, t := range m.tracks {
		if t.RoomID == roomID && t.SortOrder > max {
			max = t.SortOrder
		}
	}
	return max, nil
}

func (m *mockTrackRepo) UpdateSortOrders(roomID string, orderMap map[string]int) error {
	for id, order := range orderMap {
		if t, ok := m.tracks[id]; ok {
			t.SortOrder = order
		}
	}
	return nil
}

func TestUserService_RegisterGuest(t *testing.T) {
	userRepo := &mockUserRepo{users: make(map[string]*domain.User)}
	tokenMaker := jwt.NewTokenMaker("test_secret_key_1234567890", 24)
	svc := service.NewUserService(userRepo, tokenMaker)

	req := &domain.CreateUserRequest{
		Username: "alice",
	}

	resp, err := svc.RegisterGuest(req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "alice", resp.User.Username)
	assert.NotEmpty(t, resp.Token)

	claims, err := tokenMaker.ValidateToken(resp.Token)
	assert.NoError(t, err)
	assert.Equal(t, resp.User.ID, claims.UserID)
}

func TestRoomService_CreateAndJoinRoom(t *testing.T) {
	userRepo := &mockUserRepo{users: map[string]*domain.User{
		"u1": {ID: "u1", Username: "host_user", CreatedAt: time.Now()},
		"u2": {ID: "u2", Username: "listener_user", CreatedAt: time.Now()},
	}}
	roomRepo := &mockRoomRepo{rooms: make(map[string]*domain.Room), members: make(map[string][]domain.RoomMember)}
	trackRepo := &mockTrackRepo{tracks: make(map[string]*domain.Track)}

	svc := service.NewRoomService(roomRepo, userRepo, trackRepo)

	// Create Room
	room, err := svc.CreateRoom("u1", &domain.CreateRoomRequest{Name: "Party Room"})
	assert.NoError(t, err)
	assert.NotNil(t, room)
	assert.Equal(t, "Party Room", room.Name)
	assert.Len(t, room.RoomCode, 6)

	// Join Room with listener
	joinRes, err := svc.JoinRoom("u2", room.RoomCode)
	assert.NoError(t, err)
	assert.NotNil(t, joinRes)
	assert.Equal(t, domain.RoomRoleListener, joinRes.Member.Role)
}

func TestTrackService_AddAndReorderTracks(t *testing.T) {
	roomRepo := &mockRoomRepo{
		rooms: map[string]*domain.Room{
			"room-1": {ID: "room-1", Name: "Test Room", HostID: "u1"},
		},
		members: make(map[string][]domain.RoomMember),
	}
	trackRepo := &mockTrackRepo{tracks: make(map[string]*domain.Track)}
	svc := service.NewTrackService(trackRepo, roomRepo)

	// Add track 1
	t1, err := svc.AddTrack("u1", "room-1", &domain.AddTrackRequest{
		YouTubeURL: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Title:      "Never Gonna Give You Up",
		Artist:     "Rick Astley",
		Duration:   "3:33",
	})
	assert.NoError(t, err)
	assert.NotNil(t, t1)
	assert.Equal(t, 0, t1.SortOrder)

	// Add track 2
	t2, err := svc.AddTrack("u1", "room-1", &domain.AddTrackRequest{
		YouTubeURL: "https://www.youtube.com/watch?v=kffacxfA7G4",
		Title:      "Baby Shark",
		Artist:     "Pinkfong",
		Duration:   "2:16",
	})
	assert.NoError(t, err)
	assert.NotNil(t, t2)
	assert.Equal(t, 1, t2.SortOrder)

	// Reorder
	tracks, err := svc.ReorderTracks("u1", "room-1", &domain.ReorderTracksRequest{
		TrackIDs: []string{t2.ID, t1.ID},
	})
	assert.NoError(t, err)
	assert.Len(t, tracks, 2)
}

func TestUUIDFormat(t *testing.T) {
	id := uuid.New().String()
	_, err := uuid.Parse(id)
	assert.NoError(t, err)
}
