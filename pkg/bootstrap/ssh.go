package bootstrap

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"net"

	"github.com/rs/zerolog/log"
)

func createSSHConfig(username string) (*ssh.ClientConfig, error) {
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	var authMethods []ssh.AuthMethod
	if err == nil {
		authMethods = append(authMethods, ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers))
	}

	// Try default keys if agent fails or is empty
	home, _ := os.UserHomeDir()
	keyPath := filepath.Join(home, ".ssh", "id_ed25519")
	if key, err := os.ReadFile(keyPath); err == nil {
		signer, err := ssh.ParsePrivateKey(key)
		if err == nil {
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH auth methods found")
	}

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
