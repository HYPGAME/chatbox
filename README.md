# chatbox

`chatbox` is a command-line app for encrypted remote text chat, with the smoothest experience today on macOS and a supported Android Termux path for CLI use.

It is intentionally small:
- direct TCP sessions between a host and one or more joiners
- one side hosts, others join
- authenticated and encrypted with a pre-shared key
- encrypted attachments relayed through the host
- no central server; the host only relays traffic and keeps encrypted short-term retention
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

Update to the latest stable GitHub Release for the current platform when self-update is supported:

```bash
./chatbox self-update
```

Update behavior:

- startup checks GitHub Releases asynchronously and prints a hint when a newer stable version exists
- `self-update` currently supports macOS and Linux `arm64` release archives
- the archive is verified against `checksums.txt` before extraction
- `chatbox` tries to replace the current executable in place
- if the current install location is not writable, the new binary is written next to it and `chatbox` prints a manual replacement path
- Android/Termux users should manually download and replace the binary from GitHub Releases

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

For a router or service host, use the non-interactive relay mode:

```bash
./chatbox host --headless --listen 0.0.0.0:7331 --psk-file ./chatbox.psk --name router
```

`--headless` is for daemon-style deployment:

- it does not start the scrollback or TUI interface
- it does not read stdin
- it logs only service-level events such as startup and join/leave
- it does not log chat message bodies
- it still serves encrypted attachment upload/download requests on the chat port plus one
- it cannot be combined with `--ui`

The host side must be reachable from the internet. In practice that means:
- a public IP on the Mac, or
- router port forwarding for the chosen TCP port

## Join a Chat

```bash
./chatbox join --peer 203.0.113.10:7331 --psk-file ./chatbox.psk --name bob
```

`join` also accepts `--ui tui` if you prefer the full-screen mode.

## Group Name Mode

You can create or join a stable room without generating a `.psk` file first:

```bash
./chatbox host --listen 0.0.0.0:7331 --name alice --group-name team-alpha
./chatbox join --peer 203.0.113.10:7331 --name bob --group-name team-alpha
```

If `--group-password` is omitted, `chatbox` prompts for it silently in an interactive terminal:

```bash
./chatbox host --listen 0.0.0.0:7331 --name alice --group-name team-alpha
group password for team-alpha:
```

For automation or router services, pass the password explicitly:

```bash
./chatbox host --headless --listen 0.0.0.0:7331 --name router --group-name team-alpha --group-password abc123
```

If you do not want the password to appear in the process list, point `chatbox` at a password file instead. Only the first line is used:

```bash
printf '%s\n' 'abc123' > /etc/chatbox/team-alpha.password
chmod 600 /etc/chatbox/team-alpha.password
./chatbox host --headless --listen 0.0.0.0:7331 --name router --group-name team-alpha --group-password-file /etc/chatbox/team-alpha.password
```

Room history, offline sync, revoke control, and room authorization stay on the same logical room as long as both the group name and password stay the same, even if the host IP changes.

## Compatibility Notes

- Peers are expected to run the same released version when joining the same room.
- The wire handshake performs a strict protocol-version check, so incompatible builds fail to connect instead of degrading gracefully.
- If one side updates and the other side cannot reconnect, verify both sides with `./chatbox version` first.

## Android / Termux

Android support is CLI-only through Termux. There is no APK or native Android UI.

Recommended device/runtime:

- Android on `arm64`
- Termux

Install from a release archive in Termux:

```bash
pkg update
pkg install tar
curl -L -o chatbox_android_arm64.tar.gz https://github.com/HYPGAME/chatbox/releases/latest/download/chatbox_android_arm64.tar.gz
tar -xzf chatbox_android_arm64.tar.gz
chmod +x ./chatbox
./chatbox version
```

Or build it yourself from source:

```bash
GOOS=android GOARCH=arm64 go build -o ./chatbox ./cmd/chatbox
```

Run commands the same way as on desktop:

```bash
./chatbox host --listen 0.0.0.0:7331 --psk-file ./chatbox.psk --name android-host
./chatbox join --peer 203.0.113.10:7331 --psk-file ./chatbox.psk --name android-joiner
```

Android-specific notes:

- `self-update` is not supported on Android; replace the binary manually from GitHub Releases
- macOS Terminal bell/badge notifications do not exist on Android Termux
- joining a host usually works fine, but hosting from a phone often fails on cellular networks because of carrier NAT or inbound-port restrictions
- `/open <attachment-id>` uses `termux-open` when available; `/download` always works without it
- transcript encryption and room history behavior are the same as other platforms

## Router Deployment

For OpenWrt/iStoreOS style deployment, the recommended layout is:

- install the Linux ARM64 binary as `/usr/bin/chatbox`
- keep the PSK in `/etc/chatbox/chatbox.psk`
- run the service as `chatbox host --headless ...`
- use the router's init system to keep it running

Example service command:

```bash
/usr/bin/chatbox host --headless --listen 0.0.0.0:7331 --psk-file /etc/chatbox/chatbox.psk --name iStoreOS
```

## Router Auto-Update

On OpenWrt/iStoreOS, `chatbox` can update itself daily from stable GitHub Releases.

Files in this repo:

- `scripts/router/chatbox-openwrt-autoupdate.sh`
- `scripts/router/chatbox-openwrt-cron.txt`

Suggested install:

```bash
cp scripts/router/chatbox-openwrt-autoupdate.sh /usr/bin/chatbox-openwrt-autoupdate.sh
chmod +x /usr/bin/chatbox-openwrt-autoupdate.sh
cat scripts/router/chatbox-openwrt-cron.txt >> /etc/crontabs/root
/etc/init.d/cron restart
```

Behavior:

- runs `chatbox self-update`
- verifies the binary using release checksums
- restarts `/etc/init.d/chatbox` only when the local version actually changed
- does not restart when already current, when the update fails, or when the updater falls back to a manual `.new` file
- if the first update attempt fails and OpenClash is enabled with `router_self_proxy=1`, the script temporarily stops OpenClash and retries once over the router's direct WAN path
- during that OpenClash bypass retry, the script also restarts `dnsmasq` once so router-local DNS stops pointing at OpenClash's `127.0.0.1:7874`
- before the retry, the script probes `https://github.com/` until direct HTTPS really comes back, instead of assuming a fixed wait is enough
- set `CHATBOX_OPENCLASH_RETRY_MODE=off` if you want to disable that OpenClash bypass retry

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
- the host also exposes the encrypted attachment service on `chat-port + 1`
- host status shows the current peer count
- `/status` shows the current online roster to both the host and all joiners
- the host view shows `joined` and `left` system lines as members connect or disconnect

This is intentionally a minimal host-relayed room, not a mesh network or a feature-rich chat server.

## In-Session Commands

- `/help`
- `/status` shows the local connection status and current online participant list
- `/events` shows join/leave event history visible to the current client
- `/file <path>` uploads an image or file to the host and sends a visible attachment message
- `/open <attachment-id>` downloads the attachment to local cache and opens it with the system default app
- `/download <attachment-id> [dest]` downloads the attachment without opening it
- `/update-all [version]` submits a room-wide update request
- `/quit`

In `--ui tui` copy mode, select an attachment message and press `O` to open it or `D` to download it.

## Attachments

- Attachments are encrypted end-to-end with keys derived from the same chat PSK.
- The host stores only ciphertext blobs and metadata under `~/Library/Application Support/chatbox/attachments/host/`.
- Attachment upload/download auth also reuses the PSK; the HTTP attachment service listens on the chat port plus one.
- Host-side encrypted blobs are deleted automatically after 7 days, including in `--headless` mode.
- Downloads are on-demand only. Receiving an attachment message never auto-downloads the file.
- Local attachment cache lives under `~/Library/Application Support/chatbox/attachments/cache/`.
- In `--ui tui` on macOS, use `Ctrl+V` to upload the current clipboard file or image through the same attachment flow.
- Plain-text paste still uses the terminal's normal paste behavior and inserts text into the input box.
- Some terminals may also emit a native paste event for clipboard file/image content; chatbox accepts that too, but `Ctrl+V` is the reliable path in Terminal.app.
- Clipboard file URLs keep their original format. Clipboard image content is exported to a temporary image file before upload.
- `/attach` and `/paste` remain accepted as compatibility aliases for older clients, but `/file` is the primary command shown in help.
- `/open` downloads to the local cache and then opens the file with the default system handler.
- `/download` writes to the provided destination, or to `~/Downloads/` when no destination is given.
- TUI shows upload/download progress in the status bar. Scrollback mode keeps progress transient on the input line and only prints the final result.
- Attachment messages render as compact summaries such as `[image] cat.gif (2.4 MB) #att_abc123`.

## History and Navigation

- Current-process history is kept in memory for the whole session.
- In the default `scrollback` UI, messages stay in the terminal's native scrollback, so you can use the terminal scrollbar or mouse wheel to review history.
- In optional `--ui tui` mode, use `PgUp`, `PgDn`, `Home`, and `End` to move through the current conversation.
- The host keeps encrypted text messages and revoke tombstones for 30 days.
- Host-side attachment blobs are retained for 7 days.
- On join, chatbox first asks the host for retained history authorized by the host's persisted first-seen timestamp for that `(room, identity)` pair.
- If the host has no retained text history yet, is still on an older build, or does not answer in time, chatbox falls back to the existing peer-sync path automatically.
- Encrypted transcript files are stored under `~/Library/Application Support/chatbox/history/`.
- Transcript encryption reuses the chat PSK.
- Transcript history is keyed by room: host mode uses the listen address, join mode uses the target host address.
- When you reconnect to the same room with the same PSK, previous messages are loaded automatically.
- In plain PSK mode, changing the host IP or port changes the local transcript key for joiners.
- In `--group-name` mode, room history and authorization stay stable across host IP changes because the derived room key stays the same.

## Delivery Behavior

- Normal outgoing messages are shown once without delivery badges.
- Outgoing messages render with the sender's configured display name, just like incoming messages.
- If a connection drops, unacknowledged messages stay pending in the current process and are resent automatically after reconnection.
- Automatic resend adds a single `[retrying]` line so retries are visible without duplicating normal delivery updates.
- Pending resend does not survive process restart.

## Terminal Alerts

- `host` and `join` accept `--alert bell|off`.
- Default is `bell`.
- `bell` currently has an effect in both `--ui scrollback` and `--ui tui` under macOS `Terminal.app`.
- The bell triggers only for real-time incoming peer messages when the current chatbox tab is not the selected Terminal.app tab.
- Transcript replay, outgoing messages, ACKs, retry markers, and system lines do not trigger alerts.
- Terminal.app profile bell/badge behavior is controlled by Terminal settings.

## Release Publishing

GitHub Actions publishes stable release archives when you push a tag matching `v*`.

Release assets:

- `chatbox_darwin_arm64.tar.gz`
- `chatbox_darwin_amd64.tar.gz`
- `chatbox_linux_arm64.tar.gz`
- `chatbox_android_arm64.tar.gz`
- `checksums.txt`

Release flow:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The workflow injects the tag into `chatbox/internal/version.Version`, builds both macOS binaries, generates checksums, and uploads all artifacts to the GitHub Release.

## Edge Prereleases

Every push to `main` also publishes a unique GitHub prerelease.

Behavior:

- each push to `main` gets its own `edge-<short-sha>` prerelease
- edge prereleases include the same darwin/android archives and `checksums.txt`
- edge binaries embed an `edge-<short-sha>` version string
- `self-update` continues to track stable releases, not edge prereleases

Use edge prereleases when you want the newest build from `main` without waiting for a tagged stable release.

## Manual Release

If GitHub Actions is blocked by account billing state, use the local fallback release script:

```bash
./scripts/release-manual.sh v0.1.3
```

The script:

- requires a clean `main` branch
- runs `go test ./...`
- builds macOS, Linux ARM64, and Android release archives
- generates `checksums.txt`
- pushes `main`
- creates and pushes the tag
- publishes the GitHub Release with assets

After a successful run, collaborators can update with:

```bash
chatbox self-update
```

On Android/Termux, download the latest `chatbox_android_arm64.tar.gz` release and replace the binary manually instead.

Recommended post-release smoke check:

```bash
./chatbox self-update
./chatbox version
```

Then do one quick `host` and `join` test with a shared PSK before telling others to upgrade.

## Limitations

- No zero-config NAT traversal
- No signaling or relay server
- No mesh group chat
- No inline image preview in the terminal
- No cross-process pending-message recovery
