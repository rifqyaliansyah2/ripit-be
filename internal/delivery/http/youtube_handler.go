package http

import (
	"net/http"

	"ripit-be/internal/domain"
	"ripit-be/pkg/response"

	"github.com/gin-gonic/gin"
)

type YouTubeHandler struct {
	ytService domain.YouTubeService
}

func NewYouTubeHandler(ytService domain.YouTubeService) *YouTubeHandler {
	return &YouTubeHandler{
		ytService: ytService,
	}
}

// Check handles POST /api/v1/youtube/check
func (h *YouTubeHandler) Check(c *gin.Context) {
	var req domain.YouTubeCheckRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	metadata, err := h.ytService.CheckAndFetchMetadata(req.URL)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusOK, "YouTube metadata fetched successfully", metadata)
}
