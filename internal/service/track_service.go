package service

import (
	"errors"
	"time"

	"ripit-be/internal/domain"

	"github.com/google/uuid"
)

type trackService struct {
	trackRepo domain.TrackRepository
	roomRepo  domain.RoomRepository
}

func NewTrackService(trackRepo domain.TrackRepository, roomRepo domain.RoomRepository) domain.TrackService {
	return &trackService{
		trackRepo: trackRepo,
		roomRepo:  roomRepo,
	}
}

func (s *trackService) AddTrack(userID, roomID string, req *domain.AddTrackRequest) (*domain.Track, error) {
	room, err := s.roomRepo.GetByID(roomID)
	if err != nil {
		return nil, err
	}
	if room == nil {
		return nil, errors.New("room not found")
	}

	maxOrder, err := s.trackRepo.GetMaxSortOrder(roomID)
	if err != nil {
		return nil, err
	}

	hasLyrics := req.Lyrics != nil && len(*req.Lyrics) > 0

	track := &domain.Track{
		ID:         uuid.New().String(),
		RoomID:     roomID,
		YouTubeURL: req.YouTubeURL,
		Title:      req.Title,
		Artist:     req.Artist,
		Duration:   req.Duration,
		CoverURL:   req.CoverURL,
		Lyrics:     req.Lyrics,
		HasLyrics:  hasLyrics,
		AddedBy:    &userID,
		SortOrder:  maxOrder + 1,
		CreatedAt:  time.Now(),
	}

	if err := s.trackRepo.Create(track); err != nil {
		return nil, err
	}

	// If room has no current track, set this track as current track automatically
	if room.CurrentTrackID == nil || *room.CurrentTrackID == "" {
		trackID := track.ID
		zeroPos := 0
		_ = s.roomRepo.UpdatePlaybackState(roomID, domain.PlaybackStatePaused, &zeroPos, &trackID)
	}

	return s.trackRepo.GetByID(track.ID)
}

func (s *trackService) GetTracksByRoomID(roomID string) ([]domain.Track, error) {
	return s.trackRepo.GetByRoomID(roomID)
}

func (s *trackService) GetTrackByID(trackID string) (*domain.Track, error) {
	track, err := s.trackRepo.GetByID(trackID)
	if err != nil {
		return nil, err
	}
	if track == nil {
		return nil, errors.New("track not found")
	}
	return track, nil
}

func (s *trackService) UpdateTrack(userID, roomID, trackID string, req *domain.UpdateTrackRequest) (*domain.Track, error) {
    track, err := s.trackRepo.GetByID(trackID)
    if err != nil {
        return nil, err
    }
    if track == nil || track.RoomID != roomID {
        return nil, errors.New("track not found in this room")
    }

    if req.Title != nil {
        track.Title = *req.Title
    }
    if req.Artist != nil {
        track.Artist = *req.Artist
    }
    if req.YouTubeURL != nil {
        track.YouTubeURL = *req.YouTubeURL
    }
    if req.CoverURL != nil {
        track.CoverURL = *req.CoverURL
    }
    if req.Duration != nil {
        track.Duration = *req.Duration
    }
    if req.Lyrics != nil {
        track.Lyrics = req.Lyrics
        track.HasLyrics = len(*req.Lyrics) > 0
    }
    if req.HasLyrics != nil {
        track.HasLyrics = *req.HasLyrics
    }
    if req.SortOrder != nil {
        track.SortOrder = *req.SortOrder
    }

    if err := s.trackRepo.Update(track); err != nil {
        return nil, err
    }

    return s.trackRepo.GetByID(track.ID)
}

func (s *trackService) DeleteTrack(userID, roomID, trackID string) error {
	track, err := s.trackRepo.GetByID(trackID)
	if err != nil {
		return err
	}
	if track == nil || track.RoomID != roomID {
		return errors.New("track not found in this room")
	}

	// If room's current_track_id is this track, advance or clear it
	room, err := s.roomRepo.GetByID(roomID)
	if err == nil && room != nil && room.CurrentTrackID != nil && *room.CurrentTrackID == trackID {
		tracks, _ := s.trackRepo.GetByRoomID(roomID)
		var nextTrackID *string
		for _, t := range tracks {
			if t.ID != trackID {
				nextID := t.ID
				nextTrackID = &nextID
				break
			}
		}
		zeroPos := 0
		_ = s.roomRepo.UpdatePlaybackState(roomID, domain.PlaybackStatePaused, &zeroPos, nextTrackID)
	}

	return s.trackRepo.Delete(trackID)
}

func (s *trackService) ReorderTracks(userID, roomID string, req *domain.ReorderTracksRequest) ([]domain.Track, error) {
	orderMap := make(map[string]int)
	for idx, trackID := range req.TrackIDs {
		orderMap[trackID] = idx
	}

	if err := s.trackRepo.UpdateSortOrders(roomID, orderMap); err != nil {
		return nil, err
	}

	return s.trackRepo.GetByRoomID(roomID)
}
