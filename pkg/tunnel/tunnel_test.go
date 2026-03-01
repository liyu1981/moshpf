package tunnel

import (
	"net"
	"testing"

	"github.com/liyu1981/moshpf/pkg/protocol"
)

func TestSessionSendReceive(t *testing.T) {
	s_conn, c_conn := net.Pipe()

	errChan := make(chan error, 2)

	var s_session, c_session *Session

	go func() {
		var err error
		s_session, err = NewSession(s_conn, true)
		errChan <- err
	}()

	go func() {
		var err error
		c_session, err = NewSession(c_conn, false)
		errChan <- err
	}()

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			t.Fatalf("NewSession failed: %v", err)
		}
	}

	// Test Send/Receive
	msg := protocol.Hello{Version: "1.2.3", AutoForward: true}
	go func() {
		errChan <- c_session.Send(msg)
	}()

	received, err := s_session.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	h, ok := received.(protocol.Hello)
	if !ok {
		t.Fatalf("Expected Hello, got %T", received)
	}

	if h.Version != "1.2.3" {
		t.Errorf("Expected version 1.2.3, got %s", h.Version)
	}

	// Test Heartbeat termination
	// This might take too long to test the actual 35s timeout.
	// But we can check if it sends Heartbeat.
}
