# Design: Master-Side Conflict Check (Revision)

*This design is a revision and fix for the conflict resolution mechanism described in [multi_user_isolation.md](./multi_user_isolation.md).*

## Motivation
The initial design for multi-user isolation placed the conflict detection logic on the slave side (the agent process itself). Moving this to the master side provides a cleaner UX by allowing the user to resolve conflicts on their local terminal before the `mosh` session starts.

## Proposed Changes

### 1. Enhanced `list` Command Output
The `moshpf list` command (and the underlying agent logic) will be updated to provide a clearer, session-oriented view of active forwardings.

-   **Header**: Each session will be preceded by a header: `Session: <master ip> -> <slave ip>`.
-   **Indentation**: All port forwarding entries belonging to a session will be indented by 2 spaces.

### 2. Master-Side Conflict Resolution Flow
The `pkg/bootstrap/bootstrap.go` will be updated to perform the following steps after establishing the initial SSH connection:

1.  **Probe**: Run `moshpf list` remotely via SSH.
2.  **Evaluate**:
    -   **No Agent Running**: If the command fails (e.g., "could not connect to agent"), no agent is running for this user. Proceed to start a new agent.
    -   **Idle Agent**: If the command succeeds but reports no active sessions (e.g., "ERROR: No active session"), the existing agent is idle.
        -   **Action**: Automatically stop the idle agent (`moshpf stop`) and proceed to start a new agent for the current session.
    -   **Active Agent (Conflict)**: If the command returns a list of active sessions.
3.  **Conflict Handling**:
    -   Display the list of active sessions and their forwardings to the user locally.
    -   **User Prompt**:
        -   `[C]ontinue`: Do **not** start a new agent process on the target machine. Proceed to start `mosh` normally. This allows the user to still use `mosh` even if they cannot use `moshpf` for new port forwardings in this session.
        -   `[A]bort`: Exit immediately.

### 3. Agent Refactoring
-   Remove the internal `checkConflict()` from `pkg/agent/agent.go`.
-   The agent remains a singleton per user on the remote machine. If it's already running and active, it continues to serve existing tunnels.

## Benefits
-   **Seamless Experience**: Idle agents are replaced automatically without user intervention.
-   **Safety**: Active agents are protected; users can either use `mosh` passively or abort, but they cannot accidentally kill another active session's agent.
-   **Improved UX**: Users see a clear view of who is connected and what ports are forwarded before making a decision.
-   **Reliability**: Interaction happens on the local terminal before the tunnel protocols or `mosh` take over.
