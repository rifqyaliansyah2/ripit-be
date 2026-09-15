package http

import (
	"ripit-be/internal/config"
	"ripit-be/internal/delivery/ws"
	"ripit-be/pkg/jwt"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

type RouterDependencies struct {
	Config          *config.Config
	TokenMaker      *jwt.TokenMaker
	UserHandler     *UserHandler
	RoomHandler     *RoomHandler
	TrackHandler    *TrackHandler
	YouTubeHandler  *YouTubeHandler
	WSTicketHandler *WSTicketHandler
	HubManager      *ws.HubManager
}

func SetupRouter(deps *RouterDependencies) *gin.Engine {
	if deps.Config.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger())

	// Global Middlewares
	router.Use(CORSMiddleware(deps.Config.CORSAllowedOrigins))

	limiter := NewIPRateLimiter(rate.Limit(deps.Config.RateLimitReqPerSecond), deps.Config.RateLimitBurst)
	router.Use(RateLimitMiddleware(limiter))

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "app": "ripit-be"})
	})

	// WebSocket Endpoint
	// ws://host/ws/rooms/:id?ticket=... — auth is done inside HandleWS via a
	// short-lived single-use ticket (see POST /api/v1/ws-ticket), NOT via
	// AuthMiddleware/JWT, so the long-lived JWT never travels in this URL.
	wsGroup := router.Group("/ws")
	{
		wsGroup.GET("/rooms/:id", deps.HubManager.HandleWS)
	}

	// RESTful API v1
	v1 := router.Group("/api/v1")
	{
		// Public Auth routes
		users := v1.Group("/users")
		{
			users.POST("", deps.UserHandler.RegisterGuest)
			users.GET("/me", AuthMiddleware(deps.TokenMaker), deps.UserHandler.GetProfile)
		}

		// Exchanges a valid Bearer JWT for a short-lived WS connection ticket.
		v1.POST("/ws-ticket", AuthMiddleware(deps.TokenMaker), deps.WSTicketHandler.IssueTicket)

		// YouTube helper
		v1.POST("/youtube/check", deps.YouTubeHandler.Check)

		// Room routes
		rooms := v1.Group("/rooms")
		{
			// Public room details (viewable before join by room_code or id)
			rooms.GET("/:id", deps.RoomHandler.GetRoomByCode)

			// Protected room routes
			protectedRooms := rooms.Group("")
			protectedRooms.Use(AuthMiddleware(deps.TokenMaker))
			{
				protectedRooms.POST("", deps.RoomHandler.CreateRoom)
				protectedRooms.POST("/:id/join", deps.RoomHandler.JoinRoom)
				protectedRooms.PUT("/:id/playback", deps.RoomHandler.UpdatePlayback)

				// Tracks queue management
				protectedRooms.GET("/:id/tracks", deps.TrackHandler.GetTracks)
				protectedRooms.POST("/:id/tracks", deps.TrackHandler.AddTrack)
				protectedRooms.PUT("/:id/tracks/reorder", deps.TrackHandler.ReorderTracks)
				protectedRooms.PUT("/:id/tracks/:track_id", deps.TrackHandler.UpdateTrack)
				protectedRooms.DELETE("/:id/tracks/:track_id", deps.TrackHandler.DeleteTrack)
			}
		}
	}

	return router
}