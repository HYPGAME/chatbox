# Minimal Group Chat Design

## Goal

Extend `chatbox` from single-peer chat to a minimal multi-user room model that is easy to test locally: one host, multiple joiners, all messages broadcast to the whole room.

## Product Decision

- Topology: single host, multiple joiners
- Host role: room relay and broadcaster
- Join role: unchanged point-to-host client
- CLI surface: keep existing `host` and `join` commands
- Message format: unchanged
- Delivery semantics: sender receipt still means "host accepted the message"
- UI additions:
  - show `joined` / `left` system lines
  - show host peer count in status
- History model: store one transcript per room key instead of per single peer

## Why This Design

The user wants multi-user chat mainly to make testing easier. The shortest path is not a new protocol or mesh network. It is a host-side relay that reuses the existing encrypted point-to-point session layer.

This keeps the risky parts small:

- no handshake redesign
- no new frame types
- no NAT or peer discovery work
- no new CLI concepts

It also keeps future options open. If the project later needs richer rooms, member lists, or offline replay, those can be layered on top of a host-side room abstraction instead of being baked into the transport now.

## Architecture

### Session Layer

`internal/session.Session` remains a single encrypted connection between exactly two peers. Its responsibilities do not change.

### Host-Side Room Layer

Add a small host-side room/broker abstraction above `Session`:

- accept inbound sessions from `Host`
- track connected members
- receive messages from each member session
- broadcast messages to all other members
- emit host-visible events for:
  - member joined
  - member left
  - inbound chat message
  - local host send

The host should treat its own local messages the same way as relayed joiner messages so that the UI only has one broadcast path.

### Join-Side Model

Joiners keep their current structure:

- one connection to the host
- existing reconnect behavior
- existing send/receipt behavior

No joiner ever talks directly to another joiner.

## Message Flow

### Joiner Message

1. Joiner sends a message to host.
2. Host session returns receipt to the sender exactly as today.
3. Host broker receives the message event.
4. Host broker broadcasts the same message to:
   - the host's local view
   - every other connected joiner
5. Broadcast copies preserve the original `From` field.

### Host Local Message

1. Host user types a message locally.
2. Host broker creates a room message using the host name.
3. Host local view renders it.
4. Host broker broadcasts it to all connected joiners.

### Join / Leave

When a member connects or disconnects, the host broker emits system events:

- `aaa joined`
- `aaa left`

These are for room visibility and testability only. They are not protocol-level control messages and do not need message IDs or receipts.

## Reliability Semantics

Keep semantics intentionally minimal:

- sender receipt means host accepted the message
- no per-recipient delivery status
- no end-to-end group ACK aggregation
- no offline replay for members who disconnect
- reconnect logic remains only between a joiner and the host

This preserves the current mental model and avoids inventing false guarantees the system cannot yet uphold.

## UI Design

### Shared Rendering

Continue rendering chat messages as:

- `[time] name: body`

Continue rendering system messages separately.

### Host Status

Host status should become room-oriented, for example:

- `hosting on 0.0.0.0:7331 (0 peers)`
- `hosting on 0.0.0.0:7331 (3 peers)`

### Join Status

Join status remains connection-oriented:

- `connected to host-name`

No member list sidebar, no `/who`, and no new room commands in the minimal version.

## Transcript Design

The current transcript store keys history by `(localName, peerName, psk)`, which fits one-to-one chat but fragments room history across members.

For group chat, transcript identity should move to a room key:

- host side: a stable room key derived from host mode and listen address
- join side: a stable room key derived from host target address

This keeps one room history per group conversation without changing the encrypted file format itself.

## Testing Strategy

### Session / Broker Tests

Add multi-member host tests covering:

- two joiners connect to one host
- joiner A sends a message and joiner B receives it
- host local send reaches every joiner
- join and leave events are emitted
- sender receipts remain host-scoped

### UI Tests

Add minimal UI tests covering:

- host status includes peer count
- `joined` / `left` system lines render correctly
- relayed messages preserve original sender name

### Transcript Tests

Add tests covering:

- room transcript key selection for host mode
- room transcript key selection for join mode
- group history stays in one transcript instead of per-member shards

## Non-Goals

- mesh networking
- private messages
- room naming and room discovery
- member list commands
- offline catch-up or missed-message replay
- member-level receipt state
- server persistence beyond current transcript behavior
