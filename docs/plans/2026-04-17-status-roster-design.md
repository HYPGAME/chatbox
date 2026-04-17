# Status Roster Design

## Goal

Extend `/status` so every participant can see the current online roster, not just the host-side peer count.

## Scope

- Keep using `/status`; do not add `/who` or `/online`.
- Host and joiners should both be able to view the same current participant list.
- Do not expose internal control traffic as normal chat messages.
- Do not redesign the wire protocol with new frame types.

## Approach

Use a minimal hidden request/response flow over existing encrypted chat messages.

- Joiner-side `/status` sends a reserved internal message body to the host.
- `HostRoom` intercepts that internal request instead of broadcasting it as chat.
- The host computes a fresh participant snapshot from its authoritative member set and sends a reserved internal response back only to the requester.
- The UI intercepts that response and renders it as a muted `system` line, not as a normal chat line.
- Host-side `/status` renders local status plus the same roster text immediately, using the same formatting path.

## Output

- Local status line remains, for example:
  - `connected to alice`
  - `hosting on 0.0.0.0:7331 (2 peers)`
- Roster line is appended as:
  - `online (3): alice, bob, carol`

Names are sorted for stable output. The count includes the host plus all currently connected joiners.

## Expected Result

- `/status` becomes the single place to inspect room presence.
- Joiners no longer need host-only visibility to see who is online.
- Internal status messages do not pollute transcript history as chat content.
