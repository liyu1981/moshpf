# mpf: Mosh Port Forwarding

`mpf` is a lightweight wrapper for `mosh` that adds persistent and dynamic TCP port forwarding. It solves the limitation of `mosh` not supporting SSH-style port forwarding (`-L`) while maintaining the robustness of the `mosh` session.

## Features

- **Persistent Port Forwarding**: Port forwards are saved to `~/.mpf/forwards.json` and automatically restored across sessions.
- **Dynamic Forwarding**: Add or remove port forwards on-the-fly without restarting your session.
- **Reliable Tunnel**: Uses a multiplexed `yamux` tunnel with heartbeat monitoring and automatic reconnection.
- **Self-Deploying**: Automatically deploys the agent binary to the remote host.
- **Seamless Integration**: Hand-off control to the system `mosh` binary while maintaining the tunnel in the background.

## Installation

```bash
go build -o mpf ./cmd/moshpf
```

Ensure the resulting `mpf` binary is in your `PATH`.

## Usage

### 1. Start a Mosh Session
Start a session just like you would with `mosh`. `mpf` will establish the tunnel and then execute `mosh`.

```bash
mpf mosh user@hostname
```

### 2. Manage Port Forwards
In a separate terminal (on the client side), use the `forward`, `list`, and `close` commands. These commands communicate with the active `mpf` agent via a Unix socket.

**Forward a port:**
```bash
mpf forward 8080
```
*Note: This currently assumes `localhost:8080` on the master maps to `localhost:8080` on the slave.*

**List active forwards:**
```bash
mpf list
```
Output format: `<port> <status>`

**Close a forward:**
```bash
mpf close 8080
```

## How It Works

1. **Bootstrap**: `mpf` connects via SSH, ensures the `mpf` agent is present on the remote host, and starts it.
2. **Tunneling**: A `yamux` session is established over the SSH-started agent's stdin/stdout.
3. **Mosh Handover**: `mpf` forks and executes the system `mosh` binary.
4. **Supervision**: The `mpf` parent process remains running to manage the tunnel and listeners. It monitors the connection with heartbeats.
5. **Reconnection**: If the tunnel drops (e.g., network change), `mpf` automatically re-establishes the SSH connection and tunnel in the background without affecting the `mosh` session.
6. **Persistence**: Requested ports are stored in `~/.mpf/forwards.json` and are restored whenever you connect to that specific `user@host` again.

## Requirements

- `mosh` (client and server)
- SSH access to the remote host
- Go 1.21+ (for building)
