package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/liyu1981/moshpf/pkg/constant"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/rs/zerolog/log"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type AutoForwarder struct {
	agent          *Agent
	activeForwards map[uint32]bool
	mu             sync.Mutex
	stopChan       chan struct{}
	excludedSubs   []string
	currentExe     string
}

func NewAutoForwarder(agent *Agent) *AutoForwarder {
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)

	return &AutoForwarder{
		agent:          agent,
		activeForwards: make(map[uint32]bool),
		stopChan:       make(chan struct{}),
		currentExe:     exe,
		excludedSubs:   constant.AutoForwardExcludedSubstrings,
	}
}

func (af *AutoForwarder) Start() {
	go af.run()
}

func (af *AutoForwarder) Stop() {
	close(af.stopChan)
}

func (af *AutoForwarder) run() {
	ticker := time.NewTicker(constant.AutoForwardScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			af.scan()
		case <-af.stopChan:
			return
		}
	}
}

func (af *AutoForwarder) scan() {
	ports, err := af.listListeningPorts()
	if err != nil {
		log.Error().Err(err).Msg("Failed to list listening ports")
		return
	}

	af.mu.Lock()
	foundPorts := make(map[uint32]bool)
	for _, p := range ports {
		foundPorts[p] = true
		if !af.activeForwards[p] {
			af.startForward(p)
		}
	}

	// Detect closed ports
	for p := range af.activeForwards {
		if !foundPorts[p] {
			af.stopForward(p)
		}
	}
	af.mu.Unlock()
}

func (af *AutoForwarder) startForward(port uint32) {
	s := af.agent.getBestSession()
	if s == nil {
		return
	}

	log.Info().Uint32("port", port).Msg("Auto-forwarding new port")
	err := s.Send(protocol.ListenRequest{
		LocalAddr:  fmt.Sprintf(":%d", port),
		RemoteHost: "localhost",
		RemotePort: uint16(port),
		IsAuto:     true,
	})

	if err == nil {
		af.activeForwards[port] = true
	} else {
		log.Error().Err(err).Uint32("port", port).Msg("Failed to send ListenRequest for auto-forward")
	}
}

func (af *AutoForwarder) stopForward(port uint32) {
	s := af.agent.getBestSession()
	if s == nil {
		return
	}

	log.Info().Uint32("port", port).Msg("Stopping auto-forward for closed port")
	err := s.Send(protocol.CloseRequest{
		Port: uint16(port),
	})

	if err == nil {
		delete(af.activeForwards, port)
	} else {
		log.Error().Err(err).Uint32("port", port).Msg("Failed to send CloseRequest for auto-forward")
	}
}

func (af *AutoForwarder) listListeningPorts() ([]uint32, error) {
	conns, err := net.Connections("tcp")
	if err != nil {
		return nil, err
	}

	var results []uint32
	for _, c := range conns {
		if c.Status != "LISTEN" {
			continue
		}

		port := c.Laddr.Port
		if port < 1024 {
			continue
		}

		if c.Pid == 0 {
			continue
		}

		p, err := process.NewProcess(c.Pid)
		if err != nil {
			continue
		}

		cmdline, _ := p.Cmdline()
		exe, _ := p.Exe()

		if af.shouldExclude(cmdline, exe) {
			continue
		}

		results = append(results, port)
	}

	return results, nil
}

func (af *AutoForwarder) shouldExclude(cmdline, exe string) bool {
	lc := strings.ToLower(cmdline)
	for _, s := range af.excludedSubs {
		if strings.Contains(lc, s) {
			return true
		}
	}

	exeResolved, _ := filepath.EvalSymlinks(exe)
	if exeResolved == af.currentExe {
		return true
	}

	return false
}
