# ripit-be

Go backend service for **ripit** (collaborative room-based music listening and real-time playback synchronization).

## Tech Stack
- **Language**: Go (1.22+)
- **HTTP Framework**: Gin Web Framework
- **ORM / DB**: GORM + MySQL (Connection Pooling, UUID v4 Keys)
- **Real-Time**: Gorilla WebSocket (Hub Pattern, concurrency safe)
- **Auth**: JWT (JSON Web Tokens)
- **Validation & Security**: Strict CORS, Token bucket IP rate limiting, WSS support

---

## Project Structure
```text
├── cmd/
│   └── api/
│       └── main.go                 # Application entrypoint & graceful shutdown
├── internal/
│   ├── config/                     # Environment configuration (godotenv)
│   ├── delivery/
│   │   ├── http/                   # Gin HTTP Handlers & Middlewares
│   │   │   ├── handler.go
│   │   │   ├── middleware.go       # Auth, CORS, Rate Limiter
│   │   │   ├── room_handler.go
│   │   │   ├── track_handler.go
│   │   │   ├── user_handler.go
│   │   │   └── youtube_handler.go
│   │   └── ws/                     # Gorilla WebSocket Hub, Client, & Dispatcher
│   │       ├── client.go
│   │       ├── hub.go
│   │       └── message.go
│   ├── domain/                     # Domain Entities, DTOs & Interfaces
│   │   ├── room.go
│   │   ├── track.go
│   │   ├── user.go
│   │   └── youtube.go
│   ├── repository/                 # MySQL Database Repositories
│   │   ├── room_repository.go
│   │   ├── track_repository.go
│   │   └── user_repository.go
│   └── service/                    # Business Logic Services & Unit Tests
│       ├── room_service.go
│       ├── service_test.go
│       ├── track_service.go
│       ├── user_service.go
│       └── youtube_service.go
├── pkg/
│   ├── database/                   # MySQL connection pool setup
│   ├── jwt/                        # JWT token maker & validation
│   ├── logger/                     # App logger
│   └── response/                   # Standardized JSON response helper
├── migrations/
│   ├── 000001_init_schema.up.sql
│   └── 000001_init_schema.down.sql
├── schema.sql                      # Complete MySQL 8.0+ Schema
├── .env.example                    # Environment template
├── go.mod
└── README.md
```

---

## Setup & Running Guide

### 1. Configure Environment Variables
Copy `.env.example` into `.env` and fill in your MySQL credentials:
```bash
cp .env.example .env
```

### 2. Setup Database Schema
Execute `schema.sql` on your MySQL server:
```bash
mysql -u root -p < schema.sql
```
*(Or use `golang-migrate` with the files in `migrations/`)*

### 3. Download Dependencies
```bash
go mod tidy
```

### 4. Run Unit Tests
```bash
go test ./... -v
```

### 5. Run the Application
```bash
go run ./cmd/api/main.go
```

---

## REST API Endpoints

### Auth & Users
- `POST /api/v1/users` - Create guest session (returns JWT token)
- `GET /api/v1/users/me` - Get profile (*Requires Bearer Token*)

### Rooms
- `POST /api/v1/rooms` - Create a room (*Requires Bearer Token*)
- `GET /api/v1/rooms/:room_code` - Get room details by 6-char code
- `POST /api/v1/rooms/:room_code/join` - Join a room (*Requires Bearer Token*)
- `PUT /api/v1/rooms/:id/playback` - Update playback state / timestamp (*Host only*)

### Tracks (Shared Queue)
- `GET /api/v1/rooms/:id/tracks` - List queue tracks
- `POST /api/v1/rooms/:id/tracks` - Add track to queue
- `PUT /api/v1/rooms/:id/tracks/reorder` - Reorder tracks in queue
- `PUT /api/v1/rooms/:id/tracks/:track_id` - Update track/lyrics
- `DELETE /api/v1/rooms/:id/tracks/:track_id` - Delete track from queue

### YouTube Integration
- `POST /api/v1/youtube/check` - Validate URL & fetch metadata (title, artist, duration, cover)

---

## WebSocket Protocol

**Endpoint**: `ws://localhost:8080/ws/rooms/:id?token=YOUR_JWT_TOKEN`

### Events Handled:
- `SYNC_PLAYBACK`: Broadcast playback position and status
- `CHANGE_STATE`: Play / Pause playback
- `QUEUE_UPDATED`: Notify queue mutations
- `USER_JOINED` / `USER_LEFT`: Room member presence updates
