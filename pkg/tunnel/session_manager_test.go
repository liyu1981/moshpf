package tunnel

import (
	"io"
	"sync"
	"testing"
)

type MockMultiplexer struct {
	muxType string
	closed  bool
}

func (m *MockMultiplexer) OpenStream() (io.ReadWriteCloser, error)   { return nil, nil }
func (m *MockMultiplexer) AcceptStream() (io.ReadWriteCloser, error) { return nil, nil }
func (m *MockMultiplexer) Close() error                              { m.closed = true; return nil }
func (m *MockMultiplexer) Type() string                              { return m.muxType }

func TestSessionManager(t *testing.T) {
	sm := NewSessionManager()

	s1 := &Session{Mux: &MockMultiplexer{muxType: "TCP"}}
	s2 := &Session{Mux: &MockMultiplexer{muxType: "QUIC"}}

	var wg sync.WaitGroup
	wg.Add(1)
	sm.Add(s1, func() {
		wg.Done()
	})
	if sm.Count() != 1 {
		t.Errorf("Expected 1 session, got %d", sm.Count())
	}

	if sm.GetBest() != s1 {
		t.Errorf("Expected s1 to be best")
	}

	sm.Add(s2, nil)
	if sm.Count() != 2 {
		t.Errorf("Expected 2 sessions, got %d", sm.Count())
	}

	if sm.GetBest() != s2 {
		t.Errorf("Expected s2 (QUIC) to be best")
	}

	sm.Remove(s1)
	if sm.Count() != 1 {
		t.Errorf("Expected 1 session, got %d", sm.Count())
	}

	wg.Wait() // Ensure onRemove for s1 was called

	sm.CloseAll()
	if sm.Count() != 0 {
		t.Errorf("Expected 0 sessions, got %d", sm.Count())
	}
}
