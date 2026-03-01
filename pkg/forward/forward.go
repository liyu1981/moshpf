package forward

import (
	"encoding/gob"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/state"
	"github.com/liyu1981/moshpf/pkg/tunnel"
	"github.com/liyu1981/moshpf/pkg/util"
	"github.com/rs/zerolog/log"
)

type Forwarder struct {
	sessions   *tunnel.SessionManager
	remoteName string
	masterIP   string
	localOnly  bool
	nextID     uint32
	listeners  map[uint16]net.Listener
	forwards   map[uint16]protocol.ForwardEntry
	state      *state.Manager
	target     string // user@host
	mu         sync.Mutex
}

func NewForwarder(session *tunnel.Session, remoteName string, stateMgr *state.Manager, target string, localOnly bool) *Forwarder {
	f := &Forwarder{
		sessions:   tunnel.NewSessionManager(),
		remoteName: remoteName,
		masterIP:   protocol.GetLocalIP(),
		localOnly:  localOnly,
		state:      stateMgr,
		target:     target,
		listeners:  make(map[uint16]net.Listener),
		forwards:   make(map[uint16]protocol.ForwardEntry),
	}
	if session != nil {
		f.AddSession(session)
	}
	return f
}

func (f *Forwarder) GetRemoteName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remoteName
}

func (f *Forwarder) GetMasterIP() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.masterIP
}

func (f *Forwarder) AddSession(session *tunnel.Session) {
	f.sessions.Add(session, nil)
}

func (f *Forwarder) RemoveSession(session *tunnel.Session) {
	f.sessions.Remove(session)
}

func (f *Forwarder) GetSessions() *tunnel.SessionManager {
	return f.sessions
}

func (f *Forwarder) getBestSession() *tunnel.Session {
	return f.sessions.GetBest()
}

func (f *Forwarder) getBestSessionLocked() *tunnel.Session {
	return f.sessions.GetBest()
}

func (f *Forwarder) ListenAndForward(localAddr, remoteHost string, remotePort uint16, isAuto bool) error {
	var masterPort uint16

	// Resolve localAddr based on localOnly if it is a port-only address
	if strings.HasPrefix(localAddr, ":") {
		if f.localOnly {
			localAddr = "127.0.0.1" + localAddr
		} else {
			localAddr = "0.0.0.0" + localAddr
		}
	}

	fmt.Sscanf(localAddr, "%*[^:]:%d", &masterPort)
	if masterPort == 0 {
		// Try to parse if it contains hostname
		_, portStr, err := net.SplitHostPort(localAddr)
		if err == nil {
			fmt.Sscanf(portStr, "%d", &masterPort)
		}
	}

	f.mu.Lock()
	if _, exists := f.listeners[masterPort]; exists {
		f.mu.Unlock()
		return fmt.Errorf("port %d already has an active listener", masterPort)
	}

	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		f.forwards[masterPort] = protocol.ForwardEntry{
			LocalAddr:  localAddr,
			RemoteHost: remoteHost,
			RemotePort: remotePort,
			IsAuto:     isAuto,
			Error:      err.Error(),
		}
		f.mu.Unlock()
		return err
	}

	if masterPort == 0 {
		if addr, ok := ln.Addr().(*net.TCPAddr); ok {
			masterPort = uint16(addr.Port)
		}
	}

	f.forwards[masterPort] = protocol.ForwardEntry{
		LocalAddr:  localAddr,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
		IsAuto:     isAuto,
	}

	if f.state != nil {
		_ = f.state.AddForward(f.target, fmt.Sprintf("%d", remotePort), fmt.Sprintf("%d", masterPort))
	}

	displayHost := remoteHost
	if remoteHost == "localhost" || remoteHost == "127.0.0.1" {
		displayHost = f.remoteName
	}

	f.listeners[masterPort] = ln
	f.mu.Unlock()

	log.Info().
		Str("local", localAddr).
		Str("remote", fmt.Sprintf("%s:%d", displayHost, remotePort)).
		Msg("Forwarding started")

	go func() {
		defer func() {
			ln.Close()
			f.mu.Lock()
			if f.listeners[masterPort] == ln {
				delete(f.listeners, masterPort)
				delete(f.forwards, masterPort)
			}
			f.mu.Unlock()
		}()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.handleConnection(conn, remoteHost, remotePort)
		}
	}()

	return nil
}

func (f *Forwarder) CloseForward(masterPort uint16) bool {
	f.mu.Lock()
	ln, ok := f.listeners[masterPort]
	if ok {
		ln.Close()
		delete(f.listeners, masterPort)
		delete(f.forwards, masterPort)
		if f.state != nil {
			_ = f.state.RemoveForward(f.target, fmt.Sprintf("%d", masterPort))
		}
		log.Info().
			Str("remote", f.remoteName).
			Uint16("port", masterPort).
			Msg("Forwarding stopped")
	} else if _, exists := f.forwards[masterPort]; exists {
		// Even if no listener (e.g. failed), remove from forwards and state
		delete(f.forwards, masterPort)
		if f.state != nil {
			_ = f.state.RemoveForward(f.target, fmt.Sprintf("%d", masterPort))
		}
	}
	f.mu.Unlock()
	return ok
}

func (f *Forwarder) GetForwardEntries() []protocol.ForwardEntry {
	f.mu.Lock()
	defer f.mu.Unlock()

	transport := "NONE"
	s := f.getBestSessionLocked()
	if s != nil {
		transport = s.Mux.Type()
	}

	entries := make([]protocol.ForwardEntry, 0, len(f.forwards))
	for _, e := range f.forwards {
		e.Transport = transport
		entries = append(entries, e)
	}
	return entries
}

func (f *Forwarder) handleConnection(localConn net.Conn, remoteHost string, remotePort uint16) {
	defer localConn.Close()

	s := f.getBestSession()
	if s == nil {
		log.Error().Msg("No active session for forwarding")
		return
	}

	remoteConn, err := s.Mux.OpenStream()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open multiplexer stream")
		return
	}
	defer remoteConn.Close()

	// Send header directly on the stream
	encoder := gob.NewEncoder(remoteConn)
	err = encoder.Encode(protocol.StreamHeader{
		Host: remoteHost,
		Port: remotePort,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to send stream header")
		return
	}

	// Wait for 1-byte ACK (1 = success, 0 = fail)
	ack := make([]byte, 1)
	_, err = remoteConn.Read(ack)
	if err != nil || ack[0] != 1 {
		log.Error().Err(err).Msg("Failed to get stream ACK")
		return
	}

	util.Proxy(localConn, remoteConn)
}
