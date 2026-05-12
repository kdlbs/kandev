#!/bin/sh
set -e

if [ "$(id -u)" = '0' ]; then
    # HOME for the kandev user lives on the PV so agent CLI auth state
    # (gh, claude, codex, auggie, copilot, amp, ...) survives pod restarts
    # and image upgrades. Make sure it exists before dropping privileges.
    mkdir -p /data/home
    chown -R kandev:kandev /data
    exec gosu kandev "$@"
fi

exec "$@"
