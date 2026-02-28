package tunnel

import (
	"sync"
)

// SessionManager manages multiple sessions (TCP/QUIC) and provides the best one.
type SessionManager struct {
	sessions map[*Session]chan struct{}
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[*Session]chan struct{}),
	}
}

func (m *SessionManager) Add(s *Session, onRemove func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	stop := make(chan struct{})
	m.sessions[s] = stop

	go s.StartHeartbeat(stop)

	if onRemove != nil {
		go func() {
			<-stop
			onRemove()
		}()
	}
}

func (m *SessionManager) Remove(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if stop, ok := m.sessions[s]; ok {
		close(stop)
		delete(m.sessions, s)
		s.Mux.Close()
	}
}

func (m *SessionManager) GetBest() *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var best *Session
	for s := range m.sessions {
		if best == nil {
			best = s
			continue
		}
		// Prefer QUIC
		if s.Mux.Type() == "QUIC" {
			best = s
		}
	}
	return best
}

func (m *SessionManager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for s, stop := range m.sessions {
		close(stop)
		s.Mux.Close()
	}
	m.sessions = make(map[*Session]chan struct{})
}

func (m *SessionManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}
