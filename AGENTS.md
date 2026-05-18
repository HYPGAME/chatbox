## Release and Router Safety

- When the user asks to "发版" or "release", publish a full GitHub Release by default. A tag without a GitHub Release is not a completed release because `self-update` and `/update-all` consume GitHub Releases.
- After publishing a release, verify it with `./scripts/release-verify.sh <version>`.
- Do not update router binaries by writing directly to `/usr/bin/chatbox`. Upload to a temporary path, verify the file is non-empty and executable, verify `chatbox version`, then atomically move it into place and restart services.
- For router updates, keep the previous binary as a rollback file and verify `/etc/init.d/chatbox status`, `/etc/init.d/chatbox-group status`, and listening ports after restart.
