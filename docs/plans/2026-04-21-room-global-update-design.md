# Room-Scoped Global Update Design

## Goal

Allow a running chat room to trigger a coordinated client update for all connected `join` peers.

The trigger may come from:

- the current `host`
- a `join` peer whose `identity_id` is explicitly allowed by the current `host`

The update target may be:

- a specific release tag such as `v0.1.24`
- the latest formal GitHub release when no version is specified

`join` peers should update silently when possible, verify checksums, replace the local binary, and restart themselves automatically. `host` should not auto-update as part of this flow.

## Scope

In scope:

- room-scoped update request and execution control messages
- `host`-side authorization using a local admin whitelist file
- a user command such as `/update-all [version]`
- `join`-side silent update and automatic restart
- host-visible execution results and summary
- compatibility with existing clients so unknown control messages are ignored rather than crashing

Out of scope:

- updating every running `chatbox` instance on all devices outside the current room
- auto-updating `host` as part of the same command
- mandatory minimum-version enforcement during handshake
- Android auto-update and auto-restart

## User Experience

### Command

Both `host` and `join` support:

```text
/update-all
/update-all v0.1.24
```

Behavior:

- if no version is supplied, `host` resolves the latest formal release tag before execution
- if a version is supplied, `host` validates that it looks like `vMAJOR.MINOR.PATCH`
- the command does not directly cause clients to update when entered by a `join`; it sends an update request to `host`

### Result Visibility

The initiator receives clear system feedback:

- request accepted
- permission denied
- invalid version
- host failed to resolve latest release

The room sees concise host-generated progress and summary lines, for example:

```text
system [time]: update request accepted: v0.1.24
system [time]: update result: alice success
system [time]: update result: bob already-up-to-date
system [time]: update result: carol fallback-written
system [time]: update summary: success=1 up-to-date=1 fallback=1 failed=0
```

## Authorization Model

`host` is the only authority that can approve room-wide updates.

Rules:

- `host` can always trigger the update
- `join` can trigger only if its `identity_id` is present in the host-local whitelist
- `join` clients never trust update execution orders from other `join` peers
- `join` clients trust execution orders only when they arrive through the room and identify the current `host` as sender

This keeps configuration centralized on `host` and avoids distributing room admin policy to every client.

## Configuration

### Host Admin Whitelist File

Path:

```text
~/.config/chatbox/admins.json
```

Shape:

```json
{
  "allowed_update_identities": [
    "identity-abc",
    "identity-def"
  ]
}
```

Rules:

- file is optional
- missing file means only `host` may trigger `/update-all`
- invalid file causes the command to fail closed with a host-local error message
- matching is based only on `identity_id`, never on display name

## Control Flow

### 1. Initiation

When a user enters `/update-all [version]`:

- if running on `host`, the request is evaluated locally
- if running on `join`, the client sends a hidden room control message to `host`

The request contains:

- protocol version for this control family
- optional target version
- requester identity
- requester display name for logs only
- request ID
- timestamp

### 2. Host Validation

`host` performs:

- identity lookup for the requester
- whitelist authorization
- target version validation
- latest release resolution if target version is omitted
- deduplication by request ID

If validation fails, `host` responds to the requester with a hidden result message that renders as a readable system line.

### 3. Execution Broadcast

On success, `host` broadcasts a hidden execution control message containing:

- request ID
- approved target version
- initiator identity
- initiator display name
- approval timestamp

### 4. Join Execution

Each `join` client:

- ignores duplicate request IDs
- compares target version with local version
- skips execution when already on the same or newer version
- performs the existing verified self-update flow
- auto-restarts only if in-place replacement succeeds
- reports a result message back to `host`

### 5. Host Summary

`host` collects result messages for the current request and emits readable status lines and a summary.

The summary window can be best-effort rather than perfect. For example:

- emit each result immediately
- keep counters for the active request
- print a final summary when the room becomes quiet for a short period or when a timeout expires

## Update Execution Rules

### Platform Rules

- `darwin` and `linux` `join` clients may auto-update and auto-restart
- `android` must not attempt automatic self-update because the current updater explicitly rejects it

Android result should be reported as a manual-action state such as:

- `android-manual-required`

### Restart Rules

Automatic restart applies only to `join`.

Restart behavior:

- preserve the original executable path
- preserve the original CLI arguments used to start the current `join` process
- close the current session gracefully before restart
- launch the new binary with the preserved arguments

If the updater falls back to writing `chatbox.new` instead of replacing the live binary:

- do not auto-restart
- report `fallback-written`
- keep the current process alive

## Control Message Types

New hidden control families are needed, separate from history sync and revoke messages.

The prefix family is:

```text
\x00chatbox:update:
```

The concrete message kinds are:

- `request`: `join` or `host` asks `host` to start a room update
- `execute`: `host` instructs all `join` clients to update to a specific version
- `result`: `join` reports outcome back to `host`

Each message should include:

- a small version field for this control schema
- request ID for deduplication
- room key for scoping

Execution and result messages should also include:

- target version
- actor identity fields needed for validation and logs

## Compatibility

Backward compatibility requirement:

- old clients must not crash when they receive the new hidden update control messages

Expected behavior for old clients:

- they ignore the unknown control message body and continue running
- they do not auto-update
- they may remain on the old version until manually updated

This means room-wide update is best-effort across mixed versions, which is acceptable for the first version of this feature.

## Failure Handling

Possible result states:

- `success`
- `already-up-to-date`
- `permission-denied`
- `invalid-version`
- `resolve-latest-failed`
- `download-failed`
- `checksum-failed`
- `extract-failed`
- `replace-failed`
- `fallback-written`
- `restart-failed`
- `android-manual-required`

Rules:

- `host` should log short readable lines, not raw stack traces
- local detailed errors can still be written to stderr or local logs when available
- a single peer failure must not abort the rest of the room update

## Testing Strategy

### Unit Tests

- update control payload round-trip tests
- whitelist loading and authorization tests
- command parsing tests for `/update-all`
- host request validation tests
- join execution dispatch tests
- restart decision tests:
  - in-place replacement triggers restart
  - fallback path does not trigger restart
  - android reports manual action

### Integration Tests

- authorized `join` triggers request and `host` broadcasts execution
- unauthorized `join` is denied
- `host` triggers update directly
- duplicate request IDs execute only once per client
- mixed-version room ignores unknown control messages safely
- host summary reflects received result states

## Rollout Notes

This design intentionally does not combine global update with protocol-level version enforcement.

Recommended rollout:

1. ship room-scoped manual global update first
2. observe behavior on mixed-client rooms
3. only then consider adding minimum-version enforcement if still needed
