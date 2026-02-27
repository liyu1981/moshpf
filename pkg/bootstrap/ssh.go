package bootstrap

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"syscall"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"net"

	"github.com/rs/zerolog/log"
)

func createSSHConfig(username string) (*ssh.ClientConfig, error) {
	var authMethods []ssh.AuthMethod

	// Try SSH agent
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			// Note: we don't close conn because the agent client signers might need it
			authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}

	// Try default keys
	if home, err := os.UserHomeDir(); err == nil {
		var signers []ssh.Signer
		for _, name := range []string{"id_ed25519", "id_rsa", "id_ecdsa", "id_dsa"} {
			path := filepath.Join(home, ".ssh", name)
			if key, err := os.ReadFile(path); err == nil {
				signer, err := ssh.ParsePrivateKey(key)
				if err == nil {
					signers = append(signers, signer)
				}
			}
		}
		if len(signers) > 0 {
			authMethods = append(authMethods, ssh.PublicKeys(signers...))
		}
	}

	// Always allow password fallback
	authMethods = append(authMethods, ssh.PasswordCallback(func() (string, error) {
		fmt.Printf("Password for %s: ", username)
		password, err := term.ReadPassword(int(syscall.Stdin))
		fmt.Println() // Print newline after password entry
		if err != nil {
			return "", err
		}
		return string(password), nil
	}))

	return &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // For now, should be improved later
	}, nil
}

func Connect(target string) (*ssh.Client, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}

	username := u.Username
	host := target
	// Simple user@host parsing
	for i := 0; i < len(target); i++ {
		if target[i] == '@' {
			username = target[:i]
			host = target[i+1:]
			break
		}
	}

	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "22")
	}

	config, err := createSSHConfig(username)
	if err != nil {
		return nil, err
	}

	log.Info().Str("host", host).Str("user", username).Msg("Connecting to SSH host")
	return ssh.Dial("tcp", host, config)
}

func GetRemoteUDPBufferInfo(client *ssh.Client) (int, int, error) {
	run := func(cmd string) (int, error) {
		session, err := client.NewSession()
		if err != nil {
			return 0, err
		}
		defer session.Close()
		out, err := session.Output(cmd)
		if err != nil {
			return 0, err
		}
		var val int
		_, err = fmt.Sscanf(string(out), "%d", &val)
		return val, err
	}

	rmem, err := run("sysctl -n net.core.rmem_max")
	if err != nil {
		return 0, 0, err
	}
	wmem, err := run("sysctl -n net.core.wmem_max")
	if err != nil {
		return 0, 0, err
	}

	return rmem, wmem, nil
}
