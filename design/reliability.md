# mpf Reliability & Persistence Design

This document outlines the strategy for improving the connection reliability between the master (local daemon) and slave (remote agent) and ensuring port forwards persist across sessions.

## 1. Heartbeat Timeout Mechanism

Currently, the heartbeat only detects transmission failures. We will implement a "missed reply" tracking system.

- **Tracking:** The `tunnel.Session` will maintain a `lastAckReceived` timestamp or a `missedHeartbeats` counter (reset on any valid message or specifically on `HeartbeatAck`).
- **Interval:** Heartbeats are sent every 10 seconds.
- **Threshold:** If 3 consecutive heartbeats (30 seconds) go unacknowledged, the session is considered "stale" or "down."
- **Action:**
    - The master will initiate a reconnection attempt.
    - The slave will exit (it should already exit if the master closes the connection, but this provides a secondary safety).

## 2. Persistent Port Forwards

To ensure port forwards survive crashes or deliberate restarts, we will persist the state on the **master (local daemon)** side.

- **Storage:** A JSON file located at `~/.mpf/forwards.json`.
- **Structure:**
  ```json
  {
    "remotes": {
      "user@host": {
        "forwards": [
            "8080",
            "3210"
        ]
      }
    }
  }
  ```
- **Lifecycle:**
    - **Add:** When a `ListenRequest` is successfully handled (or even when requested), it's added to the persistent store.
    - **Remove:** When a `CloseRequest` is processed, it's removed.
    - **Cleanup:** Entries are scoped to the remote host to avoid conflicts.

## 3. Reconnection Strategy

When the master detects a connection failure (via heartbeat timeout or TCP error):

1. **Backoff Loop:** The master enters a retry loop with exponential backoff (e.g., 1s, 2s, 4s... up to 30s).
2. **Re-bootstrap:**
    - Re-establish SSH connection.
    - Deploy/Verify agent binary.
    - Start the remote agent.
    - Establish Yamux tunnel and Handshake.
3. **State Restoration:**
    - Read `~/.mpf/forwards.json` for the current remote.
    - For each recorded forward, the master re-establishes the local listener.
    - Note: Since the *agent* usually initiates the request via the Unix socket in the current implementation, we need a way for the *master* to proactively restore these or for the *agent* to re-request them.
    - **Change:** The master owns the source of truth for "desired forwards" for a given session. Upon reconnection, it simply re-opens the listeners. No message to the agent is strictly needed until a connection actually arrives at a local listener (which triggers a `ForwardRequest`).

## 4. Implementation Plan

### Step 1: Heartbeat Refactor
- Update `tunnel.Session` to track health.
- Modify `StartHeartbeat` to check for timeouts.

### Step 2: Persistence Layer
- Create `pkg/state` package to manage `forwards.json`.
- Integrate with `Forwarder` to save/load state.

### Step 3: Reconnection Loop
- Refactor `bootstrap.Run` to wrap the session logic in a retryable loop.
- Ensure the `mosh` process (the child) remains unaffected while the parent (master) reconnects the tunnel in the background.
