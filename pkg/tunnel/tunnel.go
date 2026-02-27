package tunnel

import (
	"context"
	"encoding/gob"
	"io"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/quic-go/quic-go"
)

type Multiplexer interface {
	OpenStream() (io.ReadWriteCloser, error)
	AcceptStream() (io.ReadWriteCloser, error)
	Close() error
	Type() string
}

type YamuxMultiplexer struct {
	Session *yamux.Session
}

func (y *YamuxMultiplexer) Type() string {
	return "TCP"
}

func (y *YamuxMultiplexer) OpenStream() (io.ReadWriteCloser, error) {
	return y.Session.Open()
}

func (y *YamuxMultiplexer) AcceptStream() (io.ReadWriteCloser, error) {
	return y.Session.Accept()
}

func (y *YamuxMultiplexer) Close() error {
	return y.Session.Close()
}

type QuicMultiplexer struct {
	Conn *quic.Conn
}

func (q *QuicMultiplexer) Type() string {
	return "QUIC"
}

func (q *QuicMultiplexer) OpenStream() (io.ReadWriteCloser, error) {
	return q.Conn.OpenStreamSync(context.Background())
}

func (q *QuicMultiplexer) AcceptStream() (io.ReadWriteCloser, error) {
	return q.Conn.AcceptStream(context.Background())
}

func (q *QuicMultiplexer) Close() error {
	return q.Conn.CloseWithError(0, "")
}

type Session struct {
	Mux          Multiplexer
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

	mux := &YamuxMultiplexer{Session: ySession}

	// Open stream 0 for control
	var controlStream io.ReadWriteCloser
	if server {
		controlStream, err = mux.AcceptStream()
	} else {
		controlStream, err = mux.OpenStream()
	}
	if err != nil {
		mux.Close()
		return nil, err
	}

	return &Session{
		Mux:          mux,
		Control:      gob.NewEncoder(controlStream),
		Decoder:      gob.NewDecoder(controlStream),
		lastReceived: time.Now(),
	}, nil
}

func NewQuicSession(qConn *quic.Conn, server bool) (*Session, error) {
	protocol.Register()

	mux := &QuicMultiplexer{Conn: qConn}

	// Open stream 0 for control
	var controlStream io.ReadWriteCloser
	var err error
	if server {
		controlStream, err = mux.AcceptStream()
	} else {
		controlStream, err = mux.OpenStream()
	}
	if err != nil {
		mux.Close()
		return nil, err
	}

	return &Session{
		Mux:          mux,
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
				s.Mux.Close()
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
