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
	Forwards []string `json:"forwards"`
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

func (m *Manager) AddForward(remote, port string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc := m.cfg.Remotes[remote]
	exists := false
	for _, p := range rc.Forwards {
		if p == port {
			exists = true
			break
		}
	}
	if !exists {
		rc.Forwards = append(rc.Forwards, port)
		m.cfg.Remotes[remote] = rc
		return m.save()
	}
	return nil
}

func (m *Manager) RemoveForward(remote, port string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	rc := m.cfg.Remotes[remote]
	newForwards := make([]string, 0, len(rc.Forwards))
	for _, p := range rc.Forwards {
		if p != port {
			newForwards = append(newForwards, p)
		}
	}
	rc.Forwards = newForwards
	m.cfg.Remotes[remote] = rc
	return m.save()
}

func (m *Manager) GetForwards(remote string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg.Remotes[remote].Forwards
}

func (m *Manager) save() error {
	data, err := json.MarshalIndent(m.cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0600)
}
