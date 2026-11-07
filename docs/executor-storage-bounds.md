# Docker executor temporary storage

New containers created through the Docker client use rotating `json-file` logs
with a 100 MB file limit and three retained files. Their default `/tmp` is a
2 GiB tmpfs with mode 1777, `nosuid` and `nodev`. Scratch disappears when that
container is removed and consumes host memory/swap while allocated.

These defaults apply to normal and interactive creation through the shared
HostConfig builder. Reconnecting to an existing container leaves its storage
unchanged. An explicit task mount at `/tmp` retains its own storage and retention
policy and is not shadowed by the default tmpfs.

The bounds do not limit the writable layer, persistent task workspaces, package
stores, bind mounts or evidence archives. Those need separate lifecycle cleanup
and measured retention. Required evidence must be written to its designated
persistent task path before container closeout.

Validation:

```
cd apps/backend
KANDEV_DOCKER_STORAGE_CANARY_IMAGE=postgres:16 go test -race ./internal/agent/docker -count=1
```

The opt-in daemon test requires an existing local image with `/bin/sh`, `stat`,
`df`, `awk` and `dd`. It creates a network-isolated container through the actual
client, inspects its log budget, verifies the mounted scratch capacity and writes
a small scratch probe. It removes only that container and its anonymous volumes.
It neither downloads images nor recreates existing executors.
