# Backend Implementation Guide & Task List

**Tech Stack:** Golang (Gin or Fiber), MySQL, WebSockets (Gorilla WebSocket)

## 1. Architecture & Core Principles
- **Clean Architecture & Best Practices**: The project must follow the standard Go project layout (`cmd/`, `internal/`, `pkg/`). Code must be highly modular, adhere to SOLID principles, and use interfaces for dependency injection (Repository-Service-Handler pattern).
- **Speed & Performance**: Use connection pooling for MySQL, efficient JSON serialization (e.g., `sonic` or `goccy/go-json`), and minimal memory allocations (avoiding unnecessary pointers/allocations in hot paths like the WebSocket hub).
- **Security & End-to-End (E2E) Focus**:
  - All primary keys and sensitive identifiers **MUST use UUID v4** to prevent enumeration attacks.
  - Since this is an E2E encrypted room concept, the backend should act mostly as a secure relay. Sensitive track metadata (like exact lyrics) can optionally be handled as encrypted payloads (`encrypted_data` BLOBs) that the server merely stores and broadcasts without decrypting, or at minimum, all WebSocket communication must enforce WSS (TLS) with strict origin and payload validation.
  - Enforce strict CORS, API rate limiting, and secure JWT/Session handling.

## 2. Database Schema (MySQL)
*All `id` fields must use UUID v4.*

### `users`
- `id` (CHAR(36), Primary Key) - UUID v4
- `username` (VARCHAR)
- `avatar_url` (VARCHAR, nullable)
- `created_at` (TIMESTAMP)

### `rooms`
- `id` (CHAR(36), Primary Key) - UUID v4
- `room_code` (VARCHAR, Unique, Indexed) - e.g., "UJF422" (short code for easy user sharing)
- `name` (VARCHAR)
- `host_id` (CHAR(36), Foreign Key -> users.id)
- `current_track_id` (CHAR(36), Foreign Key -> tracks.id, nullable)
- `playback_state` (ENUM: 'playing', 'paused')
- `playback_position_ms` (INT)
- `last_sync_timestamp` (TIMESTAMP)
- `created_at` (TIMESTAMP)

### `room_members`
- `room_id` (CHAR(36), Foreign Key -> rooms.id)
- `user_id` (CHAR(36), Foreign Key -> users.id)
- `role` (ENUM: 'host', 'listener')
- `joined_at` (TIMESTAMP)
- *Primary Key (room_id, user_id)*

### `tracks` (Shared Queue)
- `id` (CHAR(36), Primary Key) - UUID v4
- `room_id` (CHAR(36), Foreign Key -> rooms.id)
- `youtube_url` (VARCHAR)
- `title` (VARCHAR)
- `artist` (VARCHAR)
- `duration` (VARCHAR)
- `cover_url` (VARCHAR)
- `lyrics` (TEXT, nullable)
- `has_lyrics` (BOOLEAN)
- `added_by` (CHAR(36), Foreign Key -> users.id)
- `sort_order` (INT)
- `created_at` (TIMESTAMP)

*(Note: If strictly E2E encrypted, the track metadata fields above should be grouped into a single `encrypted_payload` TEXT/BLOB field that only clients can decrypt).*

---

## 3. Project Structure (Standard Go Layout)
```text
├── cmd/
│   └── api/
│       └── main.go           # Entry point
├── internal/
│   ├── config/               # Environment variables, DB setup
│   ├── delivery/             # HTTP Handlers & WebSocket endpoints
│   ├── domain/               # Structs, Interfaces (Models)
│   ├── repository/           # MySQL queries (GORM or sqlc)
│   └── service/              # Core business logic
├── pkg/                      # Reusable utilities (Logger, JWT, UUID)
├── go.mod
└── go.sum
```

---

## 4. RESTful API Endpoints

### Auth & Users
- `POST /api/v1/users` - Create a guest session
- `GET /api/v1/users/me` - Get current profile

### Rooms
- `POST /api/v1/rooms` - Create a new room
- `GET /api/v1/rooms/:room_code` - Get room details by 6-char code
- `POST /api/v1/rooms/:room_code/join` - Join a room

### Tracks (Queue Management)
- `GET /api/v1/rooms/:id/tracks` - Get all queue tracks
- `POST /api/v1/rooms/:id/tracks` - Add track
- `PUT /api/v1/rooms/:id/tracks/:track_id` - Update track/lyrics
- `DELETE /api/v1/rooms/:id/tracks/:track_id` - Delete track

### YouTube Integration
- `POST /api/v1/youtube/check` - Validate URL & fetch metadata

---

## 5. Real-time Communication (WebSockets)
Endpoint: `WS /ws/rooms/:id`
*Use Go channels and select statements to handle concurrent read/writes efficiently without deadlocks.*

### Events (Client <-> Server)
- `SYNC_PLAYBACK`: Sent by host to sync timestamp.
- `CHANGE_STATE`: Play/Pause commands.
- `QUEUE_UPDATED`: Broadcasts queue mutations.
- `USER_JOINED` / `USER_LEFT`: Updates member list.

---

## 6. Development Tasks (Step-by-Step)

### Phase 1: Foundation & Clean Architecture
- [ ] Initialize `go mod init`. Setup the standard project layout (`cmd`, `internal`, `pkg`).
- [ ] Configure environment variables using `viper` or `godotenv`.
- [ ] Setup high-performance router (Gin or Fiber).
- [ ] Setup MySQL connection pooling and optimized settings (`SetMaxOpenConns`, `SetMaxIdleConns`).
- [ ] Create domain models and define interfaces for Repositories and Services.

### Phase 2: Database & UUID Integration
- [ ] Use `google/uuid` package for all primary and foreign keys.
- [ ] Implement robust database migrations (e.g., using `golang-migrate`).
- [ ] Implement Repositories for Users, Rooms, and Tracks.

### Phase 3: Core Business Logic (Services & Delivery)
- [ ] Implement Services mapping to domain interfaces.
- [ ] Implement robust error handling (custom error types, standardized JSON error responses).
- [ ] Wire HTTP Handlers in `delivery/` and register routes.

### Phase 4: WebSockets & Security
- [ ] Setup Gorilla WebSocket upgrader with strict Origin checking.
- [ ] Implement the Room Hub pattern with goroutines. Ensure thread safety using `sync.RWMutex` or channels.
- [ ] Enforce security: API Rate Limiting, JWT validation middleware, WSS for production.

### Phase 5: YouTube & Final Polish
- [ ] Implement YouTube metadata fetching logic.
- [ ] Write unit tests for core services (using `testify` and mocks).

note: I'll handle the database setup myself, so just send me the .sql file. Also, leave the .env file for me to configure—I'll fill in all the credentials and sensitive keys on my end.