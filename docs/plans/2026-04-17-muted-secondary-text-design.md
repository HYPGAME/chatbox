# Muted Secondary Text Design

## Goal

Reduce visual weight for timestamps and `system`/`error` lines without changing sender colors or message-body readability.

## Scope

- Keep sender-name colors as-is.
- Keep message body brightness as-is.
- Do not change top status text, `history:` hint, or input border styling.
- Only adjust:
  - message timestamps
  - `system` lines
  - `error` lines

## Approach

Use dedicated muted lipgloss styles for secondary text instead of relying on the current default/plain rendering.

- Timestamp gets its own muted gray-blue style and is rendered separately from sender/body text.
- `system` lines get a dim neutral style so they remain readable but less dominant than chat messages.
- `error` lines keep a red semantic, but shift to a darker, less saturated red than the current bright error style.

## Expected Result

- Message rows visually emphasize sender and body, not the timestamp.
- `system` and `error` lines remain distinguishable, but stop competing with actual chat content.
- Existing message semantics and tests remain stable after stripping ANSI sequences.
