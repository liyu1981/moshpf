package bootstrap

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/liyu1981/moshpf/pkg/constant"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

func DeployAgent(client *ssh.Client, remotePath string, force bool) (string, error) {
	if after, ok := strings.CutPrefix(remotePath, "~/"); ok {
		remotePath = after
	}

	shouldDeploy := force

	if shouldDeploy {
		// In dev/force mode, try to stop the existing agent first to avoid "text file busy"
		session, err := client.NewSession()
		if err == nil {
			_ = session.Run(fmt.Sprintf("./%s stop", remotePath))
			session.Close()
		}
	}

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
		err = session.Run(fmt.Sprintf("./%s version", remotePath))
		installedVersion := strings.TrimSpace(b.String())

		if err != nil || !strings.Contains(installedVersion, constant.Version) {
			shouldDeploy = true
		}
	}

	if shouldDeploy {
		log.Info().Str("version", constant.Version).Msg("Deploying mpf to remote")
		remoteArch, err := getRemoteArch(client)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get remote architecture, assuming match and trying to upload")
			if err := uploadBinary(client, remotePath); err != nil {
				return "", fmt.Errorf("failed to upload binary: %v", err)
			}
		} else if runtime.GOOS == "linux" && isArchMatch(remoteArch) {
			if err := uploadBinary(client, remotePath); err != nil {
				return "", fmt.Errorf("failed to upload binary: %v", err)
			}
		} else {
			log.Info().Str("remote_arch", remoteArch).Str("local_arch", runtime.GOARCH).Str("local_os", runtime.GOOS).Msg("Architecture or OS mismatch, falling back to download")
			if err := downloadBinary(client, remotePath, remoteArch); err != nil {
				return "", fmt.Errorf("failed to download binary: %v", err)
			}
		}
	}

	return remotePath, nil
}

func getRemoteArch(client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var b bytes.Buffer
	session.Stdout = &b
	if err := session.Run("uname -m"); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

func isArchMatch(remoteArch string) bool {
	localArch := runtime.GOARCH
	switch remoteArch {
	case "x86_64":
		return localArch == "amd64"
	case "aarch64":
		return localArch == "arm64"
	case "armv7l", "armv6l":
		return localArch == "arm"
	case "i386", "i686":
		return localArch == "386"
	}
	return remoteArch == localArch
}

func downloadBinary(client *ssh.Client, remotePath string, remoteArch string) error {
	if constant.Version == "dev" {
		return fmt.Errorf("architecture mismatch (%s vs %s) and cannot download 'dev' version", remoteArch, runtime.GOARCH)
	}

	// Map remote arch to what we use in release filenames
	releaseArch := remoteArch
	switch remoteArch {
	case "x86_64":
		releaseArch = "amd64"
	case "aarch64":
		releaseArch = "arm64"
	case "armv7l", "armv6l":
		releaseArch = "arm"
	case "i386", "i686":
		releaseArch = "386"
	}

	version := "v" + constant.Version
	filename := fmt.Sprintf("mpf-%s-linux-%s.tar.gz", version, releaseArch)
	url := fmt.Sprintf("%s/releases/download/%s/%s", constant.Github, version, filename)

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	log.Info().Str("url", url).Msg("Downloading binary to remote")
	dir := filepath.Dir(remotePath)
	// Try wget then curl. -L for curl to follow redirects.
	cmd := fmt.Sprintf("mkdir -p %s && (wget -qO- %s || curl -L %s) | tar -xzC %s",
		dir, url, url, dir)

	return session.Run(cmd)
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
