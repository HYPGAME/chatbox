#!/bin/sh
set -eu

export HOME="${HOME:-/root}"
export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-/root/.config}"

mkdir -p "$XDG_CONFIG_HOME/chatbox/historymeta" "$XDG_CONFIG_HOME/chatbox/hosthistory"

CHATBOX_LISTEN="${CHATBOX_LISTEN:-0.0.0.0:7441}"
CHATBOX_NAME="${CHATBOX_NAME:-$(uci get system.@system[0].hostname 2>/dev/null || echo iStoreOS)-group}"
CHATBOX_GROUP_NAME="${CHATBOX_GROUP_NAME:-}"
CHATBOX_GROUP_PASSWORD_FILE="${CHATBOX_GROUP_PASSWORD_FILE:-/etc/chatbox/chatbox-group.password}"

if [ -f /etc/chatbox/chatbox-group.env ]; then
	. /etc/chatbox/chatbox-group.env
fi

[ -n "$CHATBOX_GROUP_NAME" ] || exit 1
[ -n "$CHATBOX_GROUP_PASSWORD_FILE" ] || exit 1

exec /usr/bin/chatbox host --headless --listen "$CHATBOX_LISTEN" --group-name "$CHATBOX_GROUP_NAME" --group-password-file "$CHATBOX_GROUP_PASSWORD_FILE" --name "$CHATBOX_NAME"
