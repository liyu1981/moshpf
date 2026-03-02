package agent

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/liyu1981/moshpf/pkg/constant"
	"github.com/liyu1981/moshpf/pkg/logger"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/tunnel"
	"github.com/liyu1981/moshpf/pkg/util"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
)

type Agent struct {
	sessions      *tunnel.SessionManager
	mu            sync.Mutex
	listChan      chan protocol.ListResponse
	closeChan     chan protocol.CloseResponse
	listenChan    chan protocol.ListenResponse
	shutdownTimer *time.Timer
	autoForwarder *AutoForwarder
}

func (a *Agent) addSession(s *tunnel.Session) {
	a.mu.Lock()
	if a.shutdownTimer != nil {
		log.Info().Msg("New session connected, canceling shutdown timer")
		a.shutdownTimer.Stop()
		a.shutdownTimer = nil
	}
	a.mu.Unlock()

	a.sessions.Add(s, func() {
		var maxWait = 10 * time.Minute
		if util.IsDev() {
			maxWait = 5 * time.Second
		}
		if a.sessions.Count() == 0 {
			a.mu.Lock()
			if a.shutdownTimer != nil {
				a.shutdownTimer.Stop()
			}
			log.Info().Msg("No active sessions left, starting 10-minute shutdown timer")
			a.shutdownTimer = time.AfterFunc(maxWait, func() {
				log.Info().Msg("Shutdown timer expired, agent exiting")
				os.Exit(0)
			})
			a.mu.Unlock()
		}
	})
	go a.startStreamAcceptor(s)
	go a.startControlLoop(s)
}

func (a *Agent) removeSession(s *tunnel.Session) {
	a.sessions.Remove(s)
}

func (a *Agent) getBestSession() *tunnel.Session {
	return a.sessions.GetBest()
}

func (a *Agent) startControlLoop(s *tunnel.Session) {
	for {
		msg, err := s.Receive()
		if err != nil {
			a.removeSession(s)
			return
		}
		a.handleMessage(s, msg)
	}
}

func (a *Agent) handleMessage(s *tunnel.Session, msg protocol.Message) {
	switch m := msg.(type) {
	case protocol.Heartbeat:
		_ = s.Send(protocol.HeartbeatAck{})
	case protocol.HeartbeatAck:
		// OK
	case protocol.Shutdown:
		// Exit process if shutdown received on any session
		os.Exit(0)
	case protocol.ListRequest:
		// Master requesting list from agent (slave)
		// For now, slave doesn't track its own forwards as all listening is on master.
		_ = s.Send(protocol.ListResponse{
			Entries:  []protocol.ForwardEntry{},
			MasterIP: protocol.GetLocalIP(),
		})
	case protocol.ListResponse:
		log.Debug().Int("count", len(m.Entries)).Msg("Agent received ListResponse")
		select {
		case a.listChan <- m:
		default:
			log.Warn().Msg("ListResponse dropped - no receiver")
		}
	case protocol.ListenResponse:
		if m.Success {
			log.Info().Uint16("port", m.RemotePort).Msg("Forwarding confirmed by daemon")
		} else {
			log.Error().Uint16("port", m.RemotePort).Str("reason", m.Reason).Msg("Forwarding failed in daemon")
		}
		select {
		case a.listenChan <- m:
		default:
			log.Warn().Msg("ListenResponse dropped - no receiver")
		}
	case protocol.CloseResponse:
		select {
		case a.closeChan <- m:
		default:
			log.Warn().Msg("CloseResponse dropped - no receiver")
		}
	case protocol.ListenRequest:
		// For future support of reverse port forwarding
		log.Warn().Msg("ListenRequest received from master, not implemented yet")
	default:
		log.Debug().Type("type", msg).Msg("Unknown message type received")
	}
}

func (a *Agent) startStreamAcceptor(s *tunnel.Session) {
	for {
		stream, err := s.Mux.AcceptStream()
		if err != nil {
			return
		}
		go a.handleAcceptedStream(stream)
	}
}

func (a *Agent) handleAcceptedStream(stream io.ReadWriteCloser) {
	defer stream.Close()

	decoder := gob.NewDecoder(stream)
	var header protocol.StreamHeader
	if err := decoder.Decode(&header); err != nil {
		log.Error().Err(err).Msg("Failed to decode stream header")
		return
	}

	target := fmt.Sprintf("%s:%d", header.Host, header.Port)
	remoteConn, err := net.Dial("tcp", target)
	if err != nil {
		log.Error().Err(err).Str("target", target).Msg("Failed to dial target")
		_, _ = stream.Write([]byte{0}) // NAK
		return
	}
	defer remoteConn.Close()

	_, _ = stream.Write([]byte{1}) // ACK

	util.Proxy(remoteConn, stream)
}

func Run() error {
	logger.Init()
	log.Info().Msg("Agent starting")

	// Generate ephemeral cert for QUIC
	cert, fingerprint, err := tunnel.GenerateEphemeralCert()
	if err != nil {
		return fmt.Errorf("failed to generate cert: %v", err)
	}

	// Start QUIC listener
	var qListener *quic.Listener
	var qPort uint16
	quicConfig := &quic.Config{
		Tracer: logger.GetQuicTracer(),
	}

	for {
		port := uint16(constant.QUIC_PORT_START + rand.Intn(constant.QUIC_PORT_END-constant.QUIC_PORT_START+1))
		l, err := quic.ListenAddr(fmt.Sprintf(":%d", port), tunnel.GetTLSConfigServer(cert), quicConfig)
		if err == nil {
			qListener = l
			qPort = port
			log.Info().Uint16("port", qPort).Msg("QUIC listener started")
			break
		}
		log.Debug().Uint16("port", port).Err(err).Msg("Failed to bind to QUIC port, retrying...")
		time.Sleep(100 * time.Millisecond)
	}

	conn := &stdioConn{
		stdin:  os.Stdin,
		stdout: os.Stdout,
	}

	session, err := tunnel.NewSession(conn, true)
	if err != nil {
		return err
	}

	a := &Agent{
		sessions:      tunnel.NewSessionManager(),
		listChan:      make(chan protocol.ListResponse, 10),
		closeChan:     make(chan protocol.CloseResponse, 10),
		listenChan:    make(chan protocol.ListenResponse, 10),
		shutdownTimer: nil,
	}

	msg, err := session.Receive()
	if err != nil {
		return err
	}
	hello, ok := msg.(protocol.Hello)
	if !ok {
		return fmt.Errorf("expected Hello, got %T", msg)
	}

	if hello.Version != constant.Version {
		_ = session.Send(protocol.Shutdown{Reason: "Version mismatch"})
		return fmt.Errorf("version mismatch: %s != %s", hello.Version, constant.Version)
	}

	if hello.AutoForward {
		log.Info().Msg("Auto port forwarding enabled")
		a.autoForwarder = NewAutoForwarder(a)
		a.autoForwarder.Start()
	}

	// Send HelloAck with QUIC info
	ack := protocol.HelloAck{
		Version: constant.Version,
		UDPPort: qPort,
		TLSHash: fingerprint,
	}
	if err := session.Send(ack); err != nil {
		return err
	}

	// Add the primary session
	a.addSession(session)

	go a.startUnixSocketServer()

	// Wait for QUIC connection if listener started
	if qListener != nil {
		go func() {
			defer qListener.Close()
			for {
				qConn, err := qListener.Accept(context.Background())
				if err != nil {
					log.Debug().Err(err).Msg("QUIC accept failed")
					return
				}
				log.Info().Msg("QUIC connection established, adding session")
				qSession, err := tunnel.NewQuicSession(qConn, true)
				if err != nil {
					log.Error().Err(err).Msg("Failed to create QUIC session")
					continue
				}
				a.addSession(qSession)
			}
		}()
	}

	// The main goroutine just blocks now.
	// We can use a channel to wait for a global shutdown if needed.
	select {}
}

func (a *Agent) startUnixSocketServer() {
	sockPath := protocol.GetUnixSocketPath()
	_ = os.Remove(sockPath)

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		log.Fatal().Err(err).Str("path", sockPath).Msg("Failed to listen on unix socket, agent exiting")
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
		go a.handleUnixConn(conn)
	}
}

func (a *Agent) handleUnixConn(conn net.Conn) {
	defer conn.Close()
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return
	}

	cmd := strings.TrimSpace(string(buf[:n]))
	if cmd == "STOP" {
		log.Info().Msg("Stop command received, shutting down")
		_, _ = conn.Write([]byte("Stopping agent..."))
		os.Exit(0)
	}

	if cmd == "SESSIONS" {
		_, _ = conn.Write([]byte(strconv.Itoa(a.sessions.Count())))
		return
	}

	if cmd == "LIST" {
		if a.sessions.Count() == 0 {
			_, _ = conn.Write([]byte("ERROR: No active session"))
			return
		}

		s := a.getBestSession()
		if s == nil {
			_, _ = conn.Write([]byte("ERROR: No active session"))
			return
		}

		err = s.Send(protocol.ListRequest{})
		if err != nil {
			log.Error().Err(err).Msg("Failed to send ListRequest")
			return
		}

		select {
		case resp := <-a.listChan:
			res := fmt.Sprintf("Session: %s -> %s\n", resp.MasterIP, protocol.GetLocalIP())
			for _, e := range resp.Entries {
				status := "OK"
				if e.Error != "" {
					status = "ERROR: " + e.Error
				}

				localAddr := e.LocalAddr
				if strings.HasPrefix(localAddr, ":") {
					localAddr = resp.MasterIP + localAddr
				}

				autoStr := "MANUAL"
				if e.IsAuto {
					autoStr = "AUTO"
				}

				res += fmt.Sprintf("  %d -> %s [%s] (%s) %s\n", e.RemotePort, localAddr, e.Transport, status, autoStr)
			}
			_, _ = conn.Write([]byte(res))
		case <-time.After(5 * time.Second):
			_, _ = conn.Write([]byte("ERROR: Timeout waiting for list response"))
		}
	} else if strings.HasPrefix(cmd, "CLOSE:") {
		portStr := strings.TrimPrefix(cmd, "CLOSE:")
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			_, _ = conn.Write([]byte("ERROR: Invalid port"))
			return
		}

		s := a.getBestSession()
		if s == nil {
			_, _ = conn.Write([]byte("ERROR: No active session"))
			return
		}

		err = s.Send(protocol.CloseRequest{Port: uint16(port)})
		if err != nil {
			_, _ = conn.Write([]byte("ERROR: Failed to send CloseRequest"))
			return
		}

		select {
		case resp := <-a.closeChan:
			if resp.Success {
				_, _ = conn.Write([]byte(fmt.Sprintf("Closed port %d", resp.Port)))
			} else {
				_, _ = conn.Write([]byte(fmt.Sprintf("ERROR: Failed to close port %d: %s", resp.Port, resp.Reason)))
			}
		case <-time.After(5 * time.Second):
			_, _ = conn.Write([]byte("ERROR: Timeout waiting for close response"))
		}
	} else if strings.HasPrefix(cmd, "FORWARD:") {
		arg := strings.TrimPrefix(cmd, "FORWARD:")
		var slavePort, masterPort uint16
		if strings.Contains(arg, ":") {
			parts := strings.Split(arg, ":")
			s_port, _ := strconv.ParseUint(parts[0], 10, 16)
			m_port, _ := strconv.ParseUint(parts[1], 10, 16)
			slavePort = uint16(s_port)
			masterPort = uint16(m_port)
		} else {
			p, _ := strconv.ParseUint(arg, 10, 16)
			slavePort = uint16(p)
			masterPort = uint16(p)
		}

		if slavePort == 0 || masterPort == 0 {
			_, _ = conn.Write([]byte("ERROR: Invalid port mapping"))
			return
		}

		s := a.getBestSession()
		if s == nil {
			_, _ = conn.Write([]byte("ERROR: No active session"))
			return
		}

		localAddr := fmt.Sprintf(":%d", masterPort)
		remoteHost := "localhost"

		log.Info().Uint16("slave", slavePort).Uint16("master", masterPort).Msg("Requesting listen from daemon")
		err = s.Send(protocol.ListenRequest{
			LocalAddr:  localAddr,
			RemoteHost: remoteHost,
			RemotePort: slavePort,
		})
		if err != nil {
			log.Error().Err(err).Msg("Failed to send ListenRequest")
			_, _ = conn.Write([]byte("ERROR: Failed to send ListenRequest"))
			return
		}

		select {
		case resp := <-a.listenChan:
			if resp.Success {
				_, _ = conn.Write([]byte(fmt.Sprintf("Forwarding started: slave %d -> master %d", slavePort, masterPort)))
			} else {
				_, _ = conn.Write([]byte(fmt.Sprintf("ERROR: Failed to start forwarding: %s", resp.Reason)))
			}
		case <-time.After(5 * time.Second):
			_, _ = conn.Write([]byte("ERROR: Timeout waiting for listen response"))
		}
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
