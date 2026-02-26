package main

import (
	"fmt"
	"net"
	"os"
	
	"github.com/liyu1981/moshpf/pkg/agent"
	"github.com/liyu1981/moshpf/pkg/bootstrap"
	"github.com/liyu1981/moshpf/pkg/logger"
	"github.com/liyu1981/moshpf/pkg/protocol"
)

const Version = "dev"

func main() {
	logger.Init()
	
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	isDev := os.Getenv("APP_ENV") == "dev"

	switch os.Args[1] {
	case "version":
		fmt.Printf("mpf version %s (protocol version %s)\n", Version, protocol.Version)
	case "agent":
		if err := agent.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
			os.Exit(1)
		}
	case "forward":
		if len(os.Args) < 3 {
			fmt.Println("Usage: mpf forward <port>")
			os.Exit(1)
		}
		resp, err := sendToAgent("FORWARD:" + os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send request to agent: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(resp)
	case "close":
		if len(os.Args) < 3 {
			fmt.Println("Usage: mpf close <port>")
			os.Exit(1)
		}
		resp, err := sendToAgent("CLOSE:" + os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send request to agent: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(resp)
	case "list":
		resp, err := sendToAgent("LIST")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send request to agent: %v\n", err)
			os.Exit(1)
		}
		if resp != "" {
			fmt.Println(resp)
		}
	case "mosh":
		if len(os.Args) < 3 {
			fmt.Println("Usage: mpf mosh [user@]host")
			os.Exit(1)
		}
		// Default remote path
		remotePath := "~/.local/bin/mpf"
		if err := bootstrap.Run(os.Args[2:], remotePath, isDev); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Usage: mpf <command> [args]")
	fmt.Println("Commands:")
	fmt.Println("  agent           Run in agent mode (internal use)")
	fmt.Println("  forward <port>  Request port forward from an active session")
	fmt.Println("  close <port>    Close an active port forward")
	fmt.Println("  list            List active port forwards")
	fmt.Println("  mosh <args>     Start a mosh session with port forwarding")
	fmt.Println("  version         Show version")
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
