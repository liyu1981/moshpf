package forward

import (
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/tunnel"
)

type Forwarder struct {
	session    *tunnel.Session
	remoteName string
	nextID     uint32
	listeners  map[uint16]net.Listener
	forwards   map[uint16]protocol.ForwardEntry
	mu         sync.Mutex
}

func NewForwarder(session *tunnel.Session, remoteName string) *Forwarder {
	return &Forwarder{
		session:    session,
		remoteName: remoteName,
		listeners:  make(map[uint16]net.Listener),
		forwards:   make(map[uint16]protocol.ForwardEntry),
	}
}

func (f *Forwarder) ListenAndForward(localAddr, remoteHost string, remotePort uint16) error {
	ln, err := net.Listen("tcp", localAddr)

	f.mu.Lock()
	f.forwards[remotePort] = protocol.ForwardEntry{
		LocalAddr:  localAddr,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
	}
	if err != nil {
		entry := f.forwards[remotePort]
		entry.Error = err.Error()
		f.forwards[remotePort] = entry
		f.mu.Unlock()
		return err
	}

	displayHost := remoteHost
	if remoteHost == "localhost" || remoteHost == "127.0.0.1" {
		displayHost = f.remoteName
	}

	if oldLn, exists := f.listeners[remotePort]; exists {
		oldLn.Close()
	}
	f.listeners[remotePort] = ln
	f.mu.Unlock()

	log.Info().
		Str("local", localAddr).
		Str("remote", fmt.Sprintf("%s:%d", displayHost, remotePort)).
		Msg("Forwarding started")

	go func() {
		defer func() {
			ln.Close()
			f.mu.Lock()
			if f.listeners[remotePort] == ln {
				delete(f.listeners, remotePort)
				delete(f.forwards, remotePort)
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

func (f *Forwarder) CloseForward(port uint16) bool {
	f.mu.Lock()
	ln, ok := f.listeners[port]
	if ok {
		ln.Close()
		delete(f.listeners, port)
		delete(f.forwards, port)
		log.Info().
			Str("remote", f.remoteName).
			Uint16("port", port).
			Msg("Forwarding stopped")
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
