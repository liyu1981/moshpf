package main

import (
	"fmt"
	"net"
	"os"

	"github.com/liyu1981/moshpf/pkg/agent"
	"github.com/liyu1981/moshpf/pkg/bootstrap"
	"github.com/liyu1981/moshpf/pkg/logger"
	"github.com/liyu1981/moshpf/pkg/protocol"
	"github.com/liyu1981/moshpf/pkg/util"
)

const (
	m      = "\033[35mm\033[0m" // Magenta
	p      = "\033[36mp\033[0m" // Cyan
	f      = "\033[32mf\033[0m" // Green
	mpf    = m + p + f
	github = "https://github.com/liyu1981/moshpf"
)

func main() {
	logger.Init()

	mode := bootstrap.TransportModeFallback
	var cmd string
	var cmdArgs []string

	i := 1
	for i < len(os.Args) {
		arg := os.Args[i]
		if arg == "--quic" {
			mode = bootstrap.TransportModeQUIC
			i++
			continue
		} else if arg == "--tcp" {
			mode = bootstrap.TransportModeTCP
			i++
			continue
		}

		// Not a known global flag, must be the command
		cmd = arg
		cmdArgs = os.Args[i+1:]
		break
	}

	if cmd == "" {
		printUsage()
		return
	}

	isDev := util.IsDev()

	handlers := map[string]func([]string) error{
		"version": handleVersion,
		"agent":   handleAgent,
		"forward": handleForward,
		"close":   handleClose,
		"list":    handleList,
		"stop":    handleStop,
		"mosh": func(args []string) error {
			if len(args) < 1 {
				printMoshUsage()
				os.Exit(1)
			}
			remotePath := "~/.local/bin/mpf"
			return bootstrap.Run(args, remotePath, isDev, mode)
		},
	}

	if handler, ok := handlers[cmd]; ok {
		if err := handler(cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func handleVersion(args []string) error {
	fmt.Printf("%s (%sosh %sort %sorward) - %s\n", mpf, m, p, f, github)
	fmt.Printf("version: %s\n", protocol.Version)
	return nil
}

func handleAgent(args []string) error {
	return agent.Run()
}

func handleForward(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: mpf forward <port>")
	}
	resp, err := sendToAgent("FORWARD:" + args[0])
	if err != nil {
		return err
	}
	fmt.Println(resp)
	return nil
}

func handleClose(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("Usage: mpf close <port>")
	}
	resp, err := sendToAgent("CLOSE:" + args[0])
	if err != nil {
		return err
	}
	fmt.Println(resp)
	return nil
}

func handleList(args []string) error {
	resp, err := sendToAgent("LIST")
	if err != nil {
		return err
	}
	if resp != "" {
		fmt.Println(resp)
	}
	return nil
}

func handleStop(args []string) error {
	resp, err := sendToAgent("STOP")
	if err != nil {
		return err
	}
	if resp != "" {
		fmt.Println(resp)
	}
	return nil
}

func printMoshUsage() {
	fmt.Println("Usage: mpf [flags] mosh [user@]host [more mosh args]")
}

func printUsage() {
	fmt.Printf("%s (%sosh %sort %sorward) - %s\n", mpf, m, p, f, github)
	fmt.Println("\nUsage: mpf [flags] <command> [args]")
	fmt.Println("\nFlags:")
	fmt.Println("  --quic          Use QUIC transport only")
	fmt.Println("                  (Default: try QUIC, fallback to TCP)")
	fmt.Println("  --tcp           Use TCP transport only")
	fmt.Println("                  (Default: try QUIC, fallback to TCP)")
	fmt.Println("\nCommands:")
	fmt.Println("  mosh <args>     Start a mosh session with port forwarding")
	fmt.Println("  forward <port>  Request port forward from an active session")
	fmt.Println("  close <port>    Close an active port forward")
	fmt.Println("  list            List active port forwards")
	fmt.Println("  stop            Stop the active agent")
	fmt.Println("  version         Show version")
	fmt.Println("  agent           Run in agent mode (internal use)")
}

func sendToAgent(cmd string) (string, error) {
	sockPath := protocol.GetUnixSocketPath()

	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		return "", fmt.Errorf("could not connect to agent at %s: %v", sockPath, err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cmd))
	if err != nil {
		return "", err
	}

	// Read response
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		// It's okay if there's no response for some commands
		return "", nil
	}

	return string(buf[:n]), nil
}
