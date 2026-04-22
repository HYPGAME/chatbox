# Status Version Decoupling Design

**Problem**

`/status` currently shows peer versions only after the host receives a `HistorySyncHello` payload with `client_version`. That makes version visibility depend on identity loading, room authorization, and history sync capability. A client can be fully connected on a new build and still appear as `unknown` in `/status`.

**Goal**

Make `/status` show each connected client's version reliably after connection establishment, even when history sync is unavailable or skipped.

## Approach

Introduce a dedicated lightweight control message for version advertisement.

- Join clients send a version announcement immediately after `sessionReady`, alongside existing startup control messages.
- The host records the announced version independently of history sync state.
- `/status` continues to render host and participant versions from the host-side roster, but that roster is now fed by the dedicated version control path.
- `HistorySyncHello.client_version` remains supported for backward compatibility during rollout, but it is no longer required for version display.

## Protocol

Add a new hidden room control message, separate from status and history sync controls.

- Direction: client -> host
- Payload:
  - `version`: control schema version
  - `client_version`: application version string
- Delivery:
  - Sent once per successful session bind
  - Best-effort only; on reconnect it is sent again

The host behavior:

- Accept the new control message
- Trim and store `client_version` by member ID
- Ignore malformed or empty payloads
- Keep the existing fallback that legacy peers without a version advertisement show as `unknown`

## Compatibility

Compatibility requirements:

- New clients talking to old hosts:
  - old hosts ignore the new control message
  - status behavior remains unchanged
- Old clients talking to new hosts:
  - they do not send the new control message
  - if they still send `HistorySyncHello.client_version`, new hosts continue to learn the version
  - otherwise they appear as `unknown`
- Mixed rollout:
  - no handshake or message parsing failure
  - no disconnects caused by the new control path

## Code Shape

Expected code changes:

- Add a new room control file for version advertisement parsing and formatting
- Update TUI session startup flow to send the new control message after bind
- Update host control handling to intercept and record the new control message
- Keep `rememberMemberVersion` as the single host-side write point for roster versions
- Add tests that prove `/status` shows versions without relying on history sync authorization

## Error Handling

- If version announcement send fails, do not fail the session; the client remains connected and may show as `unknown`
- If the host receives malformed control data, ignore it silently like other hidden compatibility controls
- Reconnect should resend the current version automatically

## Testing

Required coverage:

- Host records version from the new control message and includes it in `ParticipantNames`
- `/status` response shows the advertised version without any history sync prerequisites
- Join model sends the new control message after `sessionReady`
- Legacy `HistorySyncHello.client_version` still updates host-side version tracking
- Unknown peers still render as `unknown`

## Non-Goals

- Querying remote version on demand during `/status`
- Changing the human-readable `/status` output format
- Reworking identity, authorization, or history sync semantics
