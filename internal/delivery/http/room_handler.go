package http

import (
	"net/http"

	"ripit-be/internal/domain"
	"ripit-be/pkg/response"

	"github.com/gin-gonic/gin"
)

type RoomHandler struct {
	roomService domain.RoomService
}

func NewRoomHandler(roomService domain.RoomService) *RoomHandler {
	return &RoomHandler{
		roomService: roomService,
	}
}

// CreateRoom handles POST /api/v1/rooms
func (h *RoomHandler) CreateRoom(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	userID := userIDVal.(string)

	var req domain.CreateRoomRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	room, err := h.roomService.CreateRoom(userID, &req)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusCreated, "Room created successfully", room)
}

// GetRoomByCode handles GET /api/v1/rooms/:id (accepts room code or room UUID)
func (h *RoomHandler) GetRoomByCode(c *gin.Context) {
	codeOrID := c.Param("id")
	if codeOrID == "" {
		response.BadRequest(c, "Room identifier is required", nil)
		return
	}

	// Try finding by room_code first, then by UUID id if not found
	room, err := h.roomService.GetRoomByCode(codeOrID)
	if err != nil {
		room, err = h.roomService.GetRoomByID(codeOrID)
	}

	if err != nil || room == nil {
		response.NotFound(c, "Room not found")
		return
	}

	response.Success(c, http.StatusOK, "Room fetched successfully", room)
}

// JoinRoom handles POST /api/v1/rooms/:id/join
func (h *RoomHandler) JoinRoom(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	userID := userIDVal.(string)

	codeOrID := c.Param("id")
	if codeOrID == "" {
		response.BadRequest(c, "Room identifier is required", nil)
		return
	}

	res, err := h.roomService.JoinRoom(userID, codeOrID)
	if err != nil {
		// If code wasn't found, try looking up by ID to get code
		if room, rErr := h.roomService.GetRoomByID(codeOrID); rErr == nil && room != nil {
			res, err = h.roomService.JoinRoom(userID, room.RoomCode)
		}
	}

	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "Joined room successfully", res)
}

// UpdatePlayback handles PUT /api/v1/rooms/:id/playback
func (h *RoomHandler) UpdatePlayback(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	userID := userIDVal.(string)

	roomID := c.Param("id")
	if roomID == "" {
		response.BadRequest(c, "Room ID is required", nil)
		return
	}

	var req domain.SyncPlaybackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	room, err := h.roomService.UpdatePlayback(userID, roomID, &req)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "Playback updated successfully", room)
}
