package forward

import (
	"net"
	"testing"

	"github.com/liyu1981/moshpf/pkg/tunnel"
)

func TestForwarder(t *testing.T) {
	// Setup a session using net.Pipe
	s_conn, c_conn := net.Pipe()

	errChan := make(chan error, 2)
	var s_session, c_session *tunnel.Session

	go func() {
		var err error
		s_session, err = tunnel.NewSession(s_conn, true)
		errChan <- err
	}()

	go func() {
		var err error
		c_session, err = tunnel.NewSession(c_conn, false)
		errChan <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			t.Fatalf("NewSession failed: %v", err)
		}
	}
	_ = s_session // Avoid unused error

	f := NewForwarder(c_session, "test-remote", nil, "user@host")

	// Test ListenAndForward
	// Use :0 to get an ephemeral port
	err := f.ListenAndForward(":0", "localhost", 1234, false)
	if err != nil {
		t.Fatalf("ListenAndForward failed: %v", err)
	}

	entries := f.GetForwardEntries()
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(entries))
	}

	var masterPort uint16
	for p := range f.listeners {
		masterPort = p
		break
	}

	if masterPort == 0 {
		t.Fatal("Failed to get master port")
	}

	// Test CloseForward
	if !f.CloseForward(masterPort) {
		t.Errorf("CloseForward failed")
	}

	if len(f.GetForwardEntries()) != 0 {
		t.Errorf("Expected 0 entries after close")
	}
}
