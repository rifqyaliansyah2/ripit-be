package domain

import (
	"time"
)

type Track struct {
	ID         string    `json:"id" gorm:"type:char(36);primaryKey"`
	RoomID     string    `json:"room_id" gorm:"type:char(36);not null;index"`
	YouTubeURL string    `json:"youtube_url" gorm:"type:varchar(512);not null"`
	Title      string    `json:"title" gorm:"type:varchar(512);not null"`
	Artist     string    `json:"artist" gorm:"type:varchar(255);not null"`
	Duration   string    `json:"duration" gorm:"type:varchar(64);default:'0:00'"`
	CoverURL   string    `json:"cover_url" gorm:"type:varchar(512);default:''"`
	Lyrics     *string   `json:"lyrics,omitempty" gorm:"type:longtext"`
	HasLyrics  bool      `json:"has_lyrics" gorm:"type:tinyint(1);default:0"`
	AddedBy    string    `json:"added_by" gorm:"type:char(36);not null;index"`
	SortOrder  int       `json:"sort_order" gorm:"type:int;default:0"`
	CreatedAt  time.Time `json:"created_at" gorm:"type:timestamp;autoCreateTime"`

	User *User `json:"user,omitempty" gorm:"foreignKey:AddedBy"`
}

type AddTrackRequest struct {
	YouTubeURL string  `json:"youtube_url" binding:"required,url"`
	Title      string  `json:"title" binding:"required"`
	Artist     string  `json:"artist" binding:"required"`
	Duration   string  `json:"duration" binding:"required"`
	CoverURL   string  `json:"cover_url" binding:"omitempty"`
	Lyrics     *string `json:"lyrics,omitempty"`
}

type UpdateTrackRequest struct {
	Title     *string `json:"title,omitempty"`
	Artist    *string `json:"artist,omitempty"`
	Lyrics    *string `json:"lyrics,omitempty"`
	HasLyrics *bool   `json:"has_lyrics,omitempty"`
	SortOrder *int    `json:"sort_order,omitempty"`
}

type ReorderTracksRequest struct {
	TrackIDs []string `json:"track_ids" binding:"required,min=1"`
}

type TrackRepository interface {
	Create(track *Track) error
	GetByID(id string) (*Track, error)
	GetByRoomID(roomID string) ([]Track, error)
	Update(track *Track) error
	Delete(id string) error
	GetMaxSortOrder(roomID string) (int, error)
	UpdateSortOrders(roomID string, trackOrderMap map[string]int) error
}

type TrackService interface {
	AddTrack(userID, roomID string, req *AddTrackRequest) (*Track, error)
	GetTracksByRoomID(roomID string) ([]Track, error)
	GetTrackByID(trackID string) (*Track, error)
	UpdateTrack(userID, roomID, trackID string, req *UpdateTrackRequest) (*Track, error)
	DeleteTrack(userID, roomID, trackID string) error
	ReorderTracks(userID, roomID string, req *ReorderTracksRequest) ([]Track, error)
}
