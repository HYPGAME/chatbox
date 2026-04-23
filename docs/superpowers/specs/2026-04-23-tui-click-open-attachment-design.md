# TUI Click-To-Open Attachment Design

## Goal

Allow users in TUI mode to open an attachment by clicking its message row with the mouse, without changing the existing attachment transport, download, or preview flow.

## Scope

This change applies only to TUI mode.

Included:
- single-click attachment open in the message viewport
- drag-vs-click distinction so viewport scrolling keeps working
- reuse of the existing attachment open pipeline and status notices

Excluded:
- inline image or PDF preview inside the terminal
- scrollback/raw CLI click behavior
- changes to attachment message format
- changes to copy mode or revoke mode workflows

## Current State

The TUI already supports:
- `/open <attachment-id>` to download and open an attachment
- `O` in copy mode to open the selected attachment
- mouse wheel and left-button drag to scroll the viewport

The current mouse handler does not distinguish between a click and a drag. Any left press inside the viewport immediately enters drag mode, so there is no way to map a stable click action onto a rendered message.

## Design

### Interaction Model

- A left-button press inside the viewport records a pending click candidate.
- Mouse movement beyond a small vertical threshold converts that pending click into viewport dragging.
- Left-button release inside the viewport is treated as a click only if dragging never started.
- If the clicked rendered row belongs to an attachment message, chatbox reuses `startOpenCommand(attachmentID)`.
- If the clicked row is not an attachment row, nothing happens.
- If copy mode or revoke mode is active, click-to-open is disabled and existing mode behavior remains unchanged.

### Render Mapping

The viewport renderer already builds a rendered-state structure before handing lines to Bubble Tea. That structure will be extended so each rendered line also carries:
- the source history index
- whether the line belongs to a clickable attachment message
- the attachment ID when applicable

This keeps hit testing local to the rendered viewport state rather than reparsing terminal output.

### Mouse Handling

The mouse handler will move from "press immediately means drag" to a three-step state machine:

1. Press:
   - remember mouse Y
   - remember that a click candidate exists
   - do not start drag yet

2. Motion:
   - if the pointer moved enough, switch into drag mode
   - once dragging starts, preserve current viewport scroll behavior

3. Release:
   - if dragging occurred, stop dragging and do nothing else
   - if no drag occurred, resolve the clicked viewport line
   - if that line maps to an attachment, run the open command

This preserves the current scroll UX while making attachment rows actionable.

## Error Handling

- If attachments are unavailable, existing `startOpenCommand` error handling is reused.
- If the clicked row maps to a revoked or non-message entry, no open is attempted.
- If the click lands on wrapped lines belonging to the same attachment entry, all wrapped lines should still open the same attachment.

## Testing

Add TUI tests for:
- clicking an attachment row triggers one open request
- clicking a normal message does not trigger open
- dragging the viewport does not trigger open on release
- wrapped attachment rows remain clickable across multiple rendered lines
- copy mode and revoke mode ignore click-to-open

## Risks

- The viewport line-to-history mapping can drift if it is rebuilt inconsistently with the final rendered content.
- Overly sensitive drag detection could make clicking unreliable; overly lax detection could cause accidental opens during scrolling.

The implementation should keep the click/drag threshold minimal and deterministic so tests can cover it.
