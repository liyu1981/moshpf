package agent

import (
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/user/moshpf/pkg/logger"
	"github.com/user/moshpf/pkg/protocol"
	"github.com/user/moshpf/pkg/tunnel"
)

func Run() error {
	logger.Init()
	log.Info().Msg("Agent starting")

	conn := &stdioConn{
		stdin:  os.Stdin,
		stdout: os.Stdout,
	}

	session, err := tunnel.NewSession(conn, true)
	if err != nil {
		return err
	}

	msg, err := session.Receive()
	if err != nil {
		return err
	}
	hello, ok := msg.(protocol.Hello)
	if !ok {
		return fmt.Errorf("expected Hello, got %T", msg)
	}

	if hello.Version != protocol.Version {
		_ = session.Send(protocol.Shutdown{Reason: "Version mismatch"})
		return fmt.Errorf("version mismatch: %s != %s", hello.Version, protocol.Version)
	}

	if err := session.Send(protocol.HelloAck{Version: protocol.Version}); err != nil {
		return err
	}

	stopHeartbeat := make(chan struct{})
	go session.StartHeartbeat(stopHeartbeat)
	defer close(stopHeartbeat)

	go startUnixSocketServer(session)

	for {
		msg, err := session.Receive()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		switch m := msg.(type) {
		case protocol.Heartbeat:
			_ = session.Send(protocol.HeartbeatAck{})
		case protocol.HeartbeatAck:
			// OK
		case protocol.Shutdown:
			return nil
		case protocol.ForwardRequest:
			log.Info().Str("host", m.Host).Uint16("port", m.Port).Msg("Forward request received")
			go handleForward(session, m)
		default:
			log.Debug().Type("type", msg).Msg("Unknown message type received")
		}
	}
}

func handleForward(session *tunnel.Session, req protocol.ForwardRequest) {
	stream, err := session.Yamux.Accept()
	if err != nil {
		log.Error().Err(err).Msg("Failed to accept data stream")
		return
	}
	defer stream.Close()

	target := fmt.Sprintf("%s:%d", req.Host, req.Port)
	remoteConn, err := net.Dial("tcp", target)
	if err != nil {
		log.Error().Err(err).Str("target", target).Msg("Failed to dial target")
		_ = session.Send(protocol.ForwardErr{ID: req.ID, Reason: err.Error()})
		return
	}
	defer remoteConn.Close()

	_ = session.Send(protocol.ForwardAck{ID: req.ID})

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		io.Copy(remoteConn, stream)
		wg.Done()
	}()
	go func() {
		io.Copy(stream, remoteConn)
		wg.Done()
	}()
	wg.Wait()
}

func startUnixSocketServer(session *tunnel.Session) {
	sockPath := protocol.GetUnixSocketPath()
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Error().Err(err).Str("path", sockPath).Msg("Failed to listen on unix socket")
		return
	}
	defer ln.Close()
	defer os.Remove(sockPath)

	log.Info().Str("path", sockPath).Msg("Listening for CLI requests")

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleUnixConn(session, conn)
	}
}

func handleUnixConn(session *tunnel.Session, conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	portStr := strings.TrimSpace(string(buf[:n]))
	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		log.Warn().Str("input", portStr).Msg("Invalid port from unix socket")
		return
	}

	localAddr := fmt.Sprintf(":%d", port)
	remoteHost := "localhost"
	remotePort := uint16(port)

	log.Info().Uint16("port", remotePort).Msg("Requesting listen from daemon")
	err = session.Send(protocol.ListenRequest{
		LocalAddr:  localAddr,
		RemoteHost: remoteHost,
		RemotePort: remotePort,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to send ListenRequest")
	}
}

type stdioConn struct {
	stdin  io.Reader
	stdout io.Writer
}

func (c *stdioConn) Read(p []byte) (n int, err error) {
	return c.stdin.Read(p)
}

func (c *stdioConn) Write(p []byte) (n int, err error) {
	return c.stdout.Write(p)
}

func (c *stdioConn) Close() error {
	return nil
}
