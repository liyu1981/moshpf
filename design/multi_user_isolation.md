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
- **New Path**: `/tmp/moshpf-$UID.sock` (where `$UID` is the user's numeric ID).
- **Benefit**: The `moshpf` CLI tool, running as the same user, will automatically calculate the correct path based on its own UID, ensuring it only talks to its own agent.
- **Fallback**: If `/tmp` is not writable or restricted, fall back to `$HOME/.moshpf.sock`.

### 2. Randomized QUIC UDP Ports
The agent must find an available UDP port for the QUIC transport without requiring coordination from the master or other agents.
- **Port Selection Logic**:
    1. Define a preferred range (e.g., `40000-50000`).
    2. The agent will attempt to bind to a random port within this range.
    3. If the bind fails (port in use), it will retry with a new random port (up to 10 attempts).
    4. If no port is found in the range, it will fall back to port `0` to let the OS assign any available ephemeral port.
- **Discovery**: The agent will report the successfully bound port back to the master process via the `HelloAck` message on the initial TCP control session.

### 3. Master-Agent Handshake Update
The master process needs to learn the agent's dynamically assigned UDP port to establish the QUIC session.
1. **Bootstrap**: Master starts Agent via SSH (TCP connection established).
2. **Hello**: Master sends `protocol.Hello`.
3. **HelloAck**: Agent responds with `protocol.HelloAck`, which **must** now include the `UDPPort` it successfully bound to.
4. **QUIC Upgrade**: Master receives `HelloAck`, extracts the port, and initiates the QUIC connection to the agent.

### 4. Slave-Side CLI (`moshpf list`, `moshpf stop`)
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
