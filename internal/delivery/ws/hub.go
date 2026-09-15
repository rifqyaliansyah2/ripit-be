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

const hostReconnectGrace = 6 * time.Second

type HubManager struct {
	rooms          map[string]*RoomHub
	mu             sync.RWMutex
	roomRepo       domain.RoomRepository
	userRepo       domain.UserRepository
	trackRepo      domain.TrackRepository
	ticketStore    *wsticket.Store
	upgrader       websocket.Upgrader
	allowedOrigins map[string]bool
}

func NewHubManager(roomRepo domain.RoomRepository, userRepo domain.UserRepository, trackRepo domain.TrackRepository, ticketStore *wsticket.Store, allowedOrigins []string) *HubManager {
	originMap := make(map[string]bool)
	for _, o := range allowedOrigins {
		originMap[o] = true
	}

	hm := &HubManager{
		rooms:          make(map[string]*RoomHub),
		roomRepo:       roomRepo,
		userRepo:       userRepo,
		trackRepo:      trackRepo,
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
					h.destroyRoom()
				} else {
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

func (h *RoomHub) scheduleHostLeaveGrace() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.hostLeaveGen++
	gen := h.hostLeaveGen
	h.pendingHostTimer = time.AfterFunc(hostReconnectGrace, func() {
		h.hostLeaveTimeout <- gen
	})
}

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
			_ = h.roomRepo.UpdatePlaybackState(h.RoomID, state, &payload.PlaybackPositionMS, payload.CurrentTrackID)
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
			_ = h.roomRepo.UpdatePlaybackState(h.RoomID, state, payload.PositionMS, nil)
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