package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ripit-be/internal/config"
	deliveryHTTP "ripit-be/internal/delivery/http"
	deliveryWS "ripit-be/internal/delivery/ws"
	"ripit-be/internal/repository"
	"ripit-be/internal/service"
	"ripit-be/pkg/database"
	"ripit-be/pkg/jwt"
	"ripit-be/pkg/logger"
	redisclient "ripit-be/pkg/redis"
	"ripit-be/pkg/wsticket"
)

func main() {
	appLog := logger.NewLogger()
	appLog.Info("Starting ripit-be server...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Database Connection
	db, err := database.NewMySQLConnection(cfg)
	if err != nil {
		appLog.Errorf("Database connection error: %v", err)
		appLog.Info("Starting server in detached database mode (warning: database operations will fail until MySQL is up)")
	} else {
		appLog.Info("Successfully connected to MySQL database with connection pooling")
	}

	// 2b. Initialize Redis Connection (used for live playback state — see PlaybackCache)
	rdb, err := redisclient.NewRedisClient(cfg)
	if err != nil {
		appLog.Errorf("Redis connection error: %v", err)
		appLog.Info("Starting server without Redis (warning: playback sync will fall back to MySQL and may be slow)")
	} else {
		appLog.Info("Successfully connected to Redis")
	}

	// 3. Initialize Repositories
	userRepo := repository.NewUserRepository(db)
	roomRepo := repository.NewRoomRepository(db)
	trackRepo := repository.NewTrackRepository(db)
	playbackCache := repository.NewPlaybackCacheRepository(rdb)

	// 4. Initialize Utilities & Services
	tokenMaker := jwt.NewTokenMaker(cfg.JWTSecret, cfg.JWTExpirationHours)
	ticketStore := wsticket.NewStore()
	userService := service.NewUserService(userRepo, tokenMaker)
	roomService := service.NewRoomService(roomRepo, userRepo, trackRepo, playbackCache)
	trackService := service.NewTrackService(trackRepo, roomRepo)
	youtubeService := service.NewYouTubeService(cfg.YouTubeAPIKey)

	// 5. Initialize Delivery Layer (HTTP & WebSockets)
	hubManager := deliveryWS.NewHubManager(roomRepo, userRepo, trackRepo, playbackCache, ticketStore, cfg.CORSAllowedOrigins)
	userHandler := deliveryHTTP.NewUserHandler(userService)
	roomHandler := deliveryHTTP.NewRoomHandler(roomService)
	trackHandler := deliveryHTTP.NewTrackHandler(trackService)
	youtubeHandler := deliveryHTTP.NewYouTubeHandler(youtubeService)
	wsTicketHandler := deliveryHTTP.NewWSTicketHandler(ticketStore)

	router := deliveryHTTP.SetupRouter(&deliveryHTTP.RouterDependencies{
		Config:          cfg,
		TokenMaker:      tokenMaker,
		UserHandler:     userHandler,
		RoomHandler:     roomHandler,
		TrackHandler:    trackHandler,
		YouTubeHandler:  youtubeHandler,
		WSTicketHandler: wsTicketHandler,
		HubManager:      hubManager,
	})

	serverAddr := fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort)
	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// 6. Graceful Shutdown listener
	go func() {
		appLog.Infof("HTTP & WebSocket server running on http://%s", serverAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLog.Errorf("Failed to listen and serve: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	appLog.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		appLog.Errorf("Server forced to shutdown: %v", err)
	}

	if rdb != nil {
		_ = rdb.Close()
	}

	appLog.Info("Server stopped cleanly.")
}