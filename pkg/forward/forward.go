package forward

import (
	"io"
	"net"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog/log"
	"github.com/user/moshpf/pkg/protocol"
	"github.com/user/moshpf/pkg/tunnel"
)

type Forwarder struct {
	session *tunnel.Session
	nextID  uint32
}

func NewForwarder(session *tunnel.Session) *Forwarder {
	return &Forwarder{
		session: session,
	}
}

func (f *Forwarder) ListenAndForward(localAddr, remoteHost string, remotePort uint16) error {
	ln, err := net.Listen("tcp", localAddr)
	if err != nil {
		return err
	}
	log.Info().
		Str("local", localAddr).
		Str("remote_host", remoteHost).
		Uint16("remote_port", remotePort).
		Msg("Forwarding started")

	go func() {
		defer ln.Close()
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
