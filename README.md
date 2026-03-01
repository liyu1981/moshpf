# mpf: Mosh Port Forwarding

`mpf` is a tool created for myself (hopefull also for you) to solve the frequent question `how to do port forwarding with mosh?`. It is inspired by vscode's port forwarding feature, act as a lightweight wrapper for `mosh`, and adds persistent and dynamic TCP port forwarding. It will also try to maintain a mobile friendly port forwarding connection.

## Demo

![mpf Demo](demo/mpf_demo.gif)

## Features

- **Persistent Port Forwarding**: Port forwards are saved to `~/.mpf/forwards.json` and automatically restored across sessions.
- **Dynamic Forwarding**: Add or remove port forwards on-the-fly without restarting your session.
- **Reliable Tunnel**: Uses a multiplexed `yamux` tunnel with heartbeat monitoring and automatic reconnection.
- **QUIC Support**: High-performance transport with automatic fallback to TCP.
- **Self-Deploying**: Automatically deploys the agent binary to the remote host.
- **Seamless Integration**: Hand-off control to the system `mosh` binary while maintaining the tunnel in the background.

## Installation

### 1. Via npx (Recommended)
You can run `mpf` directly using `npx` without manual installation:
```bash
npx @liyu1981/mpf mosh user@hostname
```

### 2. From Source (Go)
If you have Go installed (1.25+), you can install it via:
```bash
go install github.com/liyu1981/moshpf/cmd/moshpf@latest
# The binary will be named 'moshpf', you might want to alias it to 'mpf'
```
Alternatively, clone the repo and build:
```bash
git clone https://github.com/liyu1981/moshpf.git
cd moshpf
go build -o mpf ./cmd/moshpf/main.go
```

### 3. Binary Download
Download the pre-compiled binaries from the [Releases](https://github.com/liyu1981/moshpf/releases) page.

## Supported Platforms

`mpf` supports the following platforms and architectures:
- **Linux**: amd64, arm64
- **macOS (Darwin)**: amd64, arm64

## Usage

### 1. Start a Mosh Session
Start a session just like you would with `mosh`. `mpf` will establish the tunnel and then execute `mosh`.

```bash
mpf mosh user@hostname
```

#### Connection Transport
`mpf` establishes two types of connections for the tunnel:
1. **TCP (Yamux)**: A reliable control and data tunnel established over SSH.
2. **QUIC (Optional)**: A high-performance UDP-based tunnel that provides better latency and migration support.

By default, `mpf` attempts to establish a QUIC connection and falls back to TCP if it fails. You can control this behavior with flags:
- `--quic`: Force QUIC transport only.
- `--tcp`: Force TCP transport only.

### 2. Manage Port Forwards
In a separate terminal (on the client side), use the `forward`, `list`, and `close` commands. These commands communicate with the active `mpf` agent via a Unix socket.

**Forward a port:**
```bash
mpf forward 8080
# or with explicit mapping (slave:master)
mpf forward 20000:8080
```
*Note: `mpf forward 20000:8080` means the master (local) listens on port 8080 and forwards to the slave (remote) port 20000.*

**List active forwards:**
```bash
mpf list
```

**Close a forward:**
```bash
mpf close 8080
```

## How It Works

1. **Bootstrap**: `mpf` connects via SSH, ensures the `mpf` agent is present on the remote host, and starts it.
2. **Tunneling**: A `yamux` session is established over the SSH-started agent's stdin/stdout.
3. **Mosh Handover**: `mpf` executes the system `mosh` binary.
4. **Supervision**: The `mpf` parent process remains running to manage the tunnel and listeners. It monitors the connection with heartbeats.
5. **Reconnection**: If the tunnel drops, `mpf` automatically re-establishes the connection in the background.
6. **Persistence**: Requested ports are stored in `~/.mpf/forwards.json` and are restored whenever you connect to that specific `user@host` again.

## Requirements

- `mosh` (client and server)
- SSH access to the remote host
- Go 1.25+ (if building from source)

## CI/CD

This project uses GitHub Actions for continuous integration. Every push and pull request to the `master` branch triggers the unit test suite to ensure stability and correctness.

[![Test Status](https://github.com/liyu1981/moshpf/actions/workflows/test.yml/badge.svg)](https://github.com/liyu1981/moshpf/actions/workflows/test.yml)

## License

GPLv3. See [LICENSE](LICENSE) for details.
