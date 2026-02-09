package session

import (
	"sync"
	"time"
)

// Session holds the state for a single user interaction session.
type Session struct {
	ID              string
	UserID          string
	CreatedAt       time.Time
	LastActivityAt  time.Time
	EpisodicMemory  []string
	ActiveContext   map[string]string
	mu             sync.RWMutex
}

// Manager handles session lifecycle.
type Manager struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

// Create starts a new session.
func (m *Manager) Create(sessionID, userID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := &Session{
		ID:             sessionID,
		UserID:         userID,
		CreatedAt:      time.Now(),
		LastActivityAt: time.Now(),
		EpisodicMemory: make([]string, 0),
		ActiveContext:  make(map[string]string),
	}
	m.sessions[sessionID] = s
	return s
}

// Get retrieves a session by ID.
func (m *Manager) Get(sessionID string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[sessionID]
	return s, ok
}

// Delete removes a session.
func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, sessionID)
}

// AddEpisodicMemory adds a turn to the session's episodic memory.
func (s *Session) AddEpisodicMemory(entry string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.EpisodicMemory = append(s.EpisodicMemory, entry)
	s.LastActivityAt = time.Now()

	// Keep only last 50 entries
	if len(s.EpisodicMemory) > 50 {
		s.EpisodicMemory = s.EpisodicMemory[len(s.EpisodicMemory)-50:]
	}
}

// GetEpisodicMemory returns a copy of the episodic memory.
func (s *Session) GetEpisodicMemory() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, len(s.EpisodicMemory))
	copy(result, s.EpisodicMemory)
	return result
}

// SetContext sets a key-value pair in the session context.
func (s *Session) SetContext(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ActiveContext[key] = value
	s.LastActivityAt = time.Now()
}

// GetContext returns the active context map.
func (s *Session) GetContext() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]string, len(s.ActiveContext))
	for k, v := range s.ActiveContext {
		result[k] = v
	}
	return result
}

// ListSessions returns all active session IDs.
func (m *Manager) ListSessions() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// CleanupExpired removes sessions older than the given duration.
func (m *Manager) CleanupExpired(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, s := range m.sessions {
		s.mu.RLock()
		if s.LastActivityAt.Before(cutoff) {
			delete(m.sessions, id)
			removed++
		}
		s.mu.RUnlock()
	}
	return removed
}
