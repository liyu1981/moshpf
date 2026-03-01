package util

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestProxy(t *testing.T) {
	p1_a, p1_b := net.Pipe()
	p2_a, p2_b := net.Pipe()

	done := make(chan struct{})
	go func() {
		Proxy(p1_a, p2_a)
		close(done)
	}()

	msg1 := []byte("hello from 1")
	msg2 := []byte("hello from 2")

	// Test 1 -> 2
	go func() {
		p1_b.Write(msg1)
	}()

	buf2 := make([]byte, len(msg1))
	n, err := p2_b.Read(buf2)
	if err != nil {
		t.Errorf("Read from 2 failed: %v", err)
	}
	if !bytes.Equal(buf2[:n], msg1) {
		t.Errorf("Expected %q, got %q", msg1, buf2[:n])
	}

	// Test 2 -> 1
	go func() {
		p2_b.Write(msg2)
	}()

	buf1 := make([]byte, len(msg2))
	n, err = p1_b.Read(buf1)
	if err != nil {
		t.Errorf("Read from 1 failed: %v", err)
	}
	if !bytes.Equal(buf1[:n], msg2) {
		t.Errorf("Expected %q, got %q", msg2, buf1[:n])
	}

	// Closing both ends should terminate the Proxy
	p1_b.Close()
	p2_b.Close()

	select {
	case <-time.After(2 * time.Second):
		t.Error("Proxy did not exit after connections closed")
	case <-done:
		// Success
	}
}
