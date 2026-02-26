package forward

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/state"
	"github.com/liyu1981/moshpf/pkg/tunnel"
)

type Forwarder struct {
	session    *tunnel.Session
	remoteName string
	nextID     uint32
	listeners  map[uint16]net.Listener
	forwards   map[uint16]protocol.ForwardEntry
	state      *state.Manager
	target     string // user@host
	mu         sync.Mutex
}

func NewForwarder(session *tunnel.Session, remoteName string, stateMgr *state.Manager, target string) *Forwarder {
	return &Forwarder{
		session:    session,
		remoteName: remoteName,
		state:      stateMgr,
		target:     target,
		listeners:  make(map[uint16]net.Listener),
		forwards:   make(map[uint16]protocol.ForwardEntry),
	}
}

func (f *Forwarder) GetRemoteName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.remoteName
}

func (f *Forwarder) SetSession(session *tunnel.Session) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.session = session
}

func (f *Forwarder) ListenAndForward(localAddr, remoteHost string, remotePort uint16) error {
	var masterPort uint16
	fmt.Sscanf(localAddr, ":%d", &masterPort)
	if masterPort == 0 {
		// Try to parse if it contains hostname
		_, portStr, err := net.SplitHostPort(localAddr)
		if err == nil {
			fmt.Sscanf(portStr, "%d", &masterPort)
		}
	}

	ln, err := net.Listen("tcp", localAddr)

	f.mu.Lock()
	f.forwards[masterPort] = protocol.ForwardEntry{
		LocalAddr:  localAddr,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
	}
	if err != nil {
		entry := f.forwards[masterPort]
		entry.Error = err.Error()
		f.forwards[masterPort] = entry
		f.mu.Unlock()
		return err
	}

	if f.state != nil {
		_ = f.state.AddForward(f.target, fmt.Sprintf("%d", remotePort), fmt.Sprintf("%d", masterPort))
	}

	displayHost := remoteHost
	if remoteHost == "localhost" || remoteHost == "127.0.0.1" {
		displayHost = f.remoteName
	}

	if oldLn, exists := f.listeners[masterPort]; exists {
		oldLn.Close()
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
	entries := make([]protocol.ForwardEntry, 0, len(f.forwards))
	for _, e := range f.forwards {
		entries = append(entries, e)
	}
	return entries
}

func (f *Forwarder) handleConnection(localConn net.Conn, remoteHost string, remotePort uint16) {
	defer localConn.Close()

	id := atomic.AddUint32(&f.nextID, 1)
	
	err := f.session.Send(protocol.ForwardRequest{
		ID:   id,
		Host: remoteHost,
		Port: remotePort,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to send forward request")
		return
	}

	remoteConn, err := f.session.Yamux.Open()
	if err != nil {
		log.Error().Err(err).Msg("Failed to open yamux stream")
		return
	}
	defer remoteConn.Close()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(remoteConn, localConn)
		wg.Done()
	}()
	go func() {
		io.Copy(localConn, remoteConn)
		wg.Done()
	}()
	wg.Wait()
}
