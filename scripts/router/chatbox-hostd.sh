#!/bin/sh
set -eu

export HOME="${HOME:-/root}"
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-/root/.config}"

mkdir -p "$XDG_CONFIG_HOME/chatbox/historymeta" "$XDG_CONFIG_HOME/chatbox/hosthistory"

CHATBOX_LISTEN="${CHATBOX_LISTEN:-0.0.0.0:7331}"
CHATBOX_PSK_FILE="${CHATBOX_PSK_FILE:-/etc/chatbox/chatbox.psk}"
CHATBOX_NAME="${CHATBOX_NAME:-$(uci get system.@system[0].hostname 2>/dev/null || echo iStoreOS)}"

if [ -f /etc/chatbox/chatbox.env ]; then
	. /etc/chatbox/chatbox.env
fi

exec /usr/bin/chatbox host --headless --listen "$CHATBOX_LISTEN" --psk-file "$CHATBOX_PSK_FILE" --name "$CHATBOX_NAME"
