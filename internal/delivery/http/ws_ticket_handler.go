package http

import (
	"net/http"

	"ripit-be/pkg/response"
	"ripit-be/pkg/wsticket"

	"github.com/gin-gonic/gin"
)

type WSTicketHandler struct {
	store *wsticket.Store
}

func NewWSTicketHandler(store *wsticket.Store) *WSTicketHandler {
	return &WSTicketHandler{store: store}
}

// IssueTicket handles POST /api/v1/ws-ticket (behind AuthMiddleware).
// Exchanges a valid Bearer JWT for a short-lived, single-use ticket that
// the client then uses to authenticate the WebSocket handshake, so the
// long-lived JWT never has to travel in a URL query string.
func (h *WSTicketHandler) IssueTicket(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		response.Unauthorized(c, "Unauthorized")
		return
	}
	userID := userIDVal.(string)

	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)

	ticket, err := h.store.Issue(userID, username)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to issue ticket", nil)
		return
	}

	response.Success(c, http.StatusOK, "Ticket issued", gin.H{"ticket": ticket})
}