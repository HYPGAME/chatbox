# Headless Router Host and Stable Auto-Update Design

## Goal

Allow `chatbox` to run as a true non-interactive relay service on the router, and let that router check for a newer stable release once per day and apply it locally with an immediate service restart.

## Scope

- Add a real `headless` host mode for daemon-style deployment.
- Keep host message relaying and room membership behavior unchanged.
- Avoid logging chat message bodies on the router.
- Extend stable release publishing and `self-update` support to `linux/arm64`.
- Support daily unattended local update checks on the router.
- Do not add an in-process scheduler or a router-specific control API.

## Approach

Use a new CLI flag, `chatbox host --headless`, instead of a separate subcommand.

- `--headless` starts the host relay without `scrollback` or `tui`.
- `--headless` is mutually exclusive with `--ui`.
- `--headless` ignores interactive-only concerns such as terminal alerts and startup update hints.
- A dedicated headless runtime consumes `HostRoom` events and logs only system-level information such as startup, shutdown, peer join, and peer leave.
- The headless runtime must also continuously drain host-side message and receipt channels without printing their contents, otherwise the existing buffered channels will eventually apply backpressure and stall relaying.

Daily auto-update stays outside the main `chatbox` process.

- A router-local script runs from `cron` once per day.
- The script records `before=$(chatbox version)`, runs `chatbox self-update`, then records `after=$(chatbox version)`.
- If `before != after`, the script immediately restarts `/etc/init.d/chatbox`.
- If the update reports "already up to date", fails network/download/checksum validation, or only produces a manual fallback file, the script logs that result and leaves the running service untouched.

Stable releases must publish a `linux/arm64` archive, and the updater must know how to select it.

- Release assets gain `chatbox_linux_arm64.tar.gz`.
- `checksums.txt` continues to be the integrity source of truth.
- `self-update` for `linux/arm64` follows the same checksum-verified archive flow as existing supported platforms.
- `self-update` remains stable-only. It must not consume edge prereleases.

## Runtime Behavior

Headless host invocation:

```bash
chatbox host --headless --listen 0.0.0.0:7331 --psk-file /etc/chatbox/chatbox.psk --name iStoreOS
```

Expected behavior:

- listen and accept peers exactly like the current host mode
- relay encrypted messages between all members
- keep `/status` and group-room behavior unchanged for connected clients
- not require stdin, a TTY, or a terminal UI
- terminate cleanly on service stop signals

Headless logging should stay minimal:

- startup line with listen address and local name
- peer joined / peer left lines
- fatal host/runtime errors
- optional update-script lines, produced by the external update script rather than by the headless host itself

Headless mode must not log:

- chat message bodies
- transcript contents
- background update hints such as `run: chatbox self-update`

## Failure Handling

- If `--headless` is combined with `--ui`, exit with a clear CLI error.
- If the host listener fails to start, exit non-zero so `procd` can surface and respawn appropriately.
- If the headless runtime receives a termination signal, close the host cleanly and exit zero.
- If daily update fails, keep serving the current version and try again the next day.
- If `self-update` cannot replace the binary in place and writes a fallback `.new` file instead, do not restart automatically.

## Expected Result

- The router can run `chatbox` as a real background relay service without stdin hacks.
- The service no longer depends on `tail -f /dev/null | ...` wrappers.
- The router can keep itself on the latest stable release with a simple daily `cron` job.
- Chat privacy is better preserved because router logs no longer contain message contents.
