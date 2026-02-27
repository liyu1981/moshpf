# Migrating moshpf Tunnel from TCP/Yamux to UDP/QUIC

## Context

`moshpf` is a localhost-to-remote-server port forwarder, similar in spirit to SSH port forwarding. The current implementation uses TCP with Yamux for multiplexing streams. This document summarizes a discussion about the feasibility and design of migrating the tunnel leg to QUIC (UDP-based).

---

## Is It Feasible?

Yes. QUIC (via `quic-go`) uses a similar streams-over-transport model to Yamux, so the high-level connection handling pattern — opening streams and using `io.Copy` for bidirectional data — remains largely intact. QUIC streams implement `io.ReadWriteCloser`, just like Yamux streams.

### Key Benefits of QUIC for This Use Case

- **Connection migration** — survives IP/network changes (relevant for a mosh-adjacent tool)
- **Faster reconnection** — 0-RTT or 1-RTT handshakes
- **Better performance on lossy networks** — no head-of-line blocking across streams

---

## What Needs to Change

### Component Substitution Map

| Current (TCP/Yamux) | QUIC Equivalent |
|---|---|
| `net.Listen("tcp", ...)` (tunnel side) | `quic.ListenAddr(...)` |
| `f.session.Yamux.Open()` | `quicConn.OpenStreamSync(ctx)` |
| `net.Conn` (Yamux stream) | `quic.Stream` (`io.ReadWriteCloser`) |
| No TLS required | `tls.Config` required on both ends |

### Architectural Changes Required

1. **TLS is mandatory** — QUIC bakes in TLS 1.3 at the transport layer. Both ends of the tunnel must participate in a TLS handshake.

2. **`tunnel.Session` needs refactoring** — `f.session.Yamux` is currently accessed directly. This needs to be abstracted behind an interface or replaced with a `quic.Connection`.

3. **`ForwardRequest` framing** — verify the ID-based stream correlation logic still holds. With QUIC the ordering semantics are per-stream; the signaling protocol should still work but needs validation.

4. **Local side stays TCP** — `net.Listen("tcp", ...)` on the local side remains unchanged. Only the tunnel leg (local process → remote agent) becomes QUIC/UDP. This is the correct design.

---

## TLS in Detail

### Why TLS Is Required

QUIC mandates TLS 1.3 — it is not optional. Every QUIC connection must complete a TLS handshake. For a self-managed tunnel between two controlled endpoints, this means provisioning certificates without a traditional CA.

### Step-by-Step TLS Setup

**Step 1 — Establish roles**

The remote agent acts as the QUIC server (holds the certificate). The local `moshpf` process is the QUIC client. This maps cleanly to the existing architecture.

**Step 2 — Generate a self-signed cert on the remote agent**

Two options:

- **Pre-generated at install time** — generate a cert when the agent is first set up, store at e.g. `~/.config/moshpf/agent.crt` and `agent.key`.
- **Ephemeral per-session (recommended)** — generate in-memory at agent startup using Go's `crypto/tls` + `crypto/x509`. About 30 lines of code. No files needed; fingerprint changes on restart.

**Step 3 — Communicate the fingerprint to the client**

Since there is no CA, the client needs another way to verify it's talking to the right server. Options ranked by recommendation:

1. **Pin via SSH (best)** — transmit the agent's cert fingerprint over the existing SSH channel before the QUIC connection is established. SSH is already authenticated and encrypted, so this is a clean trust bootstrap. The trust chain becomes: *"I already trust SSH to this host, therefore I trust what it told me about the QUIC cert."*

2. **Trust-on-first-use (TOFU)** — store the fingerprint in `state.Manager` after the first connection, verify on subsequent connections. Mirrors how SSH itself handles host keys.

3. **`InsecureSkipVerify: true`** — skips cert verification entirely. Acceptable if the SSH path is already the security boundary, but loses MITM protection on the QUIC leg.

**Step 4 — Client connects with TLS config**

```go
tlsConf := &tls.Config{
    InsecureSkipVerify: true, // use VerifyPeerCertificate for fingerprint pinning
    NextProtos:         []string{"moshpf-tunnel"}, // ALPN, required by quic-go
}
conn, err := quic.DialAddr(ctx, remoteAddr, tlsConf, nil)
```

For fingerprint pinning with `InsecureSkipVerify`, implement manual verification in the `VerifyPeerCertificate` callback rather than trusting blindly.

---

## Localhost-to-External-Server Considerations

### NAT Traversal

No issue for outbound connections from localhost to a remote server. NAT handles outbound UDP without problems. Issues would only arise for server-initiated connections, which is not this app's model.

### Firewalls and UDP Blocking

**This is the main real-world risk.** Many environments block or restrict UDP on non-standard ports:

- Corporate networks often whitelist only TCP/80, TCP/443, TCP/22
- Cloud VPS security groups need an explicit UDP rule for the QUIC port
- DPI appliances may drop unrecognized QUIC traffic

**Mitigation:** Ensure the remote server's firewall explicitly allows the chosen UDP port. Consider making the port configurable. If the deployment environment is reliably hostile to UDP, consider keeping TCP/Yamux as a fallback mode.

### Self-Signed Cert on an External Server

No problem. The cert does not need to be CA-signed since both ends are controlled. A transparent proxy or DPI appliance cannot intercept the TLS (it will fail the handshake), but may drop the traffic — this is a firewall concern, not a cert concern.

---

## Recommended Approach Summary

1. Remote agent generates an **ephemeral self-signed cert** in memory at startup
2. Local client fetches the **cert fingerprint over SSH** before initiating QUIC (leveraging the existing `target user@host` field)
3. Client uses `InsecureSkipVerify: true` with a manual `VerifyPeerCertificate` callback for fingerprint pinning
4. Use `NextProtos: []string{"moshpf-tunnel"}` as the ALPN identifier
5. Keep the local-side `net.Listen("tcp", ...)` unchanged — only the tunnel leg changes
6. Consider retaining TCP/Yamux as a fallback for UDP-hostile environments