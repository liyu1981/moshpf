
# mpf: Mosh with Port Forwarding

A Go-based wrapper for `mosh` that adds SSH-style port forwarding capabilities by establishing a tunneled daemon-agent relationship before handing over control to `mosh`.

## 1. Architecture Overview

The system consists of a single Go binary that functions in two modes: **Local Daemon** (the wrapper) and **Remote Agent** (the tunnel endpoint). The binary is cross-compiled for common targets and self-transfers to the remote host on first use.

- **Local Daemon:** Initiates the SSH connection, deploys/starts the agent, manages local port listeners, monitors the mosh process, and tears down the tunnel on exit.
- **Remote Agent:** Runs on the server, accepts multiplexed streams from the local daemon, and proxies them to local services on the remote machine.

## 2. Technical Strategy

### A. Connection & Bootstrapping

1. **SSH Client:** Use `golang.org/x/crypto/ssh` to establish the initial secure connection.
2. **Agent Injection:** The local daemon checks for a `mpf` binary on the remote host (at `~/.local/bin/mpf` by default) by comparing an embedded version string. If missing or outdated, it transfers the correct architecture's binary (selected from embedded cross-compiled assets) via an SCP-like copy over the SSH connection. The transfer is followed by a checksum verification step before the agent is started.
3. **Tunneling:** A stream multiplexer (`hashicorp/yamux`) runs over the stdin/stdout of the remotely-executed agent process, providing a single high-performance backbone for all forwarded ports and the control channel.

### B. Control Protocol

A lightweight, explicitly-versioned framing protocol sits on top of yamux. All messages are length-prefixed and encoded with `encoding/gob` (or msgpack if performance warrants it). Stream 0 is reserved as the **control channel**; all other streams are data channels for forwarded connections.

Defined message types:

| Message | Direction | Purpose |
|---|---|---|
| `Hello` / `HelloAck` | Both | Version handshake on startup; daemon aborts if versions are incompatible |
| `ForwardRequest` | Daemon → Agent | Ask the agent to dial a given `host:port` |
| `ForwardAck` / `ForwardErr` | Agent → Daemon | Confirm connection or report failure |
| `Heartbeat` / `HeartbeatAck` | Both | Sent every 10s; three missed replies triggers teardown |
| `Shutdown` | Both | Graceful teardown signal in either direction |

This protocol is defined and stabilized before Phase 3 begins.

### C. Port Forwarding Logic

1. Local daemon listens on `localhost:PORT` for each `-L` flag.
2. On an incoming connection, it opens a new yamux stream and sends a `ForwardRequest` on the control channel.
3. The Remote Agent dials the target `host:port` on the server side and replies with `ForwardAck`.
4. Data is bidirectionally proxied through the yamux stream via `io.Copy`.
5. If the agent fails to dial, it replies with `ForwardErr` and the local listener closes that connection cleanly.

### D. Mosh Handover & Daemon Lifecycle

1. Once the tunnel is established and all port listeners are confirmed up, the local daemon forks.
2. **The Child:** Calls `syscall.Exec` to replace itself with the system `mosh` binary. From this point the child *is* mosh — none of the daemon's code continues running in it.
3. **The Parent:** Records the child's PID and enters a supervision loop. It calls `wait4` (via `syscall.Wait4`) to detect mosh exit, including abnormal termination. On exit, it sends a `Shutdown` message to the remote agent and closes the yamux session.
4. **Signal Handling (built into Phase 4, not deferred):** The parent traps `SIGINT` and `SIGTERM`. On receipt, it forwards the signal to the mosh child, waits for it to exit, then performs the same tunnel teardown. This is not optional polish — it is part of the core lifecycle.

## 3. Project Structure

```
moshpf/
├── cmd/
│   └── moshpf/         # Entry point: parses mode flag, dispatches to daemon or agent
├── internal/
│   ├── bootstrap/      # SSH client, binary transfer, version check, agent startup
│   ├── protocol/       # Control message types, framing, encode/decode
│   ├── tunnel/         # Yamux session management, heartbeat, stream dispatch
│   ├── forward/        # TCP listeners, proxy logic, ForwardRequest flow
│   ├── agent/          # Remote agent main loop (receives streams, dials targets)
│   └── mosh/           # Fork, syscall.Exec handover, wait4 supervision loop
└── go.mod
```

The previous `ssh/` and agent deployment split has been merged into `bootstrap/` to reflect that they are tightly coupled in practice. `protocol/` is promoted to a first-class package because the message definitions are shared by both daemon and agent.

## 4. Implementation Phases

### Phase 1 — Bootstrap

Implement SSH connection, cross-compiled binary embedding, remote version check, binary transfer with checksum verification, and remote execution in agent mode. Validate on Linux/amd64 and Linux/arm64 targets before proceeding.

### Phase 2 — Control Protocol & Multiplexing

Define and implement all message types in `protocol/`. Integrate yamux. Implement the heartbeat loop and `Shutdown` handshake. Write unit tests for the framing layer against a loopback yamux session.

### Phase 3 — Port Forwarding

Build TCP listeners and the proxy logic. Wire up `ForwardRequest` / `ForwardAck` / `ForwardErr`. Test with real local services (e.g. forward a remote PostgreSQL port).

### Phase 4 — Mosh Integration & Lifecycle

Implement the fork, `syscall.Exec` handover, `wait4` supervision, and signal handling. Ensure the tunnel tears down cleanly on normal exit, mosh crash, and `SIGINT`/`SIGTERM`.

### Phase 5 — UX & Hardening

CLI argument parsing (`-L`, `--ssh-opt`, `--remote-binary-path`), user-facing error messages, remote agent cleanup on stale PIDs, and logging/debug mode.
