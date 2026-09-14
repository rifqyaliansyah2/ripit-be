package http

import (
	"net/http"

	"ripit-be/internal/domain"
	"ripit-be/pkg/response"

	"github.com/gin-gonic/gin"
)

type TrackHandler struct {
	trackService domain.TrackService
}

func NewTrackHandler(trackService domain.TrackService) *TrackHandler {
	return &TrackHandler{
		trackService: trackService,
	}
}

// GetTracks handles GET /api/v1/rooms/:id/tracks
func (h *TrackHandler) GetTracks(c *gin.Context) {
	roomID := c.Param("id")
	if roomID == "" {
		response.BadRequest(c, "Room ID is required", nil)
		return
	}

	tracks, err := h.trackService.GetTracksByRoomID(roomID)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "Tracks fetched successfully", tracks)
}

// AddTrack handles POST /api/v1/rooms/:id/tracks
func (h *TrackHandler) AddTrack(c *gin.Context) {
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

	var req domain.AddTrackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	track, err := h.trackService.AddTrack(userID, roomID, &req)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusCreated, "Track added to queue successfully", track)
}

// UpdateTrack handles PUT /api/v1/rooms/:id/tracks/:track_id
func (h *TrackHandler) UpdateTrack(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	userID := userIDVal.(string)

	roomID := c.Param("id")
	trackID := c.Param("track_id")
	if roomID == "" || trackID == "" {
		response.BadRequest(c, "Room ID and Track ID are required", nil)
		return
	}

	var req domain.UpdateTrackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	track, err := h.trackService.UpdateTrack(userID, roomID, trackID, &req)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "Track updated successfully", track)
}

// DeleteTrack handles DELETE /api/v1/rooms/:id/tracks/:track_id
func (h *TrackHandler) DeleteTrack(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	userID := userIDVal.(string)

	roomID := c.Param("id")
	trackID := c.Param("track_id")
	if roomID == "" || trackID == "" {
		response.BadRequest(c, "Room ID and Track ID are required", nil)
		return
	}

	if err := h.trackService.DeleteTrack(userID, roomID, trackID); err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "Track deleted successfully", nil)
}

// ReorderTracks handles PUT /api/v1/rooms/:id/tracks/reorder
func (h *TrackHandler) ReorderTracks(c *gin.Context) {
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

	var req domain.ReorderTracksRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	tracks, err := h.trackService.ReorderTracks(userID, roomID, &req)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "Tracks reordered successfully", tracks)
}
