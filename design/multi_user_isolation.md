# Design: Multi-User Isolation (Option B)

## Problem Statement
The current `moshpf` agent implementation uses fixed system-wide resources, which causes conflicts when multiple users attempt to use the tool on the same remote machine:
1. **Unix Socket Conflict**: All users try to bind to `/tmp/moshpf.sock`, leading to "socket hijacking" where one user's CLI commands interact with another user's agent.
2. **QUIC Port Conflict**: Fixed or overlapping UDP ports prevent multiple agents from establishing QUIC sessions simultaneously.
3. **TCP Forwarding Conflict**: Multiple users attempting to forward the same local port (e.g., `:8080`) will clash, as only one process can bind to a TCP port at a time.

## Proposed Solution: Isolated Agents
We will implement **Option B**, where each user runs their own completely isolated agent process. This model mirrors standard `ssh -R` behavior and provides the best security and reliability.

### 1. User-Scoped Unix Sockets
To isolate CLI interactions (like `moshpf list`), the agent's control socket will be moved to a user-specific path.
- **New Path**: `/tmp/mpf-$UID.sock` (where `$UID` is the user's numeric ID).
- **Benefit**: The `moshpf` CLI tool, running as the same user, will automatically calculate the correct path based on its own UID, ensuring it only talks to its own agent.
- **Constraint**: If `/tmp` is not writable or the socket cannot be created, the agent will report an error and exit. No fallback to home directory.

### 2. Randomized QUIC UDP Ports
The agent must find an available UDP port for the QUIC transport without requiring coordination from the master or other agents.
- **Port Selection Logic**:
    1. Define the range: `62000-63000`.
    2. The agent will attempt to bind to a random port within this range.
    3. If the bind fails (port in use), it will continuously retry with a new random port until successful.
    4. **No fallback** to system-assigned ports (port 0).
- **Discovery**: The agent will report the successfully bound port back to the master process via the `HelloAck` message on the initial TCP control session.

### 3. Master-Agent Handshake Update
The master process needs to learn the agent's dynamically assigned UDP port to establish the QUIC session.
1. **Bootstrap**: Master starts Agent via SSH (TCP connection established).
2. **Hello**: Master sends `protocol.Hello`.
3. **HelloAck**: Agent responds with `protocol.HelloAck`, which **must** now include the `UDPPort` it successfully bound to.
4. **QUIC Upgrade**: Master receives `HelloAck`, extracts the port, and initiates the QUIC connection to the agent.

### 4. Same-User Reconnection Conflict Resolution
To prevent accidental socket hijacking, the agent performs a pre-handshake check before initializing its control listeners.

#### Conflict Detection Phase
1. **Probe**: New agent attempts to connect to `/tmp/mpf-$UID.sock`.
2. **Gather**: If a connection is established, it sends a `LIST` command to retrieve active forwardings.
3. **Warn**: It prints the current agent's status and active tunnels to `stderr`.
4. **Prompt**: It prints an interactive prompt to `stderr` and waits for a single-character input from `stdin`:
   - `[C]ontinue`: Start mosh session without new port forwardings (Passive Mode).
   - `[K]ill`: Stop the existing agent and take over its role (Fresh Start).
   - `[A]bort`: Exit immediately.

#### Operational Modes
- **Active Mode (Default)**: Agent owns the Unix socket and manages the QUIC listener for high-performance transport.
- **Passive Mode**: Agent skips Unix socket and QUIC initialization. It only provides the `mosh-server` lifecycle management for the current terminal session. Existing tunnels from the "Active" agent remain operational and visible.

### 5. Slave-Side CLI (`moshpf list`, `moshpf stop`)
Currently, these commands assume a singleton agent.
- **Resolution**: Update the `protocol.GetUnixSocketPath()` utility to include the current user's UID in the filename.
- **Result**: When User A runs `moshpf list`, the tool connects to `/tmp/moshpf-1000.sock`. When User B runs it, it connects to `/tmp/moshpf-1001.sock`. Each user sees only their own forwarded ports.

### 5. TCP Port Forwarding Conflicts
This is an inherent limitation of networking (one port per IP).
- **Behavior**: If two users try to forward `:8080`, the second user's agent will fail to bind and return an error message to the master.
- **User UX**: The master should display a clear error: `Error: Remote port 8080 is already in use by another process.`

## Implementation Steps
1. Update `pkg/protocol/util.go` (or equivalent) to generate UID-based socket paths.
2. Modify `pkg/agent/agent.go` to implement the randomized UDP binding loop.
3. Ensure `protocol.HelloAck` correctly carries the dynamically assigned port.
4. Update `pkg/bootstrap/bootstrap.go` to use the port returned in `HelloAck` for the QUIC connection attempt.
5. Verify the 10-minute idle timeout still works correctly for these isolated processes.
