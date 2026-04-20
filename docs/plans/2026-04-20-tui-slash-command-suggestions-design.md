# TUI Slash Command Suggestions Design

**Goal:** In TUI mode, show inline slash-command suggestions and short descriptions while the user types `/` commands.

## Scope

- Only applies to `tui` mode.
- No change to `scrollback` mode input behavior.
- No command execution changes.
- No selection UI, tab completion, or keyboard navigation in the first version.

## UX

When the input starts with `/`, render a small suggestion block between the status line and the input box.

Suggestions are filtered by prefix:

- `/` shows all supported commands
- `/st` shows `/status -- 查询在线成员信息`
- full command still shows its description
- non-command input shows nothing

Initial command list:

- `/help -- 显示支持的命令`
- `/status -- 查询在线成员信息`
- `/quit -- 退出当前会话`

## Architecture

Keep the feature fully local to the TUI model layer:

- Add a small command metadata list in `internal/tui/model.go`
- Add a helper that derives matching suggestions from the current input value
- Render the suggestion block only in `uiModeTUI`

This avoids touching networking, session flow, or the scrollback terminal input loop.

## Testing

Add model/view tests that verify:

- `/` renders all suggestions
- `/st` renders only `/status`
- plain text renders no suggestions
- scrollback mode does not render the suggestion block

## Risks

- View output tests are sensitive to formatting changes
- The suggestion block reduces viewport height visually, but only in TUI mode

## Decision

Implement the minimum TUI-only read-only suggestion panel first. Defer selection, tab completion, and richer slash command metadata until after this proves useful.
