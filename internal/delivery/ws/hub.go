package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"ripit-be/internal/domain"
	"ripit-be/pkg/wsticket"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// memberReconnectGrace is how long we wait after any member's connection
// drops before treating it as a real "left the room" event. This absorbs
// page refreshes (disconnect immediately followed by a fresh reconnect)
// without flapping the member list or deleting their DB membership.
const memberReconnectGrace = 6 * time.Second

type HubManager struct {
	rooms          map[string]*RoomHub
	mu             sync.RWMutex
	roomRepo       domain.RoomRepository
	userRepo       domain.UserRepository
	trackRepo      domain.TrackRepository
	playbackCache  domain.PlaybackCache
	ticketStore    *wsticket.Store
	upgrader       websocket.Upgrader
	allowedOrigins map[string]bool
}

func NewHubManager(roomRepo domain.RoomRepository, userRepo domain.UserRepository, trackRepo domain.TrackRepository, playbackCache domain.PlaybackCache, ticketStore *wsticket.Store, allowedOrigins []string) *HubManager {
	originMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		originMap[o] = true
	}

	hm := &HubManager{
		rooms:          make(map[string]*RoomHub),
		roomRepo:       roomRepo,
		userRepo:       userRepo,
		trackRepo:      trackRepo,
		playbackCache:  playbackCache,
		ticketStore:    ticketStore,
		allowedOrigins: originMap,
	}

	hm.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			if len(hm.allowedOrigins) == 0 {
				return true
			}
			return hm.allowedOrigins[origin] || hm.allowedOrigins["*"]
		},
	}

	return hm
}

func (hm *HubManager) GetOrCreateHub(roomID string) *RoomHub {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	if hub, exists := hm.rooms[roomID]; exists {
		return hub
	}

	hub := NewRoomHub(roomID, hm.roomRepo, hm.trackRepo, hm.playbackCache, func() {
		hm.mu.Lock()
		delete(hm.rooms, roomID)
		hm.mu.Unlock()
	})
	hm.rooms[roomID] = hub
	go hub.Run()
	return hub
}

func (hm *HubManager) HandleWS(c *gin.Context) {
	roomID := c.Param("id")
	if roomID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "room ID is required"})
		return
	}

	ticket := c.Query("ticket")
	if ticket == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID, username, ok := hm.ticketStore.Consume(ticket)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired ticket"})
		return
	}
	if username == "" {
		username = "Guest"
	}

	room, err := hm.roomRepo.GetByID(roomID)
	if err != nil || room == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "room not found"})
		return
	}

	role := "listener"
	if room.HostID == userID {
		role = "host"
	}

	conn, err := hm.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[WS] Upgrade error: %v", err)
		return
	}

	hub := hm.GetOrCreateHub(roomID)
	client := &Client{
		Hub:      hub,
		Conn:     conn,
		Send:     make(chan []byte, 256),
		RoomID:   roomID,
		UserID:   userID,
		Username: username,
		Role:     role,
	}

	hub.Register <- client

	go client.WritePump()
	go client.ReadPump()
}

type pendingLeave struct {
	timer *time.Timer
	gen   int
}

type leaveTimeoutMsg struct {
	userID   string
	username string
	role     string
	gen      int
}

type RoomHub struct {
	RoomID     string
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	onDestroy  func()
	roomRepo   domain.RoomRepository
	trackRepo  domain.TrackRepository
	playbackCache domain.PlaybackCache
	mu         sync.RWMutex

	leaveTimeout    chan leaveTimeoutMsg
	pendingLeaves   map[string]*pendingLeave // keyed by UserID
	leaveGenCounter int
}

func NewRoomHub(roomID string, roomRepo domain.RoomRepository, trackRepo domain.TrackRepository, playbackCache domain.PlaybackCache, onDestroy func()) *RoomHub {
	return &RoomHub{
		RoomID:        roomID,
		Clients:       make(map[*Client]bool),
		Broadcast:     make(chan []byte),
		Register:      make(chan *Client),
		Unregister:    make(chan *Client),
		leaveTimeout:  make(chan leaveTimeoutMsg),
		pendingLeaves: make(map[string]*pendingLeave),
		onDestroy:     onDestroy,
		roomRepo:      roomRepo,
		trackRepo:     trackRepo,
		playbackCache: playbackCache,
	}
}

func (h *RoomHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.handleRegister(client)

		case client := <-h.Unregister:
			if h.handleUnregister(client) {
				return
			}

		case msg := <-h.leaveTimeout:
			if h.handleLeaveTimeout(msg) {
				return
			}

		case message := <-h.Broadcast:
			h.mu.RLock()
			for client := range h.Clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.Clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// handleRegister runs both for a brand-new join and for a reconnect (e.g.
// after a page refresh). Either way, membership is upserted to the DB so it
// never drifts out of sync with who is actually connected, and any pending
// "leave" grace timer for this user is cancelled.
func (h *RoomHub) handleRegister(client *Client) {
	h.mu.Lock()
	if pl, ok := h.pendingLeaves[client.UserID]; ok {
		pl.timer.Stop()
		delete(h.pendingLeaves, client.UserID)
	}
	h.Clients[client] = true
	h.mu.Unlock()

	_ = h.roomRepo.AddMember(&domain.RoomMember{
		RoomID:   h.RoomID,
		UserID:   client.UserID,
		Role:     domain.RoomRole(client.Role),
		JoinedAt: time.Now(),
	})

	h.sendInitialState(client)

	h.broadcastEvent(WSMessage{
		Type: EventUserJoined,
		Payload: UserPresencePayload{
			UserID:   client.UserID,
			Username: client.Username,
			Role:     client.Role,
		},
	})
}

// handleUnregister runs whenever a connection drops, for any reason —
// explicit "Leave Room", tab close, or a refresh. Returns true if the hub
// goroutine should stop (room was destroyed or emptied out).
func (h *RoomHub) handleUnregister(client *Client) bool {
	h.mu.Lock()
	_, existed := h.Clients[client]
	if !existed {
		h.mu.Unlock()
		return false
	}
	delete(h.Clients, client)
	close(client.Send)

	userID := client.UserID
	username := client.Username
	role := client.Role
	explicit := client.ExplicitLeave
	stillConnected := h.hasActiveConnectionLocked(userID)
	h.mu.Unlock()

	if stillConnected {
		return false
	}

	// Pause the room for everyone the moment anyone disconnects.
	// Exceptions:
	//   - Host explicit leave → destroyRoom() handles it, no point pausing first.
	//   - Room already in paused state → UpdatePlaybackState is idempotent so fine,
	//     but the extra broadcast is harmless.
	if role == "host" {
		h.pauseRoomOnDisconnect(userID, username, role)
	}

	if !explicit {
		h.scheduleLeaveGrace(userID, username, role)
		return false
	}

	return h.finalizeLeave(userID, username, role)
}

func (h *RoomHub) hasActiveConnectionLocked(userID string) bool {
	for c := range h.Clients {
		if c.UserID == userID {
			return true
		}
	}
	return false
}

func (h *RoomHub) scheduleLeaveGrace(userID, username, role string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if existing, ok := h.pendingLeaves[userID]; ok {
		existing.timer.Stop()
	}

	h.leaveGenCounter++
	gen := h.leaveGenCounter
	timer := time.AfterFunc(memberReconnectGrace, func() {
		h.leaveTimeout <- leaveTimeoutMsg{userID: userID, username: username, role: role, gen: gen}
	})
	h.pendingLeaves[userID] = &pendingLeave{timer: timer, gen: gen}
}

func (h *RoomHub) handleLeaveTimeout(msg leaveTimeoutMsg) bool {
	h.mu.Lock()
	pl, ok := h.pendingLeaves[msg.userID]
	if !ok || pl.gen != msg.gen {
		h.mu.Unlock()
		return false
	}
	delete(h.pendingLeaves, msg.userID)
	h.mu.Unlock()

	return h.finalizeLeave(msg.userID, msg.username, msg.role)
}

// finalizeLeave actually removes membership and notifies the room. Shared by
// the explicit-leave path and the grace-period-expired path.
func (h *RoomHub) finalizeLeave(userID, username, role string) bool {
	if role == "host" {
		h.destroyRoom()
		return true
	}

	_ = h.roomRepo.RemoveMember(h.RoomID, userID)

	h.broadcastEvent(WSMessage{
		Type: EventUserLeft,
		Payload: UserPresencePayload{
			UserID:   userID,
			Username: username,
			Role:     role,
		},
	})

	h.mu.RLock()
	empty := len(h.Clients) == 0
	h.mu.RUnlock()

	if empty {
		if h.onDestroy != nil {
			h.onDestroy()
		}
		return true
	}

	return false
}

func (h *RoomHub) destroyRoom() {
	h.mu.Lock()
	var remaining []*Client
	for c := range h.Clients {
		remaining = append(remaining, c)
	}
	for _, pl := range h.pendingLeaves {
		pl.timer.Stop()
	}
	h.pendingLeaves = make(map[string]*pendingLeave)
	h.mu.Unlock()

	h.broadcastEvent(WSMessage{
		Type:    EventRoomClosed,
		Payload: RoomClosedPayload{Reason: "host_left"},
	})

	for _, c := range remaining {
		close(c.Send)
	}

	_ = h.roomRepo.Delete(h.RoomID)
	_ = h.playbackCache.DeletePlaybackState(h.RoomID)

	h.mu.Lock()
	h.Clients = make(map[*Client]bool)
	h.mu.Unlock()

	if h.onDestroy != nil {
		h.onDestroy()
	}
}

// currentPlaybackSnapshot reads the live state, preferring Redis (fast,
// freshest) and falling back to MySQL if the cache is empty (e.g. right
// after a server restart before anyone has synced yet).
func (h *RoomHub) currentPlaybackSnapshot() (domain.PlaybackState, int, *string) {
	if snapshot, err := h.playbackCache.GetPlaybackState(h.RoomID); err == nil && snapshot != nil {
		return snapshot.PlaybackState, snapshot.PlaybackPositionMS, snapshot.CurrentTrackID
	}
	room, err := h.roomRepo.GetByID(h.RoomID)
	if err != nil || room == nil {
		return domain.PlaybackStatePaused, 0, nil
	}
	return room.PlaybackState, room.PlaybackPositionMS, room.CurrentTrackID
}

// pauseRoomOnDisconnect pauses playback in DB and broadcasts CHANGE_STATE to
// all clients so their players stop immediately. Position is intentionally NOT
// reset — everyone resumes from where they left off when someone clicks play.
func (h *RoomHub) pauseRoomOnDisconnect(userID, username, role string) {
	_, currentPos, _ := h.currentPlaybackSnapshot()

	// Fast path: Redis, this is what every client's next INITIAL_STATE read relies on.
	_ = h.playbackCache.SetPlaybackState(h.RoomID, domain.PlaybackStatePaused, currentPos, nil)

	// Slow path: MySQL, off the hot path.
	go func() {
		_ = h.roomRepo.UpdatePlaybackState(h.RoomID, domain.PlaybackStatePaused, nil, nil)
	}()

	h.broadcastEvent(WSMessage{
		Type: EventChangeState,
		Payload: ChangeStatePayload{
			PlaybackState: "paused",
			// PositionMS nil → frontend keeps its current local position
		},
	})

	h.broadcastEvent(WSMessage{
		Type: EventPauseOnDisconnect,
		Payload: PauseOnDisconnectPayload{
			UserID:   userID,
			Username: username,
			Role:     role,
		},
	})
}

func (h *RoomHub) broadcastEvent(msg WSMessage) {
	bytes, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	for client := range h.Clients {
		select {
		case client.Send <- bytes:
		default:
			close(client.Send)
			delete(h.Clients, client)
		}
	}
	h.mu.RUnlock()
}

func (h *RoomHub) BroadcastMessage(msg WSMessage) {
	h.broadcastEvent(msg)
}

// sendInitialState loads the room from MySQL (for full data: tracks, members,
// lyrics, etc.) then overlays the live playback fields from Redis, since
// MySQL's copy of those specific fields can briefly lag behind (see
// EventSyncPlayback / EventChangeState below).
func (h *RoomHub) sendInitialState(client *Client) {
	room, err := h.roomRepo.GetByID(h.RoomID)
	if err != nil || room == nil {
		return
	}

	if snapshot, err := h.playbackCache.GetPlaybackState(h.RoomID); err == nil && snapshot != nil {
		room.PlaybackState = snapshot.PlaybackState
		room.PlaybackPositionMS = snapshot.PlaybackPositionMS
		if snapshot.CurrentTrackID != nil {
			room.CurrentTrackID = snapshot.CurrentTrackID
		}
	}

	initialMsg := WSMessage{
		Type: EventInitialState,
		Payload: map[string]interface{}{
			"room": room,
		},
	}
	bytes, err := json.Marshal(initialMsg)
	if err == nil {
		client.Send <- bytes
	}
}

func (h *RoomHub) HandleIncomingMessage(client *Client, msg *WSMessage, raw []byte) {
	switch msg.Type {
	case EventLeaveRoom:
		h.mu.Lock()
		client.ExplicitLeave = true
		h.mu.Unlock()

	case EventSessionStarted:
		if client.Role != "host" {
			client.SendErrorMessage("Only the host can start the session")
			return
		}

		startedAt, err := h.roomRepo.MarkSessionStarted(h.RoomID)
		if err != nil || startedAt == nil {
			client.SendErrorMessage("Failed to start session")
			return
		}

		h.broadcastEvent(WSMessage{
			Type:    EventSessionStarted,
			Payload: SessionStartedPayload{SessionStartedAt: startedAt.Format(time.RFC3339)},
		})

	case EventSyncPlayback:
		var payload SyncPlaybackPayload
		payloadBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			client.SendErrorMessage("Invalid SYNC_PLAYBACK payload")
			return
		}

		if client.Role == "host" {
			state := domain.PlaybackState(payload.PlaybackState)
			if state != domain.PlaybackStatePlaying && state != domain.PlaybackStatePaused {
				state = domain.PlaybackStatePaused
			}
			positionMS := payload.PlaybackPositionMS
			currentTrackID := payload.CurrentTrackID

			// Fast path: this is what gets broadcast below and what every
			// new INITIAL_STATE read (sendInitialState) sees.
			_ = h.playbackCache.SetPlaybackState(h.RoomID, state, positionMS, currentTrackID)

			// Slow path: MySQL persistence, off the hot path so a slow
			// query never delays this broadcast (this is the exact call
			// that used to show up in the slow query log).
			go func() {
				posCopy := positionMS
				_ = h.roomRepo.UpdatePlaybackState(h.RoomID, state, &posCopy, currentTrackID)
			}()
		}

		payload.Timestamp = time.Now().UnixMilli()
		h.broadcastEvent(WSMessage{
			Type:    EventSyncPlayback,
			Payload: payload,
		})

	case EventChangeState:
		var payload ChangeStatePayload
		payloadBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			client.SendErrorMessage("Invalid CHANGE_STATE payload")
			return
		}

		if client.Role == "host" {
			state := domain.PlaybackState(payload.PlaybackState)

			positionMS := payload.PositionMS
			if positionMS == nil {
				_, currentPos, _ := h.currentPlaybackSnapshot()
				positionMS = &currentPos
			}

			// Fast path.
			_ = h.playbackCache.SetPlaybackState(h.RoomID, state, *positionMS, nil)

			// Slow path.
			go func() {
				_ = h.roomRepo.UpdatePlaybackState(h.RoomID, state, payload.PositionMS, nil)
			}()
		}

		h.broadcastEvent(WSMessage{
			Type:    EventChangeState,
			Payload: payload,
		})

	case EventQueueUpdated:
		tracks, err := h.trackRepo.GetByRoomID(h.RoomID)
		if err == nil {
			h.broadcastEvent(WSMessage{
				Type: EventQueueUpdated,
				Payload: map[string]interface{}{
					"tracks": tracks,
				},
			})
		}

	case EventPlaybackSettings:
		var payload PlaybackSettingsPayload
		payloadBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			client.SendErrorMessage("Invalid PLAYBACK_SETTINGS payload")
			return
		}

		if payload.RepeatMode != "off" && payload.RepeatMode != "all" && payload.RepeatMode != "one" {
			payload.RepeatMode = "off"
		}

		_ = h.roomRepo.UpdatePlaybackSettings(h.RoomID, payload.RepeatMode, payload.IsShuffled)

		h.broadcastEvent(WSMessage{
			Type:    EventPlaybackSettings,
			Payload: payload,
		})

	case EventPing:
		var payload PingPayload
		payloadBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return
		}
		resp, err := json.Marshal(WSMessage{Type: EventPong, Payload: payload})
		if err == nil {
			select {
			case client.Send <- resp:
			default:
			}
		}

	case EventHostLatency:
		if client.Role != "host" {
			return
		}
		var payload HostLatencyPayload
		payloadBytes, _ := json.Marshal(msg.Payload)
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return
		}
		h.broadcastEvent(WSMessage{Type: EventHostLatency, Payload: payload})

	default:
		h.Broadcast <- raw
	}
}