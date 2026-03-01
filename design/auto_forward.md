# Auto Port Forwarding

The auto port forwarding feature allows `moshpf` to automatically detect new listening ports on the remote (slave) machine and establish port forwarding for them on the local (master) machine. This is particularly useful for developers who start new services (e.g., a web server, a database) during a session and want them to be accessible locally without manual configuration.

## Design

### 1. Remote Side (Agent)
The agent (slave process) runs a background goroutine that periodically scans for new listening ports if enabled by the master. The scan interval is defined by `constant.AutoForwardScanInterval` in `pkg/constant/auto_forward.go`.

- **Scanning Mechanism:** Uses `gopsutil` to list active TCP connections and filters for those in `LISTEN` state.
- **Filtering:** 
    - Ignores ports below 1024 (system ports).
    - Ignores known infrastructure and internal services (e.g., `sshd`, `docker-proxy`, `vscode-server`). The list of excluded substrings is in `constant.AutoForwardExcludedSubstrings`.
    - Ignores the agent's own listening ports.
- **State Management:** The agent maintains a map of currently active auto-forwarded ports.
- **Triggering Forwarding:** When a new port is detected:
    1. The agent sends a `protocol.ListenRequest` to the master.
    2. The request specifies the same port number for the local listener (if available) and sets `IsAuto: true`.
- **Cleanup:** When a port is no longer detected as listening on the remote side, the agent sends a `protocol.CloseRequest` to the master to stop the forwarding and free up the local port.

### 2. Master Side (Daemon)
The master provides a flag to enable or disable this feature and handles differentiation of port types.

- **Flag:** `--no-auto-forward` (default is enabled).
- **Communication:** The master communicates this preference to the agent during the initial handshake (`protocol.Hello` message).
- **Differentiation:** The master stores whether a forward was manual or automatic in its `ForwardEntry` state. This information is used for display and can influence lifecycle decisions.

### 3. Communication Flow
1. **Master** -> `Hello {AutoForward: true/false}` -> **Agent**
2. **Agent** starts scanning goroutine if `AutoForward` is true.
3. **Agent** -> `ListenRequest {IsAuto: true}` -> **Master** (when new port found)
4. **Master** -> `ListenAndForward` (starts local listener, marks as auto)
5. **Master** -> `ListenResponse` -> **Agent** (confirmation)
6. **Agent** -> `CloseRequest` (when port is closed) -> **Master**

### 4. Consistency with `mpf list`
Auto-forwarded ports appear in `mpf list` with an "AUTO" indicator to distinguish them from manual forwards.

## Implementation Details (Snippet)

The following code snippet demonstrates the core port scanning logic that will be adapted into the agent.

```go
// portscan.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

type PortInfo struct {
	PID     int32
	IP      string
	Port    uint32
	Command string
	Exe     string
}

func main() {
	ports, err := ListListeningPorts()
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	for _, p := range ports {
		fmt.Printf("PID=%d %-15s:%-5d CMD=%s\n",
			p.PID, p.IP, p.Port, p.Command)
	}
}

func ListListeningPorts() ([]PortInfo, error) {
	conns, err := net.Connections("tcp")
	if err != nil {
		return nil, err
	}

	currentExe, _ := os.Executable()
	currentExe, _ = filepath.EvalSymlinks(currentExe)

	var results []PortInfo

	for _, c := range conns {

		// Only LISTEN sockets
		if c.Status != "LISTEN" {
			continue
		}

		// Ignore low ports (optional safety)
		if c.Laddr.Port < 1024 {
			continue
		}

		if c.Pid == 0 {
			continue
		}

		p, err := process.NewProcess(c.Pid)
		if err != nil {
			continue
		}

		cmdline, _ := p.Cmdline()
		exe, _ := p.Exe()

		if shouldExclude(cmdline, exe, currentExe) {
			continue
		}

		results = append(results, PortInfo{
			PID:     c.Pid,
			IP:      c.Laddr.IP,
			Port:    c.Laddr.Port,
			Command: cmdline,
			Exe:     exe,
		})
	}

	return results, nil
}

var excludedSubstrings = []string{
	"vscode",
	"code-server",
	"extensionhost",
	"ssh",
	"sshd",
	"docker-proxy",
	"containerd",
	"yumux", // exclude your own infra if desired
}

func shouldExclude(cmdline, exe, currentExe string) bool {

	lc := strings.ToLower(cmdline)

	// Exclude known infra by substring
	for _, s := range excludedSubstrings {
		if strings.Contains(lc, s) {
			return true
		}
	}

	// Exclude self
	exeResolved, _ := filepath.EvalSymlinks(exe)
	if exeResolved == currentExe {
		return true
	}

	return false
}
```
