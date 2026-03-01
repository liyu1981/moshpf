package constant

import "time"

const (
	// AutoForwardScanInterval is the period between port scans on the agent.
	AutoForwardScanInterval = 5 * time.Second
)

var (
	// AutoForwardExcludedSubstrings are substrings in process command lines or names that should be excluded from auto-forwarding.
	AutoForwardExcludedSubstrings = []string{
		"vscode",
		"code-server",
		"extensionhost",
		"ssh",
		"sshd",
		"docker-proxy",
		"containerd",
		"mpf",
		"mosh",
	}
)
