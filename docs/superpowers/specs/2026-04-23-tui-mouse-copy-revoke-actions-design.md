# TUI Mouse Copy/Revoke Actions Design

## Context

The TUI already supports keyboard-first message actions:

- `copy mode` lets users move a message selection and then copy, quote, open, or download.
- `revoke mode` lets users move a revoke selection and confirm recall.
- Normal mouse behavior is limited to viewport scrolling and attachment click-to-open.

This leaves an awkward gap: once a user enters `copy mode` or `revoke mode`, the mouse stops being useful. Message actions remain available, but only through keyboard shortcuts. That is inconsistent with the newer attachment click behavior and makes mixed mouse/keyboard usage feel incomplete.

## Goal

Add mouse support for message-action workflows in TUI mode without replacing the current keyboard flow.

The mouse path should:

- allow selecting a message with a click in `copy mode`
- allow selecting a revoke candidate with a click in `revoke mode`
- expose explicit clickable actions in a small action bar
- avoid accidental destructive actions such as immediate revoke on first click
- preserve current non-mode behavior, including attachment click-to-open

## Non-Goals

- No right-click menu
- No text-range selection inside a message
- No drag-to-select behavior
- No modal popups or confirmation dialog layer
- No hover tooltips for buttons
- No behavior changes for scrollback mode

## Recommended Approach

Use a dedicated action bar rendered between the message viewport and the input box.

Why this approach:

- It keeps copy/revoke safe because clicking a message only changes selection.
- It reuses existing keyboard action functions instead of creating a second action system.
- It gives a consistent location for clickable actions across copy and revoke flows.
- It avoids overloading message clicks with too many meanings.

Rejected alternatives:

- Immediate click-to-copy or click-to-revoke: fast, but too easy to trigger by mistake.
- Right-click context menu: poor fit for Terminal.app and less reliable in terminal environments.

## Interaction Design

### Copy Mode

When `copy mode` is active:

- Clicking a message selects that message.
- Clicking does not immediately perform copy, quote, or attachment actions.
- An action bar is shown above the input box.

Action bar contents:

- For normal messages: `copy`, `quote`, `cancel`
- For attachment messages: `copy`, `quote`, `open`, `download`, `cancel`

Action behavior:

- `copy`: copies the selected rendered message text and stays in copy mode
- `quote`: inserts quoted content into the input box and exits copy mode
- `open`: opens the selected attachment and stays in copy mode
- `download`: downloads the selected attachment and stays in copy mode
- `cancel`: exits copy mode

### Revoke Mode

When `revoke mode` is active:

- Clicking an eligible outgoing message selects that revoke target.
- Clicking a non-eligible message does nothing.
- An action bar is shown above the input box.

Action bar contents:

- `revoke`, `cancel`

Action behavior:

- `revoke`: confirms revoke for the currently selected candidate and exits revoke mode
- `cancel`: exits revoke mode

### Normal Mode

When neither mode is active:

- Existing mouse behavior remains unchanged.
- Attachment rows still support hover feedback and click-to-open.
- Mouse drag still scrolls the viewport.

## Selection Model

Selection remains message-based, not line-based.

- Clicking any wrapped line of a multi-line message selects the same message.
- Clicking any rendered line of an attachment message selects the same message.
- Copy mode and revoke mode continue using the existing selection state:
  - copy mode: `copySelectionPos`
  - revoke mode: `revokeSelection`

Mouse selection updates those existing indices rather than introducing a parallel selection model.

## UI Layout

The TUI layout becomes:

1. status bar
2. viewport
3. action bar, only when `copy mode` or `revoke mode` is active
4. slash suggestions, if any
5. input box

The action bar is intentionally outside the viewport so:

- it never scrolls away with message history
- it has stable click coordinates
- it does not interfere with message wrapping or viewport line mapping

## Rendering Model

Introduce a lightweight rendered action-bar state, similar in spirit to `renderedViewportState`.

This state should track:

- rendered button labels
- button x-range in the action bar row
- semantic action identifier
- whether the action is currently available

The action bar is rendered from mode state plus the currently selected message.

Recommended action identifiers:

- `copy`
- `quote`
- `open`
- `download`
- `revoke`
- `cancel`

## Mouse Handling

Mouse event dispatch order should become:

1. action bar hit-test
2. viewport selection hit-test for copy/revoke mode
3. existing viewport scroll / attachment click logic

Specific rules:

- Left click only
- In copy/revoke mode, clicking a message inside the viewport updates selection first
- In copy/revoke mode, attachment click-to-open is suppressed from message rows because click now means selection
- Dragging the viewport must not trigger selection or button actions
- Action bar clicks should be ignored when the target action is unavailable

## Reuse of Existing Behavior

The mouse path should call the same behavior used by keyboard shortcuts:

- `copySelectedMessage`
- `quoteSelectedMessage`
- `startOpenCommand`
- `startDownloadCommand`
- `confirmSelectedRevoke`
- `exitCopyMode`
- `exitRevokeMode`

This avoids divergence between keyboard and mouse behavior.

## Error Handling

The new mouse flow should reuse existing status and operation notices.

Examples:

- Clicking `copy` with no valid selection should surface the existing copy-mode error notice.
- Clicking `open` or `download` on a non-attachment selection should not happen because the action should not render as available.
- Clicking a non-eligible revoke target should do nothing instead of producing an error.
- Download/open progress should continue using the existing operation notice path.

## Compatibility

- Keyboard shortcuts remain fully supported.
- Existing attachment click-to-open remains unchanged outside copy/revoke modes.
- Older transcript/message data requires no migration because this is pure UI behavior.

## Testing Plan

Add focused TUI tests covering:

1. Copy mode message click selects a normal message without executing copy.
2. Copy mode action bar renders `copy`, `quote`, and `cancel`.
3. Copy mode attachment selection renders `open` and `download` in addition to base actions.
4. Clicking `copy` triggers clipboard copy and keeps copy mode active.
5. Clicking `quote` writes quoted content and exits copy mode.
6. Clicking `cancel` exits copy mode.
7. Revoke mode click selects an eligible revoke target.
8. Revoke mode click on a non-eligible message leaves selection unchanged.
9. Clicking `revoke` sends the revoke control flow and exits revoke mode.
10. Clicking action bar buttons during drag gestures does not trigger actions.
11. Outside copy/revoke mode, existing attachment click-to-open still works.

## Implementation Boundaries

This feature should remain a TUI-only enhancement.

It should not include:

- visual redesign of the entire input area
- hover-driven menus
- button focus management for keyboard navigation
- scrollback-mode parity

## Success Criteria

The feature is complete when:

- copy and revoke workflows can be completed entirely with the mouse after entering the corresponding mode
- no destructive action happens on first message click
- keyboard workflows continue to behave exactly as before
- normal-mode attachment click-to-open still works
- automated tests cover both mouse mode-selection and action execution paths
