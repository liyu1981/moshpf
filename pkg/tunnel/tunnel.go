package tunnel

import (
	"encoding/gob"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/liyu1981/moshpf/pkg/protocol"
)

type Session struct {
	Yamux        *yamux.Session
	Control      *gob.Encoder
	Decoder      *gob.Decoder
	mu           sync.Mutex
	lastReceived time.Time
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
		Yamux:        ySession,
		Control:      gob.NewEncoder(controlStream),
		Decoder:      gob.NewDecoder(controlStream),
		lastReceived: time.Now(),
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
	if err == nil {
		s.mu.Lock()
		s.lastReceived = time.Now()
		s.mu.Unlock()
	}
	return msg, err
}

func (s *Session) StartHeartbeat(stop chan struct{}) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			last := s.lastReceived
			s.mu.Unlock()

			if time.Since(last) > 35*time.Second {
				// 3 heartbeats missed (30s) + 5s buffer
				s.Yamux.Close()
				return
			}

			if err := s.Send(protocol.Heartbeat{}); err != nil {
				return
			}
		case <-stop:
			return
		}
	}
}
