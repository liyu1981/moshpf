package util

import (
	"bytes"
	"testing"
)

func TestNopWriterCloser(t *testing.T) {
	buf := new(bytes.Buffer)
	nwc := NopWriterCloser{buf}

	data := []byte("hello")
	n, err := nwc.Write(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected %d bytes written, got %d", len(data), n)
	}
	if buf.String() != "hello" {
		t.Errorf("Expected 'hello', got %q", buf.String())
	}

	if err := nwc.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
