# Offline History Sync Design

**Goal:** Allow room members to recover encrypted chat history from other online members without storing transcripts on the router host.

## Scope

- Keep the router `host` stateless for chat history.
- Preserve the existing local encrypted transcript store.
- Let a returning user recover room history from the moment that identity first joined the room.
- Prevent newly introduced identities from receiving messages sent before they joined.
- Preserve compatibility with older clients so they continue to chat normally instead of disconnecting.

## Non-Goals

- No server-side history persistence.
- No guaranteed recovery when the only devices holding the missing history are offline.
- No automatic identity transfer across devices in the first version.
- No multi-source merge optimization in the first version.

## Constraints

Current protocol constraints shape the design:

- Handshake messages are fixed-layout and version-checked, so adding fields there would break older clients.
- Session frame types are closed over a fixed enum, so adding new encrypted frame types would also disconnect older clients.
- The only safe compatibility path is to reuse normal chat messages with reserved hidden control prefixes that new clients understand and old clients never receive unless they first opt in.

## High-Level Approach

Introduce a long-lived per-user identity file plus a hidden client-to-client sync protocol carried inside normal room messages.

Each modern client will:

1. Generate or load a stable identity.
2. Record per-room authorization metadata tying `identity_id` to the first observed join time.
3. Announce sync capability after joining a room using a hidden control message.
4. Negotiate a single sync source among other online modern clients.
5. Request only missing authorized messages.
6. Persist recovered messages into the existing encrypted local transcript store.

The router host keeps forwarding messages and remains unaware of transcripts.

## Identity Model

Add a local identity file stored beside other client config:

- Stable `identity_id`
- Public/private signing keypair or equivalent long-lived secret
- Optional display metadata for future use

Rules:

- A brand-new identity joining a room establishes `joined_at = now`.
- A restored identity imported onto a new device inherits the same `identity_id`.
- History sync is authorized only for messages whose timestamp is greater than or equal to that identity's recorded `joined_at` for the room.

This preserves the product rule:

- New person: no access to older history
- Same person on new device: access to all history after their original join

## Room Authorization Metadata

Add a local metadata store per room:

- `room_key`
- `identity_id`
- `joined_at`
- last sync bookkeeping such as summary hashes, seen ranges, or cursor state

This metadata is local to each client. It does not require host storage.

## Compatibility Strategy

Use hidden control messages over the normal data channel with a new prefix family, for example:

- `\x00chatbox:sync:hello:...`
- `\x00chatbox:sync:offer:...`
- `\x00chatbox:sync:request:...`
- `\x00chatbox:sync:chunk:...`
- `\x00chatbox:sync:done:...`

Compatibility rule:

- A client only sends sync control messages to peers that have already announced support with `sync:hello`.
- Older clients never send `sync:hello`, so they never receive sync payloads.
- Therefore:
  - new <-> new: sync enabled
  - new <-> old: chat only
  - old <-> old: unchanged

This avoids protocol breaks and avoids control-message spam on older clients.

## Sync Protocol

### 1. Capability Announcement

After a modern client joins a room and the session is ready, it emits `sync:hello` containing:

- protocol version
- `identity_id`
- room key
- local transcript summary for that room

The summary should stay small. First version can use:

- oldest authorized timestamp present
- newest timestamp present
- message count

### 2. Offer Selection

Other modern clients receiving `sync:hello` evaluate whether they can help:

- same room
- know the requester's `joined_at`
- have any messages newer than or equal to `joined_at` that the requester appears to lack

Eligible peers respond with `sync:offer`.

The requester chooses one source only in v1, preferring:

1. highest newest timestamp
2. highest message count
3. deterministic tiebreaker by identity ID

### 3. Missing History Request

The requester sends `sync:request` containing:

- selected source identity
- requester identity
- authorized lower bound (`joined_at`)
- current transcript summary

The source computes the delta and sends only missing messages.

### 4. Chunked Replay

The source emits `sync:chunk` control messages carrying compact batches of transcript records.

Requirements:

- chunk size bounded by current max message size
- deterministic ordering by message timestamp then message ID
- replay only normal chat messages, never system/error lines
- include original message ID, sender display name, body, timestamp, direction-independent metadata

### 5. Completion

The source emits `sync:done`.

The receiver:

- de-duplicates by message ID
- writes recovered records into the encrypted transcript store
- refreshes the in-memory history view if the room is open
- shows a muted system line such as `history synced: 42 messages`

## Data Storage

Reuse the existing encrypted transcript store for recovered chat messages.

Add new local encrypted-or-private metadata files for:

- identity
- room authorization metadata
- sync cursors and summaries

The first version can keep metadata as local JSON files with `0600` permissions because the transcript body already remains PSK-encrypted. If desired, metadata encryption can be added later.

## Error Handling

- If no compatible source is online, continue normal chat with no failure.
- If sync negotiation fails mid-flight, keep chat active and retry next reconnect.
- If a chunk is malformed or unauthorized, ignore the sync session and surface a local error line.
- If an incoming sync record already exists locally, skip it.
- If timestamps are inconsistent, clamp authorization using the local stored `joined_at`.

## UI Behavior

- Sync control messages are never shown as chat lines.
- Normal room usage remains unchanged.
- Optional muted system lines:
  - `history sync available`
  - `history sync in progress`
  - `history synced: N messages`
  - `history sync failed`

First version should keep these minimal to avoid noisy UI.

## Security Notes

- Possession of the room PSK alone is not enough to unlock pre-join history; the client must also possess the same long-lived identity and be authorized from its recorded `joined_at`.
- Importing an identity file effectively grants that user's history scope, which is expected and should be documented clearly.
- Because control messages travel over the existing encrypted room transport, they inherit current transport confidentiality.

## Testing Strategy

Add coverage for:

- identity creation and load
- room authorization metadata persistence
- compatibility gating so sync messages are never sent to non-supporting peers
- new identity cannot recover pre-join history
- restored identity can recover post-join history on a second device
- recovered messages persist locally and remain encrypted at rest
- duplicate replay does not create duplicate transcript entries
- sync failure does not interrupt normal chat

## Rollout Plan

Implement in small phases:

1. Identity file support
2. Room join authorization metadata
3. Hidden sync hello/offer negotiation
4. Single-source history replay
5. UI and status polish

## Decision

Proceed with a hidden control-message sync protocol, long-lived imported identities, per-room join authorization, and single-source recovery among modern clients only.
