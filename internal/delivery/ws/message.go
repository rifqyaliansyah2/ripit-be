package ws

type EventType string

const (
	EventSyncPlayback  EventType = "SYNC_PLAYBACK"
	EventChangeState   EventType = "CHANGE_STATE"
	EventQueueUpdated  EventType = "QUEUE_UPDATED"
	EventUserJoined    EventType = "USER_JOINED"
	EventUserLeft      EventType = "USER_LEFT"
	EventInitialState  EventType = "INITIAL_STATE"
	EventError         EventType = "ERROR"
)

type WSMessage struct {
	Type    EventType   `json:"type"`
	Payload interface{} `json:"payload"`
}

type SyncPlaybackPayload struct {
	PlaybackState      string  `json:"playback_state"` // playing, paused
	PlaybackPositionMS int     `json:"playback_position_ms"`
	CurrentTrackID     *string `json:"current_track_id,omitempty"`
	Timestamp          int64   `json:"timestamp"` // client or server timestamp ms
}

type ChangeStatePayload struct {
	PlaybackState string `json:"playback_state"` // playing, paused
	PositionMS    *int   `json:"position_ms,omitempty"`
}

type UserPresencePayload struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar_url,omitempty"`
	Role     string `json:"role"`
}
