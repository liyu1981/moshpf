package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	Remotes map[string]RemoteConfig `json:"remotes"`
}

type RemoteConfig struct {
	// Map of masterPort -> slavePort
	Forwards map[string]string `json:"forwards"`
}

type Manager struct {
	path string
	mu   sync.Mutex
	cfg  Config
}

func NewManager() (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(home, ".mpf")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "forwards.json")

	m := &Manager{
		path: path,
		cfg: Config{
			Remotes: make(map[string]RemoteConfig),
		},
	}

	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err == nil {
			_ = json.Unmarshal(data, &m.cfg)
		}
	}

	if m.cfg.Remotes == nil {
		m.cfg.Remotes = make(map[string]RemoteConfig)
	}

	return m, nil
}

func (m *Manager) AddForward(remote, slavePort, masterPort string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc, ok := m.cfg.Remotes[remote]
	if !ok || rc.Forwards == nil {
		rc = RemoteConfig{Forwards: make(map[string]string)}
	}

	rc.Forwards[masterPort] = slavePort
	m.cfg.Remotes[remote] = rc
	return m.save()
}

func (m *Manager) RemoveForward(remote, masterPort string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc, ok := m.cfg.Remotes[remote]
	if !ok || rc.Forwards == nil {
		return nil
	}

	delete(rc.Forwards, masterPort)
	m.cfg.Remotes[remote] = rc
	return m.save()
}

func (m *Manager) GetForwards(remote string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := make(map[string]string)
	for k, v := range m.cfg.Remotes[remote].Forwards {
		res[k] = v
	}
	return res
}

func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0600)
}
