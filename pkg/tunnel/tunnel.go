package tunnel

import (
	"encoding/gob"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/user/moshpf/pkg/protocol"
)

type Session struct {
	Yamux   *yamux.Session
	Control *gob.Encoder
	Decoder *gob.Decoder
	mu      sync.Mutex
}

func NewSession(conn io.ReadWriteCloser, server bool) (*Session, error) {
	protocol.Register()
	
	var ySession *yamux.Session
	var err error
	if server {
		ySession, err = yamux.Server(conn, nil)
	} else {
		ySession, err = yamux.Client(conn, nil)
	}
	if err != nil {
		return nil, err
	}

	// Open stream 0 for control
	var controlStream io.ReadWriteCloser
	if server {
		controlStream, err = ySession.Accept()
	} else {
		controlStream, err = ySession.Open()
	}
	if err != nil {
		ySession.Close()
		return nil, err
	}

	return &Session{
		Yamux:   ySession,
		Control: gob.NewEncoder(controlStream),
		Decoder: gob.NewDecoder(controlStream),
	}, nil
}

func (s *Session) Send(msg protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Control.Encode(&msg)
}

func (s *Session) Receive() (protocol.Message, error) {
	var msg protocol.Message
	err := s.Decoder.Decode(&msg)
	return msg, err
}

func (s *Session) StartHeartbeat(stop chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.Send(protocol.Heartbeat{}); err != nil {
				return
			}
		case <-stop:
			return
		}
	}
}
