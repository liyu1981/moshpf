package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/user/moshpf/pkg/agent"
	"github.com/user/moshpf/pkg/bootstrap"
	"github.com/user/moshpf/pkg/logger"
	"github.com/user/moshpf/pkg/protocol"
)

type forwardFlag []string

func (f *forwardFlag) String() string {
	return fmt.Sprint(*f)
}

func (f *forwardFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func main() {
	logger.Init()
	var forwards forwardFlag
	flag.Var(&forwards, "L", "Port forward: <port>")
	isAgent := flag.Bool("agent", false, "Run in agent mode (internal use)")
	showVersion := flag.Bool("version", false, "Show version and exit")
	verbose := flag.Bool("v", false, "Verbose output")
	remotePath := flag.String("remote-path", "~/.local/bin/moshpf", "Path to moshpf on remote host")
	flag.Parse()

	isDev := os.Getenv("APP_ENV") == "dev"

	if *showVersion {
		fmt.Println(protocol.Version)
		return
	}

	if *isAgent {
		if err := agent.Run(*verbose); err != nil {
			fmt.Fprintf(os.Stderr, "Agent error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if len(flag.Args()) == 0 && len(forwards) > 0 {
		if err := sendToAgent(forwards); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send request to agent: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Daemon mode
	if err := bootstrap.Run(flag.Args(), *remotePath, *verbose, isDev); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
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
