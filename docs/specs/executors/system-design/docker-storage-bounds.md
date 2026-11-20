---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-STORAGE-001
---

# Docker executor storage bounds design

The Docker daemon owns the logging driver. The executor profile can supply
rotation options and a size for an executable `/tmp` tmpfs.

## Profile settings

Save optional values in the existing `ExecutorProfile.Config` string map.
Treat blank values like absent values.

| Profile key | Example | Meaning |
| --- | --- | --- |
| `docker_tmp_size` | `2g` | Size of an optional `/tmp` tmpfs. `2g` is 2 GiB. |
| `docker_log_max_size` | `100m` | Docker `max-size` option. |
| `docker_log_max_files` | `3` | Docker `max-file` option. |

## Fresh launch

Use the Docker network settings pattern to clear task-supplied values and
carry the selected profile's values into `buildDockerContainerConfig`. Trim
them there. Convert a supplied `/tmp` size to bytes with
`github.com/docker/go-units`'s `RAMInBytes`, already in the module graph.
Return a launch error naming `docker_tmp_size` if conversion fails or yields
a nonpositive value. This keeps arbitrary text out of the mount options.

Carry the byte size and the two Docker log options through `ContainerManager`
to `docker.ContainerConfig`. Both create methods call `buildHostConfig`. Leave
`LogConfig.Type` empty and set only the supplied entries in `LogConfig.Config`.
Docker selects its driver, merges its other options, and validates the result
against that driver. The existing launch error path reports Docker's error.
[Moby log configuration merge](https://github.com/moby/moby/blob/v26.1.5/daemon/logs.go#L163-L182)

If the profile provides a size, apply `rw,exec,nosuid,nodev,size=<bytes>,mode=1777`
at `/tmp`. An explicit mount at `/tmp` takes precedence. Reuse the existing
`path.Clean` check to recognize equivalent paths. Reconnecting to a retained
container uses its original mount and log configuration.

## Profile editor

Use the canonical Local Docker and Remote Docker profile editor. Add one
Storage limits card after Container networks. Use the existing Settings save
and discard actions. Trim the three inputs, omit cleared keys, and preserve
unrelated values in the profile. Use examples to show the expected values and
explain that the settings apply to new containers.

Use the existing settings page as the scroll owner. The same field order,
state, and save action apply on phone. Use full-width fields and the existing
coarse-pointer control sizes, with the standard save action above the safe
area. The existing Remote Docker connection setup leads to this profile editor.
[Executor profile editor design](profile-editor.md#phone-composition)

## Requirement mapping

| Acceptance criteria | Design |
| --- | --- |
| AC-EXECUTORS-STORAGE-001.3 through .7 | Fresh launch |
| AC-EXECUTORS-STORAGE-001.8 | Profile editor |

[Requirements](../requirements/docker-storage-bounds.md) · [Fix plan](../../../plans/docker-executor-storage-opt-in/plan.md) · [Public reference](../../../public/executors.md#docker-log-and-temporary-storage-limits)
