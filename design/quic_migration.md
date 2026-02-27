# QUIC Migration Plan for moshpf

This document outlines the strategy for migrating the `moshpf` tunnel from TCP/Yamux over SSH-stdio to UDP/QUIC.

## 1. Architectural Overview

The current system relies on an SSH session to transport data via `stdin/stdout`, multiplexed by Yamux. The migration introduces a UDP-based transport using QUIC, providing better performance on lossy networks and support for connection migration.

### Current State (Hybrid)
- **Transport**: Initially SSH-stdio (TCP).
- **Multiplexer**: Yamux (on SSH-stdio) and Native QUIC streams (after upgrade).
- **Control Channel**: Stream 0 of Yamux session or the first bidirectional QUIC stream.
- **Data Streams**: Dynamic streams opened per connection.
- **Promotion**: The session starts on SSH-stdio and "upgrades" to QUIC if a UDP connection can be established.

### Target State
- **Transport**: QUIC (UDP) as primary, SSH-stdio (Yamux) as fallback.
- **Multiplexer**: Native QUIC streams.
- **Trust**: SSH-bootstrapped TLS (fingerprint pinning).

---

## 2. Refactoring Phases

### Phase 1: Multiplexer Abstraction (Completed)
`tunnel.Session` and `forward.Forwarder` are now abstracted via the `Multiplexer` interface.
- `YamuxMultiplexer` and `QuicMultiplexer` implementations exist.
- `tunnel.NewSession` and `tunnel.NewQuicSession` handle initialization.

### Phase 2: TLS and Certificate Management (Completed)
QUIC requires TLS 1.3. We use ephemeral self-signed certificates.
- `pkg/tunnel/tls.go` implements ephemeral X.509 generation and fingerprinting.
- Fingerprint verification is performed via `VerifyPeerCertificate` to ensure pinning.

### Phase 3: Bootstrap and Handshake Evolution (Completed)
The bootstrap process coordinates the UDP port and TLS fingerprint.
- **Agent**: Listens on a random UDP port (62000-63000) and sends `UDPPort` and `TLSHash` in `HelloAck`.
- **Client**: Receives `HelloAck` over SSH, then attempts to dial QUIC in the background.
- **Promotion**: If QUIC succeeds, the `forward.Forwarder` and `bootstrap` loops switch to the QUIC session.

### Phase 4: Forwarding Logic Adjustment (In Progress)
The mapping between `ForwardRequest` and data streams must be deterministic even with high concurrency.

**The Problem:**
Currently, `ForwardRequest` is sent over the control channel (Stream 0), and the data stream is opened separately. In a multi-stream environment like QUIC or Yamux, if two requests are sent nearly simultaneously, the Agent might call `AcceptStream()` and receive the stream for Request B while it was expecting the stream for Request A.

**The Solution: Self-Describing Streams**
Instead of using the control channel to announce a new connection, we will transition to self-describing data streams.

**Action Items:**
1.  **Refactor Client (`Forwarder.handleConnection`)**:
    - Open a new stream using `Mux.OpenStream()`.
    - Write a `StreamHeader` (binary encoded or `gob`) to the beginning of the stream.
    - The header contains `Host` and `Port`.
    - Wait for a 1-byte success/fail acknowledgement from the Agent on the same stream before beginning `io.Copy`.
2.  **Refactor Agent (`Agent.Run`)**:
    - Add a background loop that continuously calls `Mux.AcceptStream()`.
    - For each accepted stream, read the `StreamHeader`.
    - Dial the target host:port.
    - Write a 1-byte status back to the stream.
    - If successful, start the `io.Copy` loop.
3.  **Deprecate `ForwardRequest`**:
    - `ForwardRequest`, `ForwardAck`, and `ForwardErr` will be removed from the control protocol as they are no longer needed. This simplifies the control loop and reduces latency (eliminates one control-plane round trip).

**Stream Header Format (Draft):**
```go
type StreamHeader struct {
    Host string
    Port uint16
}
```
Using `gob` encoding for the header is consistent with the rest of the project, though a fixed-size binary header would be slightly more efficient. Given the existing use of `gob`, we will stick with it for now.

### Phase 5: Verification and Testing (New)
The project currently lacks automated tests.

**Action Items:**
- Implement unit tests for `pkg/tunnel` (both Yamux and QUIC).
- Implement integration tests for the full bootstrap -> forward flow using a mock SSH server or local-only mode.

### Phase 6: Robustness and Fallback
- **Hybrid Multi-Streaming**: The system will maintain both the SSH-stdio (Yamux) and the QUIC transport simultaneously.
- **Path Selection**: 
    - The control plane will prefer QUIC if available but keep the SSH-stdio control stream as a "warm standby."
    - New data streams will always attempt QUIC first. If QUIC dialing fails or times out, the `Forwarder` will transparently fall back to `Yamux.Open()`.
- **Heartbeat Coordination**: Heartbeats will be sent on both transports. If QUIC heartbeats fail but SSH-stdio succeeds, the session stays alive but "downgrades" to TCP-only until a new QUIC handshake succeeds.

## 7. Operational Impacts

### Firewall Requirements
- **Inbound UDP**: The remote server must allow inbound UDP traffic on the port range used (default 62000-63000). 
- **Cloud Security Groups**: Users on AWS/GCP/Azure will need to manually open these ports in their security groups/firewalls.
- **DPI**: Some corporate firewalls may drop QUIC packets. The "Warm Fallback" to SSH-stdio ensures the tool remains functional, albeit with higher latency/HOL blocking.

### Performance Expectations
- **Connection Migration**: Users on unstable connections (e.g., roaming between base stations) should experience fewer interruptions.
- **CPU Usage**: QUIC can be more CPU-intensive than TCP/Yamux due to the userspace implementation of the transport and crypto. This is negligible for typical port-forwarding loads.

### Diagnostics
- The `ListResponse` (returned by `mpf list`) should be updated to show which transport each active forward is currently using (TCP vs QUIC).

### Phase 7: Optimization (New)
- **0-RTT Handshake**: To achieve near-instantaneous reconnection, the client will cache QUIC session tokens and transport parameters.
- **Persistence**:
    - **Location**: `~/.mpf/quic_cache.json`
    - **Content**: Map of `remote_host` to `{session_token, transport_params, last_working_port}`.
    - **Security**: The cache contains ephemeral session tickets. While not as sensitive as private keys, the file should be restricted to user-only access (0600).

---

## 8. Implementation Checklist

- [x] Abstract Multiplexer interface.
- [x] Implement Yamux/QUIC multiplexers.
- [x] Implement ephemeral TLS cert generation.
- [x] Implement fingerprint pinning.
- [x] Basic QUIC upgrade logic in bootstrap.
- [x] Refactor data streams to be self-describing (Remove `ForwardRequest`).
- [x] Implement parallel transport health monitoring (Hybrid mode).
- [ ] Add 0-RTT caching.
- [ ] Update CLI to show transport status.

---

## 3. Component Changes

### `pkg/protocol`
- Added `UDPPort` and `TLSHash` to `HelloAck`.

### `pkg/tunnel`
- `Multiplexer` interface defined.
- `tls.go` added for ephemeral certs.

### `pkg/agent`
- Starts QUIC listener and handles session promotion.

### `pkg/bootstrap`
- Implements background QUIC dialing and session swapping.

---

## 4. Security Considerations

- **Fingerprint Pinning**: The TLS fingerprint is transmitted over the authenticated SSH channel, achieving MITM protection.
- **ALPN**: Using `moshpf-0` to identify the protocol.
- **Ephemeral Keys**: Keys are never stored on disk, reducing the impact of a potential server compromise.

## 5. Performance and Reliability

- **Native Heartbeats**: QUIC's native keep-alives are used, supplemented by application-level heartbeats for session health monitoring.
- **Head-of-Line Blocking**: Resolved by using independent QUIC streams for each forwarded connection.
