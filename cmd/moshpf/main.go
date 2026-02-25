package main

import (
	"fmt"
	"net"
	"os"

	"github.com/user/moshpf/pkg/agent"
	"github.com/user/moshpf/pkg/bootstrap"
	"github.com/user/moshpf/pkg/logger"
	"github.com/user/moshpf/pkg/protocol"
)

func main() {
	logger.Init()
	
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	isDev := os.Getenv("APP_ENV") == "dev"

	switch os.Args[1] {
	case "version":
		fmt.Println(protocol.Version)
	case "agent":
		if err := agent.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
			os.Exit(1)
		}
	case "forward":
		if len(os.Args) < 3 {
			fmt.Println("Usage: moshpf forward <port>")
			os.Exit(1)
		}
		if err := sendToAgent(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send request to agent: %v\n", err)
			os.Exit(1)
		}
	case "mosh":
		if len(os.Args) < 3 {
			fmt.Println("Usage: moshpf mosh [user@]host")
			os.Exit(1)
		}
		// Default remote path
		remotePath := "~/.local/bin/moshpf"
		if err := bootstrap.Run(os.Args[2:], remotePath, isDev); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Usage: moshpf <command> [args]")
	fmt.Println("Commands:")
	fmt.Println("  agent           Run in agent mode (internal use)")
	fmt.Println("  forward <port>  Request port forward from an active session")
	fmt.Println("  mosh <args>     Start a mosh session with port forwarding")
	fmt.Println("  version         Show version")
}

func sendToAgent(forwards []string) error {
	sockPath := protocol.GetUnixSocketPath()
	
	for _, f := range forwards {
		conn, err := net.Dial("unix", sockPath)
		if err != nil {
			return fmt.Errorf("could not connect to agent at %s: %v", sockPath, err)
		}
		_, err = conn.Write([]byte(f))
		conn.Close()
		if err != nil {
			return err
		}
		fmt.Printf("Forward request for %s sent to agent\n", f)
	}
	return nil
}
