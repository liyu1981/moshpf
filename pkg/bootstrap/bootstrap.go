package bootstrap

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/liyu1981/moshpf/pkg/forward"
	"github.com/liyu1981/moshpf/pkg/logger"
	"github.com/liyu1981/moshpf/pkg/mosh"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/state"
	"github.com/liyu1981/moshpf/pkg/tunnel"
	"github.com/liyu1981/moshpf/pkg/util"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

func waitEnterOrEsc() error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil
	}

	state, err := term.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer term.Restore(fd, state)

	buf := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			return err
		}
		if buf[0] == '\r' || buf[0] == '\n' {
			fmt.Print("\r\n")
			return nil
		}
		if buf[0] == 0x1b { // Esc
			fmt.Print("\r\nAborted by user\r\n")
			return fmt.Errorf("aborted")
		}
	}
}

type TransportMode string

const (
	TransportModeFallback TransportMode = "fallback"
	TransportModeQUIC     TransportMode = "quic"
	TransportModeTCP      TransportMode = "tcp"
)

func Run(args []string, remoteBinaryPath string, isDev bool, mode TransportMode) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mpf mosh [user@]host")
	}

	target := args[0]

	stateMgr, err := state.NewManager()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize state manager")
	}

	remoteHostname := target
	for i := 0; i < len(target); i++ {
		if target[i] == '@' {
			remoteHostname = target[i+1:]
			break
		}
	}

	// 1. Initial Local Check
	if mode != TransportModeTCP {
		localBuf, err := tunnel.GetUDPBufferInfo()
		if err == nil {
			if warn := tunnel.GetBufferWarning("local", localBuf); warn != "" {
				fmt.Print(warn)
				if err := waitEnterOrEsc(); err != nil {
					return nil // Graceful exit
				}
			}
		}
	}

	// 2. Initial Remote Check & Deployment (Synchronous)
	client, err := Connect(target)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}

	remotePath, err := DeployAgent(client, remoteBinaryPath, isDev)
	if err != nil {
		client.Close()
		return fmt.Errorf("failed to deploy agent: %v", err)
	}

	if mode != TransportModeTCP {
		rmem, wmem, err := GetRemoteUDPBufferInfo(client)
		if err == nil {
			remoteBuf := tunnel.UDPBufferInfo{RMemMax: rmem, WMemMax: wmem}
			if warn := tunnel.GetBufferWarning("remote", remoteBuf); warn != "" {
				fmt.Print(warn)
				if err := waitEnterOrEsc(); err != nil {
					return nil // Graceful exit
				}
			}
		}
	}

	fwd := forward.NewForwarder(nil, remoteHostname, stateMgr, target)

	// Start the session for port forwarding
	go func() {
		// Initial restore from state
		if stateMgr != nil {
			for mStr, sStr := range stateMgr.GetForwards(target) {
				var mPort, sPort uint16
				fmt.Sscanf(mStr, "%d", &mPort)
				fmt.Sscanf(sStr, "%d", &sPort)
				if mPort > 0 && sPort > 0 {
					_ = fwd.ListenAndForward(fmt.Sprintf(":%d", mPort), "localhost", sPort)
				}
			}
		}

		// Initial session using the already established client
		err := runSessionWithClient(client, remotePath, target, fwd, mode)
		if err != nil {
			log.Error().Err(err).Msg("Initial session failed, reconnecting...")
		}

		backoff := 1 * time.Second
		for {
			err := runSession(target, remoteBinaryPath, isDev, fwd, mode)
			if err != nil {
				log.Error().Err(err).Msg("Session failed, reconnecting...")
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				continue
			}
			return
		}
	}()

	// Run mosh in child
	return mosh.Run(args, isDev)
}

func runSession(target string, remoteBinaryPath string, isDev bool, fwd *forward.Forwarder, mode TransportMode) error {
	client, err := Connect(target)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer client.Close()

	remotePath, err := DeployAgent(client, remoteBinaryPath, isDev)
	if err != nil {
		return fmt.Errorf("failed to deploy agent: %v", err)
	}
	return runSessionWithClient(client, remotePath, target, fwd, mode)
}

func runSessionWithClient(client *ssh.Client, remotePath, target string, fwd *forward.Forwarder, mode TransportMode) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Error().Msgf("Remote agent error: %s", scanner.Text())
		}
	}()

	agentCmd := fmt.Sprintf("./%s agent", remotePath)
	if util.IsDev() {
		if err := session.Setenv("APP_ENV", "dev"); err != nil {
			log.Error().Msgf("Set APP_ENV=dev for remote failed: %s", err.Error())
		}
	}
	if err := session.Start(agentCmd); err != nil {
		return err
	}

	conn := &sessionConn{
		stdin:  stdin,
		stdout: stdout,
	}

	tSession, err := tunnel.NewSession(conn, false)
	if err != nil {
		return err
	}

	if err := tSession.Send(protocol.Hello{Version: protocol.Version}); err != nil {
		return err
	}

	msg, err := tSession.Receive()
	if err != nil {
		return err
	}

	ack, ok := msg.(protocol.HelloAck)
	if !ok || ack.Version != protocol.Version {
		return fmt.Errorf("failed handshake or version mismatch")
	}

	log.Info().Msg("Tunnel established")

	errChan := make(chan error, 1)
	remoteHostname := fwd.GetRemoteName()

	var startControlLoop func(s *tunnel.Session)
	startControlLoop = func(s *tunnel.Session) {
		fwd.GetSessions().Add(s, func() {
			if fwd.GetSessions().Count() == 0 {
				errChan <- fmt.Errorf("all sessions closed")
			}
		})

		go func() {
			for {
				msg, err := s.Receive()
				if err != nil {
					fwd.RemoveSession(s)
					return
				}

				log.Debug().Type("type", msg).Msg("Master received message")

				if stop := handleMasterMessage(s, msg, fwd, remoteHostname, errChan); stop {
					return
				}
			}
		}()
	}

	startControlLoop(tSession)

	// Attempt QUIC if available and mode allows it
	if mode != TransportModeTCP && ack.UDPPort > 0 && ack.TLSHash != "" {
		go func() {
			if err := attemptQUICUpgrade(target, ack, fwd, mode, startControlLoop, tSession); err != nil {
				if mode == TransportModeQUIC {
					errChan <- err
				}
			}
		}()
	} else if mode == TransportModeQUIC {
		// Agent didn't offer QUIC
		errChan <- fmt.Errorf("remote agent does not support QUIC")
	}

	return <-errChan
}

func handleMasterMessage(s *tunnel.Session, msg protocol.Message, fwd *forward.Forwarder, remoteHostname string, errChan chan error) bool {
	switch m := msg.(type) {
	case protocol.Heartbeat:
		_ = s.Send(protocol.HeartbeatAck{})
	case protocol.HeartbeatAck:
		// OK
	case protocol.ListenRequest:
		log.Info().
			Str("local", m.LocalAddr).
			Str("remote", fmt.Sprintf("%s:%d", remoteHostname, m.RemotePort)).
			Msg("Dynamic listen request received")
		err := fwd.ListenAndForward(m.LocalAddr, m.RemoteHost, m.RemotePort)
		resp := protocol.ListenResponse{
			RemotePort: m.RemotePort,
			Success:    err == nil,
		}
		if err != nil {
			log.Error().Err(err).Msg("Failed to handle dynamic listen request")
			resp.Reason = err.Error()
		}
		_ = s.Send(resp)
	case protocol.ListRequest:
		entries := fwd.GetForwardEntries()
		masterIP := fwd.GetMasterIP()
		err := s.Send(protocol.ListResponse{
			Entries:  entries,
			MasterIP: masterIP,
		})
		if err != nil {
			log.Error().Err(err).Msg("Master failed to send ListResponse")
		}
	case protocol.CloseRequest:
		log.Info().
			Str("remote", remoteHostname).
			Uint16("port", m.Port).
			Msg("Close request received")
		success := fwd.CloseForward(m.Port)
		_ = s.Send(protocol.CloseResponse{
			Port:    m.Port,
			Success: success,
		})
	case protocol.Shutdown:
		errChan <- nil
		return true
	}
	return false
}

func attemptQUICUpgrade(target string, ack protocol.HelloAck, fwd *forward.Forwarder, mode TransportMode, startControl func(*tunnel.Session), tSession *tunnel.Session) error {
	remoteHost := target
	if i := strings.Index(remoteHost, "@"); i != -1 {
		remoteHost = remoteHost[i+1:]
	}
	if h, _, err := net.SplitHostPort(remoteHost); err == nil {
		remoteHost = h
	}

	log.Info().Str("host", remoteHost).Uint16("port", ack.UDPPort).Msg("Attempting QUIC upgrade")
	tlsConf := tunnel.GetTLSConfigClient(ack.TLSHash)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	quicConfig := &quic.Config{
		Tracer: logger.GetQuicTracer(),
	}

	qConn, err := quic.DialAddr(ctx, fmt.Sprintf("%s:%d", remoteHost, ack.UDPPort), tlsConf, quicConfig)
	if err != nil {
		log.Warn().Err(err).Msg("QUIC upgrade failed, staying on TCP")
		return err
	}

	qSession, err := tunnel.NewQuicSession(qConn, false)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create QUIC session")
		qConn.CloseWithError(0, "")
		return err
	}

	log.Info().Msg("QUIC upgrade successful")
	startControl(qSession)

	if mode == TransportModeQUIC {
		log.Info().Msg("QUIC-only mode: closing TCP tunnel")
		fwd.RemoveSession(tSession)
	}
	return nil
}

type sessionConn struct {
	stdin  io.WriteCloser
	stdout io.Reader
}

func (c *sessionConn) Read(p []byte) (n int, err error) {
	return c.stdout.Read(p)
}

func (c *sessionConn) Write(p []byte) (n int, err error) {
	return c.stdin.Write(p)
}

func (c *sessionConn) Close() error {
	return c.stdin.Close()
}
