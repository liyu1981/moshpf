package mosh

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

func Run(args []string, isDev bool) error {
	if isDev {
		fmt.Println(">>> Dev mode active. Mosh will not be started.")
		fmt.Println(">>> Type 'exit' to terminate the session.")
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == "exit" {
				return nil
			}
		}
		return scanner.Err()
	}

	// args should be the original args passed to mpf minus our flags
	// For now, let's assume all remaining args are for mosh.

	moshPath, err := exec.LookPath("mosh")
	if err != nil {
		return fmt.Errorf("mosh not found: %v", err)
	}

	log.Info().Str("path", moshPath).Strs("args", args).Msg("Starting mosh")
	cmd := exec.Command(moshPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start mosh: %v", err)
	}

	// Signal handling
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		if cmd.Process != nil {
			cmd.Process.Signal(sig)
		}
	}()

	return cmd.Wait()
}
