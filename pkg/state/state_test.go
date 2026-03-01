package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStateManager(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "moshpf-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "forwards.json")
	m := &Manager{
		path: path,
		cfg: Config{
			Remotes: make(map[string]RemoteConfig),
		},
	}

	remote := "user@host"
	slavePort := "1234"
	masterPort := "5678"

	// Test Add
	err = m.AddForward(remote, slavePort, masterPort)
	if err != nil {
		t.Fatalf("AddForward failed: %v", err)
	}

	// Test Get
	forwards := m.GetForwards(remote)
	if forwards[masterPort] != slavePort {
		t.Errorf("Expected %s, got %s", slavePort, forwards[masterPort])
	}

	// Test persistence by loading into a new manager
	m2 := &Manager{
		path: path,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	err = json.Unmarshal(data, &m2.cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if m2.cfg.Remotes[remote].Forwards[masterPort] != slavePort {
		t.Errorf("Persistence check failed: expected %s, got %s", slavePort, m2.cfg.Remotes[remote].Forwards[masterPort])
	}

	// Test Remove
	err = m.RemoveForward(remote, masterPort)
	if err != nil {
		t.Errorf("RemoveForward failed: %v", err)
	}

	forwards = m.GetForwards(remote)
	if _, exists := forwards[masterPort]; exists {
		t.Errorf("Expected masterPort to be removed")
	}
}
