#!/bin/sh
set -eu

CHATBOX_BIN="${CHATBOX_BIN:-/usr/bin/chatbox}"
CHATBOX_SERVICE="${CHATBOX_SERVICE:-chatbox}"
CHATBOX_INITD_DIR="${CHATBOX_INITD_DIR:-/etc/init.d}"
OPENCLASH_SERVICE="${OPENCLASH_SERVICE:-openclash}"
CHATBOX_OPENCLASH_RETRY_MODE="${CHATBOX_OPENCLASH_RETRY_MODE:-auto}"
CHATBOX_OPENCLASH_RETRY_SLEEP="${CHATBOX_OPENCLASH_RETRY_SLEEP:-3}"
LOCKDIR="${LOCKDIR:-/tmp/chatbox-update.lock}"
OPENCLASH_WAS_STOPPED=0

log() {
	message="$1"
	if command -v logger >/dev/null 2>&1; then
		logger -t chatbox-autoupdate -- "$message"
	fi
	printf '%s\n' "$message"
}

restore_openclash() {
	if [ "$OPENCLASH_WAS_STOPPED" != "1" ]; then
		return
	fi
	if service "$OPENCLASH_SERVICE" start >/dev/null 2>&1; then
		OPENCLASH_WAS_STOPPED=0
		return
	fi
	log "chatbox auto-update warning: failed to restart $OPENCLASH_SERVICE"
}

cleanup() {
	status=$?
	trap - EXIT INT TERM
	restore_openclash
	rmdir "$LOCKDIR"
	exit "$status"
}

openclash_retry_allowed() {
	[ "$CHATBOX_OPENCLASH_RETRY_MODE" = "off" ] && return 1
	command -v uci >/dev/null 2>&1 || return 1
	[ "$(uci -q get openclash.config.enable 2>/dev/null || true)" = "1" ] || return 1
	[ "$(uci -q get openclash.config.router_self_proxy 2>/dev/null || true)" = "1" ] || return 1
	command -v service >/dev/null 2>&1 || return 1
	return 0
}

run_self_update() {
	"$CHATBOX_BIN" self-update 2>&1
}

if ! mkdir "$LOCKDIR" 2>/dev/null; then
	log "chatbox auto-update skipped: another update is already running"
	exit 0
fi
trap cleanup EXIT INT TERM

if [ ! -x "$CHATBOX_BIN" ]; then
	log "chatbox auto-update failed: missing executable at $CHATBOX_BIN"
	exit 1
fi

before="$("$CHATBOX_BIN" version 2>/dev/null || printf 'unknown')"

if ! output="$(run_self_update)"; then
	if ! openclash_retry_allowed; then
		log "chatbox auto-update failed: $output"
		exit 1
	fi

	log "chatbox auto-update retrying with $OPENCLASH_SERVICE bypass after failure: $output"
	if ! service "$OPENCLASH_SERVICE" stop >/dev/null 2>&1; then
		log "chatbox auto-update failed: unable to stop $OPENCLASH_SERVICE for bypass retry"
		exit 1
	fi
	OPENCLASH_WAS_STOPPED=1
	sleep "$CHATBOX_OPENCLASH_RETRY_SLEEP"

	if ! output="$(run_self_update)"; then
		log "chatbox auto-update failed after $OPENCLASH_SERVICE bypass retry: $output"
		exit 1
	fi

	restore_openclash
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
"$CHATBOX_INITD_DIR/$CHATBOX_SERVICE" restart
