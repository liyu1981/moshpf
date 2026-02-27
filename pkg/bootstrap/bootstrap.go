package bootstrap

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/liyu1981/moshpf/pkg/forward"
	"github.com/liyu1981/moshpf/pkg/mosh"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/state"
	"github.com/liyu1981/moshpf/pkg/tunnel"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog/log"
)

func Run(args []string, remoteBinaryPath string, isDev bool) error {
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

		backoff := 1 * time.Second
		for {
			err := runSession(target, remoteBinaryPath, isDev, fwd)
			if err != nil {
				log.Error().Err(err).Msg("Session failed, reconnecting...")
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				continue
			}
			// If runSession returns nil, it means graceful shutdown or deliberate exit
			return
		}
	}()

	// Run mosh in child
	return mosh.Run(args, isDev)
}

func runSession(target string, remoteBinaryPath string, isDev bool, fwd *forward.Forwarder) error {
	client, err := Connect(target)
	if err != nil {
		return fmt.Errorf("failed to connect: %v", err)
	}
	defer client.Close()

	remotePath, err := DeployAgent(client, remoteBinaryPath, isDev)
	if err != nil {
		return fmt.Errorf("failed to deploy agent: %v", err)
	}
	log.Info().Str("path", remotePath).Msg("Remote binary found/deployed")

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

	go io.Copy(os.Stderr, stderr)

	agentCmd := fmt.Sprintf("%s agent", remotePath)
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
	fwd.AddSession(tSession)

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

	sessions := make(map[*tunnel.Session]chan struct{})
	var sessionsMu sync.Mutex
	errChan := make(chan error, 1)
	remoteHostname := fwd.GetRemoteName()

	addSession := func(s *tunnel.Session) {
		sessionsMu.Lock()
		defer sessionsMu.Unlock()
		stop := make(chan struct{})
		sessions[s] = stop
		go func() {
			s.StartHeartbeat(stop)
		}()
		go func() {
			for {
				msg, err := s.Receive()
				if err != nil {
					sessionsMu.Lock()
					if _, ok := sessions[s]; ok {
						delete(sessions, s)
						fwd.RemoveSession(s)
						close(stop)
					}
					isPrimary := (len(sessions) == 0)
					sessionsMu.Unlock()
					if isPrimary {
						errChan <- err
					}
					return
				}

				log.Debug().Type("type", msg).Msg("Master received message")

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
					return
				}
			}
		}()
	}

	addSession(tSession)

	// Attempt QUIC if available
	if ack.UDPPort > 0 && ack.TLSHash != "" {
		go func() {
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

			qConn, err := quic.DialAddr(ctx, fmt.Sprintf("%s:%d", remoteHost, ack.UDPPort), tlsConf, nil)
			if err != nil {
				log.Warn().Err(err).Msg("QUIC upgrade failed, staying on TCP")
				return
			}

			qSession, err := tunnel.NewQuicSession(qConn, false)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create QUIC session")
				qConn.CloseWithError(0, "")
				return
			}

			log.Info().Msg("QUIC upgrade successful")
			fwd.AddSession(qSession)
			addSession(qSession)
		}()
	}

	select {
	case err := <-errChan:
		sessionsMu.Lock()
		for s, stop := range sessions {
			close(stop)
			s.Mux.Close()
		}
		sessionsMu.Unlock()
		return err
	}
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
