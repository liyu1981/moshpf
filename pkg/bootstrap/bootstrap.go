package bootstrap

import (
	"fmt"
	"io"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/liyu1981/moshpf/pkg/forward"
	"github.com/liyu1981/moshpf/pkg/mosh"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/tunnel"
)

func Run(args []string, remoteBinaryPath string, isDev bool) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: mpf mosh [user@]host")
	}

	target := args[0]
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

	log.Info().Str("path", remotePath).Msg("Starting remote agent")
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

	if err := tSession.Send(protocol.Hello{Version: protocol.Version}); err != nil {
		return err
	}

	msg, err := tSession.Receive()
	if err != nil {
		return err
	}

	if ack, ok := msg.(protocol.HelloAck); !ok || ack.Version != protocol.Version {
		return fmt.Errorf("failed handshake or version mismatch")
	}

	log.Info().Msg("Tunnel established")

	remoteHostname := target
	for i := 0; i < len(target); i++ {
		if target[i] == '@' {
			remoteHostname = target[i+1:]
			break
		}
	}

	fwd := forward.NewForwarder(tSession, remoteHostname)

	go func() {
		for {
			msg, err := tSession.Receive()
			if err != nil {
				if err != io.EOF {
					log.Error().Err(err).Msg("Session receive error")
				}
				return
			}

			switch m := msg.(type) {
			case protocol.Heartbeat:
				_ = tSession.Send(protocol.HeartbeatAck{})
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
				_ = tSession.Send(resp)
			case protocol.ListRequest:
				_ = tSession.Send(protocol.ListResponse{
					Entries: fwd.GetForwardEntries(),
				})
			case protocol.CloseRequest:
				log.Info().
					Str("remote", remoteHostname).
					Uint16("port", m.Port).
					Msg("Close request received")
				success := fwd.CloseForward(m.Port)
				_ = tSession.Send(protocol.CloseResponse{
					Port:    m.Port,
					Success: success,
				})
			case protocol.Shutdown:
				return
			}
		}
	}()

	stopHeartbeat := make(chan struct{})
	go tSession.StartHeartbeat(stopHeartbeat)
	defer close(stopHeartbeat)

	// Phase 4: Run mosh
	return mosh.Run(args, isDev)
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
