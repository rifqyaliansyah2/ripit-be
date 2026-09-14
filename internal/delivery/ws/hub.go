package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"ripit-be/internal/domain"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// hostReconnectGrace is how long the room stays alive after the host's
// connection drops before it's actually torn down. Covers page navigation,
// tab refresh, and brief network blips — anything longer than this and we
// treat the host as genuinely gone.
const hostReconnectGrace = 6 * time.Second

type HubManager struct {
	rooms          map[string]*RoomHub
	mu             sync.RWMutex
	roomRepo       domain.RoomRepository
	userRepo       domain.UserRepository
	trackRepo      domain.TrackRepository
	upgrader       websocket.Upgrader
	allowedOrigins map[string]bool
}

func NewHubManager(roomRepo domain.RoomRepository, userRepo domain.UserRepository, trackRepo domain.TrackRepository, allowedOrigins []string) *HubManager {
	originMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		originMap[o] = true
	}

	hm := &HubManager{
		rooms:          make(map[string]*RoomHub),
		roomRepo:       roomRepo,
		userRepo:       userRepo,
		trackRepo:      trackRepo,
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

	hub := NewRoomHub(roomID, hm.roomRepo, hm.trackRepo, func() {
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

	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := userIDVal.(string)

	usernameVal, _ := c.Get("username")
	username := "Guest"
	if usernameVal != nil {
		username = usernameVal.(string)
	}

	// Verify room exists
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

// RoomHub maintains the set of active clients and broadcasts messages to the room.
type RoomHub struct {
	RoomID     string
	Clients    map[*Client]bool
	Broadcast  chan []byte
	Register   chan *Client
	Unregister chan *Client
	onDestroy  func()
	roomRepo   domain.RoomRepository
	trackRepo  domain.TrackRepository
	mu         sync.RWMutex

	// Host-disconnect grace period bookkeeping. An implicit host
	// disconnect doesn't destroy the room immediately — it schedules a
	// delayed check via hostLeaveTimeout, guarded by hostLeaveGen so a
	// reconnect within the grace window (e.g. navigating pages) cancels
	// a timer that's already in flight.
	hostLeaveTimeout chan int
	pendingHostTimer *time.Timer
	hostLeaveGen     int
}

func NewRoomHub(roomID string, roomRepo domain.RoomRepository, trackRepo domain.TrackRepository, onDestroy func()) *RoomHub {
	return &RoomHub{
		RoomID:           roomID,
		Clients:          make(map[*Client]bool),
		Broadcast:        make(chan []byte),
		Register:         make(chan *Client),
		Unregister:       make(chan *Client),
		hostLeaveTimeout: make(chan int),
		onDestroy:        onDestroy,
		roomRepo:         roomRepo,
		trackRepo:        trackRepo,
	}
}

func (h *RoomHub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.mu.Lock()
			// Host reconnected within the grace window (e.g. navigated
			// from /setup to the room page) — cancel the pending destroy.
			if client.Role == "host" && h.pendingHostTimer != nil {
				h.pendingHostTimer.Stop()
				h.pendingHostTimer = nil
				h.hostLeaveGen++
			}
			h.Clients[client] = true
			h.mu.Unlock()

			h.sendInitialState(client)

			h.broadcastEvent(WSMessage{
				Type: EventUserJoined,
				Payload: UserPresencePayload{
					UserID:   client.UserID,
					Username: client.Username,
					Role:     client.Role,
				},
			})

		case client := <-h.Unregister:
			h.mu.Lock()
			_, existed := h.Clients[client]
			if !existed {
				h.mu.Unlock()
				continue
			}
			delete(h.Clients, client)
			close(client.Send)
			isHost := client.Role == "host"
			explicit := client.ExplicitLeave
			h.mu.Unlock()

			if isHost {
				if explicit {
					// User clicked "Leave Room" — no reason to wait.
					h.destroyRoom()
				} else {
					// Connection dropped without warning (navigation,
					// refresh, network blip) — give it a chance to
					// reconnect before tearing the room down.
					h.scheduleHostLeaveGrace()
				}
				continue
			}

			_ = h.roomRepo.RemoveMember(h.RoomID, client.UserID)

			h.broadcastEvent(WSMessage{
				Type: EventUserLeft,
				Payload: UserPresencePayload{
					UserID:   client.UserID,
					Username: client.Username,
					Role:     client.Role,
				},
			})

			h.mu.RLock()
			empty := len(h.Clients) == 0
			h.mu.RUnlock()

			if empty {
				if h.onDestroy != nil {
					h.onDestroy()
				}
				return
			}

		case gen := <-h.hostLeaveTimeout:
			h.mu.Lock()
			if gen != h.hostLeaveGen {
				// Stale timer — host already reconnected since this
				// was scheduled. Ignore it.
				h.mu.Unlock()
				continue
			}
			h.pendingHostTimer = nil
			h.mu.Unlock()

			h.destroyRoom()
			return

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

// scheduleHostLeaveGrace delays room destruction after an implicit host
// disconnect, giving the host hostReconnectGrace to reconnect (e.g. finish
// navigating to another page in the room) before the room is torn down.
func (h *RoomHub) scheduleHostLeaveGrace() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hostLeaveGen++
	gen := h.hostLeaveGen
	h.pendingHostTimer = time.AfterFunc(hostReconnectGrace, func() {
		h.hostLeaveTimeout <- gen
	})
}

// destroyRoom broadcasts ROOM_CLOSED, cuts off remaining clients, and
// deletes the room. Called either immediately (explicit leave) or once the
// grace window expires without the host reclaiming the room.
func (h *RoomHub) destroyRoom() {
	h.mu.Lock()
	var remaining []*Client
	for c := range h.Clients {
		remaining = append(remaining, c)
	}
	h.mu.Unlock()

	h.broadcastEvent(WSMessage{
		Type:    EventRoomClosed,
		Payload: RoomClosedPayload{Reason: "host_left"},
	})

	for _, c := range remaining {
		close(c.Send)
	}

	_ = h.roomRepo.Delete(h.RoomID)

	h.mu.Lock()
	h.Clients = make(map[*Client]bool)
	h.mu.Unlock()

	if h.onDestroy != nil {
		h.onDestroy()
	}
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

func (h *RoomHub) sendInitialState(client *Client) {
	room, err := h.roomRepo.GetByID(h.RoomID)
	if err != nil || room == nil {
		return
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
		// Client is leaving on purpose. Mark it so the Unregister
		// handler skips the grace period. The client closes its own
		// socket right after sending this, which triggers ReadPump's
		// deferred Unregister as usual.
		h.mu.Lock()
		client.ExplicitLeave = true
		h.mu.Unlock()

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
			_ = h.roomRepo.UpdatePlaybackState(h.RoomID, state, payload.PlaybackPositionMS, payload.CurrentTrackID)
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
			pos := 0
			if payload.PositionMS != nil {
				pos = *payload.PositionMS
			}
			_ = h.roomRepo.UpdatePlaybackState(h.RoomID, state, pos, nil)
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

	default:
		h.Broadcast <- raw
	}
}