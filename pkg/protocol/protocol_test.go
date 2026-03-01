package protocol

import (
	"bytes"
	"encoding/gob"
	"os"
	"strconv"
	"testing"
)

func TestGetUnixSocketPath(t *testing.T) {
	path := GetUnixSocketPath()
	uid := strconv.Itoa(os.Getuid())
	expected := "/tmp/mpf-" + uid + ".sock"
	if path != expected {
		t.Errorf("Expected %q, got %q", expected, path)
	}
}

func TestGetLocalIP(t *testing.T) {
	ip := GetLocalIP()
	if ip == "" {
		t.Error("GetLocalIP returned empty string")
	}
	// It should at least be 127.0.0.1 or some valid IP
}

func TestRegister(t *testing.T) {
	Register()

	// Verify that we can encode/decode a registered type
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	msg := Hello{Version: "1.0.0", AutoForward: true}
	var wrapper Message = msg

	err := enc.Encode(&wrapper)
	if err != nil {
		t.Fatalf("Failed to encode Hello: %v", err)
	}

	var decoded Message
	err = dec.Decode(&decoded)
	if err != nil {
		t.Fatalf("Failed to decode Hello: %v", err)
	}

	decodedHello, ok := decoded.(Hello)
	if !ok {
		t.Fatalf("Expected Hello, got %T", decoded)
	}

	if decodedHello.Version != "1.0.0" || !decodedHello.AutoForward {
		t.Errorf("Decoded message mismatch: %+v", decodedHello)
	}
}
