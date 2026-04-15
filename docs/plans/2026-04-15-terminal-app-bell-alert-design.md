# Terminal.app Bell Alert Design

## Goal

Give `chatbox` a Terminal.app-native unread reminder that behaves like a background attention signal rather than an in-chat badge.

## Product Decision

- Target environment: `macOS + Terminal.app`
- UI scope: `scrollback` mode only
- Reminder mechanism: emit terminal bell (`\a`)
- Trigger condition: only for real-time incoming messages
- Foreground rule: only trigger when the current `chatbox` session is not the selected Terminal.app tab
- Clear rule: rely on Terminal.app to clear its own reminder when the user returns to that window/tab

## Why This Design

This project is still a CLI, not a Dock-owning native app, so a real app badge is the wrong layer. Terminal.app already has built-in badge/bounce behavior for background bell events, which is the closest match to the Codex-style reminder the user asked for. Reusing Terminal's own attention path is much simpler and more reliable than trying to synthesize a Dock badge ourselves.

## Architecture

### Alert Flow

1. `chatbox` receives a new incoming session message.
2. The scrollback UI decides whether this message is a live inbound event rather than transcript replay, ACK handling, retry marker, or system text.
3. If alert mode is enabled, `chatbox` checks whether it is currently the selected Terminal.app tab.
4. If not frontmost, `chatbox` writes `\a` to the terminal output.

### Foreground Detection

Foreground detection should be exact to the current terminal tab, not just "is Terminal.app frontmost".

The recommended mechanism is:

- capture the current session `tty` at startup
- use `osascript` to query Terminal.app:
  - whether Terminal is frontmost
  - the selected tab's `tty`
- compare the selected tab `tty` to the current process `tty`

This avoids false negatives when Terminal.app is frontmost but the user is looking at a different tab.

### Failure Strategy

If any foreground detection step fails, alerting should fail closed:

- not on macOS: no alert
- not in Terminal.app: no alert
- `tty` unavailable: no alert
- `osascript` failure: no alert

The system should never produce noisy or repeated false alerts because of an environment mismatch.

## CLI Surface

Add an alert flag to `host` and `join`:

- `--alert off|bell`

Initial default:

- `bell`

Effective behavior:

- only active in `scrollback`
- ignored in `tui`
- ignored outside Terminal.app

## Message Semantics

### Should Trigger Bell

- new inbound message from the peer, received in real time

### Must Not Trigger Bell

- transcript replay on startup/reconnect
- ACK processing
- outgoing messages
- resend markers like `[retrying]`
- system/status lines
- `/help` or `/status` responses

## UX Notes

- We do not maintain our own unread count.
- We do not attempt to clear the reminder ourselves.
- We depend on Terminal.app's built-in bell attention behavior.
- The user may need to enable the desired bell/badge behavior in Terminal profile advanced settings.

## Testing Strategy

### Unit Tests

- alert only on live inbound message path
- no alert for replayed transcript records
- no alert for outgoing/ACK/retry/system flows
- alert suppressed when current tab is foreground
- alert emitted when Terminal.app is not foreground or a different tab is selected

### Manual Validation

In Terminal.app:

1. Run `chatbox` in one tab.
2. Switch to another tab or another app.
3. Receive a new message.
4. Confirm Terminal.app shows its configured bell attention behavior.
5. Return to the `chatbox` tab.
6. Confirm the reminder clears via normal Terminal behavior.

## Constraints

- Repository is currently not a git repo, so design/plan docs can be written but not committed until git is initialized.
