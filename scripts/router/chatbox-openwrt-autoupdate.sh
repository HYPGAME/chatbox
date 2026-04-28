#!/bin/sh
set -eu

CHATBOX_BIN="${CHATBOX_BIN:-/usr/bin/chatbox}"
CHATBOX_SERVICE="${CHATBOX_SERVICE:-chatbox}"
CHATBOX_SERVICES="${CHATBOX_SERVICES:-}"
CHATBOX_INITD_DIR="${CHATBOX_INITD_DIR:-/etc/init.d}"
OPENCLASH_SERVICE="${OPENCLASH_SERVICE:-openclash}"
DNSMASQ_SERVICE="${DNSMASQ_SERVICE:-dnsmasq}"
CHATBOX_OPENCLASH_RETRY_MODE="${CHATBOX_OPENCLASH_RETRY_MODE:-auto}"
CHATBOX_OPENCLASH_RETRY_SLEEP="${CHATBOX_OPENCLASH_RETRY_SLEEP:-3}"
CHATBOX_OPENCLASH_PROBE_URL="${CHATBOX_OPENCLASH_PROBE_URL:-https://github.com/}"
CHATBOX_OPENCLASH_PROBE_URLS="${CHATBOX_OPENCLASH_PROBE_URLS:-$CHATBOX_OPENCLASH_PROBE_URL https://github.com/HYPGAME/chatbox/releases/latest https://github.com/HYPGAME/chatbox/releases/latest/download/checksums.txt}"
CHATBOX_OPENCLASH_PROBE_MAX_ATTEMPTS="${CHATBOX_OPENCLASH_PROBE_MAX_ATTEMPTS:-20}"
CHATBOX_OPENCLASH_PROBE_TIMEOUT="${CHATBOX_OPENCLASH_PROBE_TIMEOUT:-5}"
CHATBOX_SELF_UPDATE_RETRIES="${CHATBOX_SELF_UPDATE_RETRIES:-3}"
CHATBOX_SELF_UPDATE_RETRY_SLEEP="${CHATBOX_SELF_UPDATE_RETRY_SLEEP:-2}"
CHATBOX_HEALTHCHECK_RETRIES="${CHATBOX_HEALTHCHECK_RETRIES:-5}"
CHATBOX_HEALTHCHECK_SLEEP="${CHATBOX_HEALTHCHECK_SLEEP:-2}"
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
	rm -f "$(lock_pid_path)" 2>/dev/null || true
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

is_transient_self_update_error() {
	output="$1"
	case "$output" in
		*"unexpected EOF"*|*"fetch latest release redirect: EOF"*|*"TLS handshake timeout"*|*"Client.Timeout exceeded"*|*"connection reset by peer"*|*"temporary failure in name resolution"*|*"i/o timeout"*)
			return 0
			;;
	esac
	return 1
}

run_self_update_with_retry() {
	attempt=1
	while true; do
		if output="$(run_self_update)"; then
			printf '%s' "$output"
			return 0
		fi

		if [ "$attempt" -ge "$CHATBOX_SELF_UPDATE_RETRIES" ] || ! is_transient_self_update_error "$output"; then
			printf '%s' "$output"
			return 1
		fi

		log "chatbox auto-update retrying self-update after transient failure ($attempt/$CHATBOX_SELF_UPDATE_RETRIES): $output"
		sleep "$CHATBOX_SELF_UPDATE_RETRY_SLEEP"
		attempt=$((attempt + 1))
	done
}

restore_local_dns() {
	if ! service "$DNSMASQ_SERVICE" restart >/dev/null 2>&1; then
		log "chatbox auto-update warning: failed to restart $DNSMASQ_SERVICE after stopping $OPENCLASH_SERVICE"
		return 1
	fi
	sleep 1
	return 0
}

wait_for_bypass_probe() {
	attempt=1
	while [ "$attempt" -le "$CHATBOX_OPENCLASH_PROBE_MAX_ATTEMPTS" ]; do
		if probe_bypass_urls; then
			return 0
		fi
		sleep 1
		attempt=$((attempt + 1))
	done
	log "chatbox auto-update warning: bypass probe did not succeed after $CHATBOX_OPENCLASH_PROBE_MAX_ATTEMPTS attempts for: $CHATBOX_OPENCLASH_PROBE_URLS"
	return 1
}

probe_bypass_urls() {
	for probe_url in $CHATBOX_OPENCLASH_PROBE_URLS; do
		if ! probe_bypass_url "$probe_url"; then
			return 1
		fi
	done
	return 0
}

probe_bypass_url() {
	probe_url="$1"
	if command -v curl >/dev/null 2>&1; then
		curl -sSI --max-time "$CHATBOX_OPENCLASH_PROBE_TIMEOUT" "$probe_url" >/dev/null 2>&1
		return $?
	fi
	if command -v wget >/dev/null 2>&1; then
		wget -q -T "$CHATBOX_OPENCLASH_PROBE_TIMEOUT" -t 1 --spider "$probe_url" >/dev/null 2>&1
		return $?
	fi
	return 1
}

lock_pid_path() {
	printf '%s/pid' "$LOCKDIR"
}

backup_bin_path() {
	printf '%s.previous' "$CHATBOX_BIN"
}

service_names() {
	if [ -n "$CHATBOX_SERVICES" ]; then
		printf '%s\n' "$CHATBOX_SERVICES"
		return 0
	fi
	printf '%s\n' "$CHATBOX_SERVICE"
}

service_script_path() {
	service_name="$1"
	printf '%s/%s' "$CHATBOX_INITD_DIR" "$service_name"
}

restart_service() {
	service_name="$1"
	service_script="$(service_script_path "$service_name")"
	if [ ! -x "$service_script" ]; then
		log "chatbox auto-update failed: missing init script for $service_name at $service_script"
		return 1
	fi
	"$service_script" restart
}

service_is_healthy() {
	service_name="$1"
	service_script="$(service_script_path "$service_name")"
	if [ ! -x "$service_script" ]; then
		return 1
	fi

	attempt=1
	while [ "$attempt" -le "$CHATBOX_HEALTHCHECK_RETRIES" ]; do
		if "$service_script" status >/dev/null 2>&1; then
			return 0
		fi
		if [ "$attempt" -lt "$CHATBOX_HEALTHCHECK_RETRIES" ]; then
			sleep "$CHATBOX_HEALTHCHECK_SLEEP"
		fi
		attempt=$((attempt + 1))
	done
	return 1
}

restart_and_check_services() {
	for service_name in $(service_names); do
		if ! restart_service "$service_name"; then
			return 1
		fi
		if ! service_is_healthy "$service_name"; then
			log "chatbox auto-update failed: service health check did not pass for $service_name"
			return 1
		fi
	done
	return 0
}

backup_current_binary() {
	cp "$CHATBOX_BIN" "$(backup_bin_path)"
}

restore_backup_binary() {
	backup_path="$(backup_bin_path)"
	[ -f "$backup_path" ] || return 1
	cp "$backup_path" "$CHATBOX_BIN"
	chmod +x "$CHATBOX_BIN"
}

remove_backup_binary() {
	rm -f "$(backup_bin_path)" 2>/dev/null || true
}

lock_is_stale() {
	[ -d "$LOCKDIR" ] || return 1

	pid_file="$(lock_pid_path)"
	if [ ! -f "$pid_file" ]; then
		return 0
	fi

	pid="$(sed -n '1p' "$pid_file" 2>/dev/null || true)"
	case "$pid" in
		''|*[!0-9]*)
			return 0
			;;
	esac

	if kill -0 "$pid" 2>/dev/null; then
		return 1
	fi
	return 0
}

acquire_lock() {
	if mkdir "$LOCKDIR" 2>/dev/null; then
		printf '%s\n' "$$" > "$(lock_pid_path)"
		return 0
	fi

	if ! lock_is_stale; then
		return 1
	fi

	rm -f "$(lock_pid_path)" 2>/dev/null || true
	rmdir "$LOCKDIR" 2>/dev/null || return 1
	if ! mkdir "$LOCKDIR" 2>/dev/null; then
		return 1
	fi
	printf '%s\n' "$$" > "$(lock_pid_path)"
	log "chatbox auto-update: recovered stale lock at $LOCKDIR"
	return 0
}

if ! acquire_lock; then
	log "chatbox auto-update skipped: another update is already running"
	exit 0
fi
trap cleanup EXIT INT TERM

if [ ! -x "$CHATBOX_BIN" ]; then
	log "chatbox auto-update failed: missing executable at $CHATBOX_BIN"
	exit 1
fi

before="$("$CHATBOX_BIN" version 2>/dev/null || printf 'unknown')"
backup_current_binary

if ! output="$(run_self_update_with_retry)"; then
	remove_backup_binary
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
	restore_local_dns || true
	sleep "$CHATBOX_OPENCLASH_RETRY_SLEEP"
	wait_for_bypass_probe || true

	if ! output="$(run_self_update_with_retry)"; then
		log "chatbox auto-update failed after $OPENCLASH_SERVICE bypass retry: $output"
		exit 1
	fi

	restore_openclash
fi

after="$("$CHATBOX_BIN" version 2>/dev/null || printf 'unknown')"

case "$output" in
	*"replace the current binary manually"*)
		remove_backup_binary
		log "chatbox auto-update requires manual replacement: $output"
		exit 0
		;;
esac

if [ "$before" = "$after" ]; then
	remove_backup_binary
	log "chatbox auto-update: $output"
	exit 0
fi

log "chatbox auto-update: updated $before -> $after; restarting service"
if restart_and_check_services; then
	remove_backup_binary
	exit 0
fi

if ! restore_backup_binary; then
	log "chatbox auto-update failed: service restart/health-check failed and rollback binary is unavailable"
	exit 1
fi

rollback_version="$("$CHATBOX_BIN" version 2>/dev/null || printf 'unknown')"
log "chatbox auto-update: rolled back to $rollback_version after failed restart/health-check"
if restart_and_check_services; then
	remove_backup_binary
	exit 1
fi

log "chatbox auto-update failed: rollback restart/health-check also failed"
exit 1
