package domain

import (
	"time"
)

type PlaybackState string

const (
	PlaybackStatePlaying PlaybackState = "playing"
	PlaybackStatePaused  PlaybackState = "paused"
)

type RoomRole string

const (
	RoomRoleHost     RoomRole = "host"
	RoomRoleListener RoomRole = "listener"
)

type Room struct {
	ID                 string        `json:"id" gorm:"type:char(36);primaryKey"`
	RoomCode           string        `json:"room_code" gorm:"type:varchar(16);uniqueIndex;not null"`
	Name               string        `json:"name" gorm:"type:varchar(255);not null"`
	HostID             string        `json:"host_id" gorm:"type:char(36);not null"`
	CurrentTrackID     *string       `json:"current_track_id" gorm:"type:char(36)"`
	PlaybackState      PlaybackState `json:"playback_state" gorm:"type:enum('playing','paused');default:'paused'"`
	PlaybackPositionMS int           `json:"playback_position_ms" gorm:"type:int;default:0"`
	LastSyncTimestamp  time.Time     `json:"last_sync_timestamp" gorm:"type:timestamp;autoUpdateTime"`
	SessionStartedAt   *time.Time    `json:"session_started_at" gorm:"type:timestamp;default:null"`
	CreatedAt          time.Time     `json:"created_at" gorm:"type:timestamp;autoCreateTime"`

	// Associations
	Host         *User        `json:"host,omitempty" gorm:"foreignKey:HostID"`
	CurrentTrack *Track       `json:"current_track,omitempty" gorm:"foreignKey:CurrentTrackID"`
	Members      []RoomMember `json:"members,omitempty" gorm:"foreignKey:RoomID"`
	Tracks       []Track      `json:"tracks,omitempty" gorm:"foreignKey:RoomID"`
	RepeatMode   string       `json:"repeat_mode" gorm:"type:enum('off','all','one');default:'off'"`
	IsShuffled   bool         `json:"is_shuffled" gorm:"column:is_shuffled;type:tinyint(1);default:0"`
}

type RoomMember struct {
	RoomID   string    `json:"room_id" gorm:"type:char(36);primaryKey"`
	UserID   string    `json:"user_id" gorm:"type:char(36);primaryKey"`
	Role     RoomRole  `json:"role" gorm:"type:enum('host','listener');default:'listener'"`
	JoinedAt time.Time `json:"joined_at" gorm:"type:timestamp;autoCreateTime"`

	User *User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

type CreateRoomRequest struct {
	Name string `json:"name" binding:"required,min=2,max=100"`
}

type JoinRoomResponse struct {
	Room   *Room       `json:"room"`
	Member *RoomMember `json:"member"`
}

type SyncPlaybackRequest struct {
	PlaybackState      PlaybackState `json:"playback_state" binding:"required,oneof=playing paused"`
	PlaybackPositionMS *int          `json:"playback_position_ms" binding:"omitempty,min=0"`
	CurrentTrackID     *string       `json:"current_track_id,omitempty"`
}

// PlaybackSnapshot is the live, low-latency view of a room's playback
// state as held in Redis. It mirrors the playback-related fields on Room
// but is written/read independently of MySQL so hot-path updates (heartbeat,
// seek, play/pause) don't pay MySQL's write latency.
type PlaybackSnapshot struct {
	PlaybackState      PlaybackState
	PlaybackPositionMS int
	CurrentTrackID     *string
	UpdatedAt          time.Time
}

type RoomRepository interface {
	Create(room *Room) error
	GetByID(id string) (*Room, error)
	GetByCode(code string) (*Room, error)
	Update(room *Room) error
	UpdatePlaybackState(roomID string, state PlaybackState, positionMS *int, currentTrackID *string) error
	MarkSessionStarted(roomID string) (*time.Time, error)
	AddMember(member *RoomMember) error
	RemoveMember(roomID, userID string) error
	GetMembers(roomID string) ([]RoomMember, error)
	GetMember(roomID, userID string) (*RoomMember, error)
	IsCodeExists(code string) (bool, error)
	Delete(id string) error
	UpdatePlaybackSettings(roomID string, repeatMode string, isShuffled bool) error
	TransferHost(roomID, oldHostID, newHostID string) error
}

// PlaybackCache is the Redis-backed store for live playback state. It's
// intentionally separate from RoomRepository: this is fast, ephemeral state
// (safe to lose on restart — the room falls back to MySQL's last persisted
// value), not the durable source of truth.
type PlaybackCache interface {
	SetPlaybackState(roomID string, state PlaybackState, positionMS int, currentTrackID *string) error
	GetPlaybackState(roomID string) (*PlaybackSnapshot, error)
	DeletePlaybackState(roomID string) error
}

type RoomService interface {
	CreateRoom(userID string, req *CreateRoomRequest) (*Room, error)
	GetRoomByCode(code string) (*Room, error)
	GetRoomByID(roomID string) (*Room, error)
	JoinRoom(userID, roomCode string) (*JoinRoomResponse, error)
	LeaveRoom(userID, roomID string) error
	UpdatePlayback(userID, roomID string, req *SyncPlaybackRequest) (*Room, error)
	GetRoomMembers(roomID string) ([]RoomMember, error)
}