# Large Message Limit Design

**Problem**

Chat messages are rejected when their encoded payload exceeds the original 4 KiB protocol limit. This is too small for pasted text and generated responses, producing `message exceeds 4096 bytes` instead of delivering the message.

**Goal**

Allow current clients to exchange messages up to 64 KiB while keeping sessions negotiated with protocol v2 or v3 on their existing 4 KiB limit.

## Approach

- Introduce protocol v4 with a 64 KiB default maximum payload.
- Keep 4 KiB as the maximum for negotiated protocol v2/v3 sessions.
- Extend handshake compatibility so a v4 host accepts v2/v3 clients.
- When a v4 client reaches a legacy host that closes the unsupported v4 handshake, reconnect once using v3.
- Clamp each session's effective maximum after negotiation; all existing send, receive, room-history chunking, and packet-allocation checks continue to use that single value.

This keeps the change in the session layer instead of duplicating text splitting in the TUI and scrollback interfaces. One submitted message remains one message with one ID, receipt, transcript record, and revoke target.

## Data Flow

1. Peers negotiate the protocol version during connection setup.
2. A v4-to-v4 session uses the configured limit, defaulting to 64 KiB.
3. Any session negotiated as v2 or v3 clamps its configured limit to 4 KiB.
4. `Session.Send` encodes the full message and validates it against the effective limit.
5. `readPacket` and the decoded-payload guard enforce the same effective limit on receipt.

## Error Handling

- Payloads above the effective limit remain rejected before a frame is written.
- The v3 fallback occurs only when the first v4 handshake ends before a server hello; authentication and other handshake failures are returned unchanged.
- A failed fallback returns the fallback connection error.
- Explicit limits smaller than the protocol default remain respected.

## Testing

- A default v4 session exchanges a message larger than 4 KiB.
- A negotiated v3 session still rejects a payload larger than 4 KiB.
- A v4 host accepts a v3 client and reports a 4 KiB effective limit.
- A v4 client reconnects to a v3 host and reports a 4 KiB effective limit.
- Existing session, room, and TUI tests remain green.

## Non-Goals

- Arbitrarily large or streaming chat messages
- Message fragmentation or reassembly
- UI character counters
- Changing attachment size limits
