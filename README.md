# mpf: Mosh Port Forwarding

`mpf` is a tool designed to solve a common problem: "How can I use port forwarding with Mosh?" Inspired by VS Code's port forwarding feature, `mpf` acts as a lightweight wrapper for Mosh, adding persistent and dynamic TCP port forwarding. It also maintains a mobile-friendly connection for your forwarded ports.

## Demo

![mpf Demo](demo/mpf_demo.gif)

## Features

- **Persistent Port Forwarding**: Port forwards are saved to `~/.mpf/forwards.json` and automatically restored across sessions.
- **Auto Forwarding**: Automatically monitors and forwards newly opened ports.
- **Dynamic Forwarding**: Add or remove port forwards on-the-fly without restarting your session.
- **Reliable Tunnel**: High-performance UDP transport with great resilience (powered by QUIC) and automatic fallback to TCP.

## Use & Installation

### 1. Via npx (Recommended)
Run `mpf` directly using `npx` without manual installation:
```bash
npx @liyu1981/mpf mosh user@hostname
```

### 2. Via Shell Script
Install `mpf` to `~/.local/bin` using the following command:
```bash
curl -fsSL https://raw.githubusercontent.com/liyu1981/moshpf/main/install.sh | bash
```

### 3. Binary Download
Download pre-compiled binaries from the [Releases](https://github.com/liyu1981/moshpf/releases) page.

### 4. From Source (Go)
If you have Go 1.25+ installed, you can install it via:
```bash
go install github.com/liyu1981/moshpf/cmd/moshpf@latest
# The binary will be named 'moshpf'; you may want to alias it to 'mpf'
```
Alternatively, clone the repository and build it:
```bash
git clone https://github.com/liyu1981/moshpf.git
cd moshpf
go build -o mpf ./cmd/moshpf/main.go
```

## Supported Platforms

`mpf` supports the following platforms and architectures:
- **Linux**: amd64, arm64
- **macOS (Darwin)**: arm64

__Note:__ only Linux with amd64 architecture is tested in real usage and others are built with golang's cross compilation. There may be issue in those platforms I do not know. If you find an issue please create an issue or a PR. 

## Basic Usage

Basic usage is as simple as prefixing `mpf` to your usual Mosh commands, as shown in the example below:

```bash
mpf mosh user@hostname -- tmux
```

Most of the time, this is all you will need.

## Advanced Usage Options

### Manage Port Forwards

On the remote side (the machine you joined via Mosh), use the `forward`, `list`, and `close` subcommands to manage port forwards.

**Forward a port:**
```bash
mpf forward 8080
# or with explicit mapping (remote:local)
mpf forward 20000:8080
```
*Note: `mpf forward 20000:8080` means the local machine listens on port 8080 and forwards to the remote port 20000.*

**List active forwards:**
```bash
mpf list
```

**Close a forward:**
```bash
mpf close 8080
```

### Choose `QUIC` or `TCP` Transport

`mpf` establishes two types of connections for the tunnel:

1. **QUIC**: A high-performance UDP-based tunnel providing superior support for mobile connections.
2. **TCP**: A reliable control and data tunnel established over SSH.

By default, `mpf` attempts to establish a QUIC connection and falls back to TCP if it fails. You can control this behavior with flags:
- `--quic`: Force QUIC transport only.
- `--tcp`: Force TCP transport only.


## Architecture

1. **Bootstrap**: `mpf` connects via SSH, ensures the `mpf` agent is present on the remote host, and starts it.
2. **Tunneling**: A QUIC or Yamux session is established using the SSH-started agent's stdin/stdout.
3. **Mosh Handover**: `mpf` executes the system `mosh` binary.
4. **Supervision**: The `mpf` parent process remains running to manage the tunnel and listeners, monitoring the connection with heartbeats.
5. **Reconnection**: If the tunnel drops, `mpf` automatically re-establishes the connection in the background.
6. **Persistence**: Requested ports are stored in `~/.mpf/forwards.json` and are restored whenever you reconnect to that specific `user@host`.

## Requirements

- `mosh` (client and server)
- SSH access to the remote host (SSH keys recommended)
- Go 1.25+ (if building from source)

## License

GPLv3. See [LICENSE](LICENSE) for details.
