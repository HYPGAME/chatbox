# chatbox

`chatbox` is a macOS-first command-line app for encrypted remote text chat.

It is intentionally small:
- direct TCP sessions between a host and one or more joiners
- one side hosts, others join
- authenticated and encrypted with a pre-shared key
- no server, no offline messages
- encrypted local transcript history

## Build

```bash
go build ./cmd/chatbox
```

To embed an explicit version string in a local build:

```bash
go build -ldflags "-X chatbox/internal/version.Version=v0.1.0" -o ./chatbox ./cmd/chatbox
```

## Version and Self-Update

Print the current build version:

```bash
./chatbox version
```

Update to the latest stable GitHub Release for the current macOS architecture:

```bash
./chatbox self-update
```

Update behavior:

- startup checks GitHub Releases asynchronously and prints a hint when a newer stable version exists
- `self-update` downloads the matching `darwin/arm64` or `darwin/amd64` release archive
- the archive is verified against `checksums.txt` before extraction
- `chatbox` tries to replace the current executable in place
- if the current install location is not writable, the new binary is written next to it and `chatbox` prints a manual replacement path

## Generate a Shared Key

```bash
./chatbox keygen --out ./chatbox.psk
```

The PSK file is required to be `0600`.

## Host a Chat

```bash
./chatbox host --listen 0.0.0.0:7331 --psk-file ./chatbox.psk --name alice
```

Default UI is `scrollback`, which uses a plain terminal line mode so messages live in the terminal's normal scrollback and can be reviewed with the terminal scrollbar, mouse wheel, and selection behavior directly.

If you want the old full-screen viewport UI instead:

```bash
./chatbox host --listen 0.0.0.0:7331 --psk-file ./chatbox.psk --name alice --ui tui
```

The host side must be reachable from the internet. In practice that means:
- a public IP on the Mac, or
- router port forwarding for the chosen TCP port

## Join a Chat

```bash
./chatbox join --peer 203.0.113.10:7331 --psk-file ./chatbox.psk --name bob
```

`join` also accepts `--ui tui` if you prefer the full-screen mode.

## Minimal Group Chat

You can use one host with multiple joiners:

```bash
./chatbox host --listen 0.0.0.0:7331 --psk-file ./chatbox.psk --name alice
./chatbox join --peer 203.0.113.10:7331 --psk-file ./chatbox.psk --name bob
./chatbox join --peer 203.0.113.10:7331 --psk-file ./chatbox.psk --name carol
```

Behavior:

- the host acts as the room relay
- every joiner connects only to the host, not to each other
- host status shows the current peer count
- the host view shows `joined` and `left` system lines as members connect or disconnect

This is intentionally a minimal host-relayed room, not a mesh network or a feature-rich chat server.

## In-Session Commands

- `/help`
- `/status`
- `/quit`

## History and Navigation

- Current-process history is kept in memory for the whole session.
- In the default `scrollback` UI, messages stay in the terminal's native scrollback, so you can use the terminal scrollbar or mouse wheel to review history.
- In optional `--ui tui` mode, use `PgUp`, `PgDn`, `Home`, and `End` to move through the current conversation.
- Encrypted transcript files are stored under `~/Library/Application Support/chatbox/history/`.
- Transcript encryption reuses the chat PSK.
- Transcript history is keyed by room: host mode uses the listen address, join mode uses the target host address.
- When you reconnect to the same room with the same PSK, previous messages are loaded automatically.

## Delivery Behavior

- Normal outgoing messages are shown once without delivery badges.
- If a connection drops, unacknowledged messages stay pending in the current process and are resent automatically after reconnection.
- Automatic resend adds a single `[retrying]` line so retries are visible without duplicating normal delivery updates.
- Pending resend does not survive process restart.

## Terminal Alerts

- `host` and `join` accept `--alert bell|off`.
- Default is `bell`.
- `bell` currently only has an effect in `--ui scrollback` under macOS `Terminal.app`.
- The bell triggers only for real-time incoming peer messages when the current chatbox tab is not the selected Terminal.app tab.
- Transcript replay, outgoing messages, ACKs, retry markers, and system lines do not trigger alerts.
- Terminal.app profile bell/badge behavior is controlled by Terminal settings.

## Release Publishing

GitHub Actions publishes macOS release archives when you push a tag matching `v*`.

Release assets:

- `chatbox_darwin_arm64.tar.gz`
- `chatbox_darwin_amd64.tar.gz`
- `checksums.txt`

Release flow:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow injects the tag into `chatbox/internal/version.Version`, builds both macOS binaries, generates checksums, and uploads all artifacts to the GitHub Release.

## Manual Release

If GitHub Actions is blocked by account billing state, use the local fallback release script:

```bash
./scripts/release-manual.sh v0.1.3
```

The script:

- requires a clean `main` branch
- runs `go test ./...`
- builds both macOS release archives
- generates `checksums.txt`
- pushes `main`
- creates and pushes the tag
- publishes the GitHub Release with assets

After a successful run, collaborators can update with:

```bash
chatbox self-update
```

## Limitations

- No zero-config NAT traversal
- No signaling or relay server
- No file transfer
- No mesh group chat
- No cross-process pending-message recovery
