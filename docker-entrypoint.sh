#!/bin/sh
set -e

if [ "$(id -u)" = '0' ]; then
    chown -R kandev:kandev /data
    exec gosu kandev "$@"
fi

exec "$@"
