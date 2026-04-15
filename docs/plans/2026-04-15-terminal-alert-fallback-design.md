# Terminal Alert Fallback Design

## Goal

Improve macOS `Terminal.app` alert reliability in scrollback mode without weakening the existing "only alert when chatbox is not the active Terminal tab" behavior.

## Approved Approach

Use a two-stage foreground check on macOS:

1. Use `lsappinfo` to determine the frontmost application bundle ID.
2. If the frontmost app is not `com.apple.Terminal`, allow the alert immediately.
3. If the frontmost app is `Terminal.app`, keep using the existing AppleScript tab/TTY check and only alert when the selected tab TTY differs from the current chatbox TTY.

Any failed detection should still fail closed unless we have already positively determined that Terminal is not the frontmost app.

## Why This Approach

This keeps the current user-facing contract intact:

- no alert when the current chatbox tab is selected
- alert when Terminal is backgrounded or another app is frontmost
- no broad "Terminal frontmost means suppress everything" shortcut

It also gives a pragmatic reliability bump. `lsappinfo` is cheap and robust for app-level foreground detection, while AppleScript remains the only precise way in this codebase to identify the selected Terminal tab.

## Key Constraint

`lsappinfo list` output can contain `parentASN=...` references inside unrelated app blocks. ASN matching must only consider real top-level entry headers, otherwise the detector can resolve the wrong bundle ID.

## Testing

Add focused tests for:

- non-Terminal frontmost app allows alert even if AppleScript is unavailable
- ASN parsing ignores `parentASN` references and only matches real entry headers
- existing selected-TTY suppression behavior remains unchanged
