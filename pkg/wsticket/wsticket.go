package wsticket

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const ttl = 15 * time.Second

type entry struct {
	userID   string
	username string
	expires  time.Time
}

// Store holds short-lived, single-use tickets used to authenticate a
// WebSocket handshake without putting the real JWT in the connection URL
// (where it could end up in proxy or server access logs).
type Store struct {
	mu      sync.Mutex
	tickets map[string]entry
}

func NewStore() *Store {
	s := &Store{tickets: make(map[string]entry)}
	go s.cleanupLoop()
	return s
}

// Issue creates a new ticket for the given user, valid for a few seconds.
func (s *Store) Issue(userID, username string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(buf)

	s.mu.Lock()
	s.tickets[ticket] = entry{userID: userID, username: username, expires: time.Now().Add(ttl)}
	s.mu.Unlock()

	return ticket, nil
}

// Consume validates a ticket and deletes it immediately, so it can only
// ever be redeemed once — even if the URL leaked somewhere.
func (s *Store) Consume(ticket string) (userID, username string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, exists := s.tickets[ticket]
	if !exists {
		return "", "", false
	}
	delete(s.tickets, ticket)

	if time.Now().After(e.expires) {
		return "", "", false
	}
	return e.userID, e.username, true
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		s.mu.Lock()
		for k, e := range s.tickets {
			if now.After(e.expires) {
				delete(s.tickets, k)
			}
		}
		s.mu.Unlock()
	}
}