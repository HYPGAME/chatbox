#!/bin/sh
set -eu

CHATBOX_BIN="${CHATBOX_BIN:-/usr/bin/chatbox}"
CHATBOX_SERVICE="${CHATBOX_SERVICE:-chatbox}"
LOCKDIR="${LOCKDIR:-/tmp/chatbox-update.lock}"

log() {
	message="$1"
	if command -v logger >/dev/null 2>&1; then
		logger -t chatbox-autoupdate -- "$message"
	fi
	printf '%s\n' "$message"
}

if ! mkdir "$LOCKDIR" 2>/dev/null; then
	log "chatbox auto-update skipped: another update is already running"
	exit 0
fi
trap 'rmdir "$LOCKDIR"' EXIT INT TERM

if [ ! -x "$CHATBOX_BIN" ]; then
	log "chatbox auto-update failed: missing executable at $CHATBOX_BIN"
	exit 1
fi

before="$("$CHATBOX_BIN" version 2>/dev/null || printf 'unknown')"

if ! output="$("$CHATBOX_BIN" self-update 2>&1)"; then
	log "chatbox auto-update failed: $output"
	exit 1
fi

after="$("$CHATBOX_BIN" version 2>/dev/null || printf 'unknown')"

case "$output" in
	*"replace the current binary manually"*)
		log "chatbox auto-update requires manual replacement: $output"
		exit 0
		;;
esac

if [ "$before" = "$after" ]; then
	log "chatbox auto-update: $output"
	exit 0
fi

log "chatbox auto-update: updated $before -> $after; restarting service"
"/etc/init.d/$CHATBOX_SERVICE" restart
