package http

import (
	"net/http"

	"ripit-be/internal/domain"
	"ripit-be/pkg/response"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService domain.UserService
}

func NewUserHandler(userService domain.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

// RegisterGuest handles POST /api/v1/users
func (h *UserHandler) RegisterGuest(c *gin.Context) {
	var req domain.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request payload", err.Error())
		return
	}

	res, err := h.userService.RegisterGuest(&req)
	if err != nil {
		response.BadRequest(c, err.Error(), nil)
		return
	}

	response.Success(c, http.StatusCreated, "User registered successfully", res)
}

// GetProfile handles GET /api/v1/users/me
func (h *UserHandler) GetProfile(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}

	user, err := h.userService.GetProfile(userID.(string))
	if err != nil {
		response.NotFound(c, "User profile not found")
		return
	}

	response.Success(c, http.StatusOK, "User profile fetched successfully", user)
}
