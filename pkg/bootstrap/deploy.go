package bootstrap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"golang.org/x/crypto/ssh"
)

func DeployAgent(client *ssh.Client, remotePath string, force bool) (string, error) {
	if strings.HasPrefix(remotePath, "~/") {
		// Basic home expansion
		remotePath = ".local/bin/mpf" // Assume relative to home if it starts with ~/
	}

	shouldDeploy := force

	if !shouldDeploy {
		// Check version
		session, err := client.NewSession()
		if err != nil {
			return "", err
		}
		defer session.Close()

		var b bytes.Buffer
		session.Stdout = &b
		// Try to get version. If it fails, we assume it's not installed or broken.
		err = session.Run(fmt.Sprintf("%s version", remotePath))
		installedVersion := strings.TrimSpace(b.String())

		if err != nil || installedVersion != protocol.Version {
			shouldDeploy = true
		}
	}

	if shouldDeploy {
		log.Info().Str("version", protocol.Version).Msg("Deploying mpf to remote")
		if err := uploadBinary(client, remotePath); err != nil {
			return "", fmt.Errorf("failed to upload binary: %v", err)
		}
	}

	return remotePath, nil
}

func uploadBinary(client *ssh.Client, remotePath string) error {
	// For now, we'll try to find the current running binary and upload it.
	// In a real release, we would use embedded cross-compiled assets.
	selfPath, err := os.Executable()
	if err != nil {
		return err
	}

	f, err := os.Open(selfPath)
	if err != nil {
		return err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	go func() {
		w, _ := session.StdinPipe()
		defer w.Close()
		fmt.Fprintf(w, "C%04o %d %s\n", 0755, stat.Size(), filepath.Base(remotePath))
		io.Copy(w, f)
		fmt.Fprint(w, "\x00")
	}()

	// Ensure the directory exists
	dir := filepath.Dir(remotePath)
	if err := session.Run(fmt.Sprintf("mkdir -p %s && scp -t %s", dir, dir)); err != nil {
		return err
	}

	return nil
}
