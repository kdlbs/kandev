#!/bin/sh
set -eu
set -f
umask 077
IFS= read -r public_key
set -- $public_key
[ "$#" -ge 2 ] || exit 10
algorithm=$1
blob=$2
case "$algorithm" in ssh-ed25519|ssh-rsa|ecdsa-sha2-nistp256|ecdsa-sha2-nistp384|ecdsa-sha2-nistp521) ;; *) exit 10 ;; esac
case "$blob" in ''|*[!A-Za-z0-9+/=]*) exit 10 ;; esac
owner=$(id -u)
# Both GNU and BSD stat return numeric ownership and modes without filenames.
check_owner_mode() {
 metadata=$(stat -c '%u %a' -- "$1" 2>/dev/null || stat -f '%u %Lp' "$1" 2>/dev/null) || return 1
 set -- $metadata
 [ "$#" -eq 2 ] && [ "$1" = "$owner" ] || return 1
 case "$2" in ''|*[!0-7]*) return 1 ;; esac
 [ $((0$2 & 077)) -eq 0 ]
}
[ -n "${HOME:-}" ] && [ -d "$HOME" ] || exit 11
directory=$HOME/.ssh
[ ! -L "$directory" ] || exit 12
if [ ! -e "$directory" ]; then mkdir "$directory" || exit 12; fi
[ -d "$directory" ] && check_owner_mode "$directory" || exit 12
lock=$directory/.kandev-enrollment-lock
mkdir "$lock" 2>/dev/null || exit 13
trap 'rmdir "$lock" 2>/dev/null || true' EXIT
trap 'exit 14' HUP INT TERM
keys=$directory/authorized_keys
[ ! -L "$keys" ] || exit 12
if [ ! -e "$keys" ]; then (set -C; : > "$keys") || exit 12; fi
[ -f "$keys" ] && check_owner_mode "$keys" || exit 12
# Compare key blobs even when options or comments precede/follow the key.
# An existing restricted entry must never gain an unrestricted duplicate.
if awk -v algorithm="$algorithm" -v blob="$blob" '
 /^[[:space:]]*#/ {next}
 {for (i=1;i<NF;i++) if ($i==algorithm && $(i+1)==blob) found=1}
 END {exit !found}
' "$keys"; then exit 0; fi
if [ -s "$keys" ] && [ -n "$(tail -c 1 "$keys")" ]; then printf '\n' >> "$keys"; fi
printf '%s\n' "$public_key" >> "$keys"
