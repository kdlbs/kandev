---
status: draft
system: executors
created: 2026-11-07
updated: 2026-11-19
owners:
  - kandev
---

# Docker executor storage bounds

## Overview

Local Docker and Remote Docker profiles can set limits for log rotation and
temporary storage. With empty settings, Docker uses its own logging policy
and the container uses its ordinary `/tmp` storage.

## Requirements

### REQ-EXECUTORS-STORAGE-001: Docker container storage limits

Profile editors can opt in to each limit for future Docker containers. This
lets them manage disk use and choose where temporary files are stored.

#### Acceptance criteria

- **AC-EXECUTORS-STORAGE-001.3:** Reconnecting to an existing Docker executor
  shall reuse its container with its original storage settings.
- **AC-EXECUTORS-STORAGE-001.4:** When a limit is blank or absent, normal and
  interactive creation on either Docker runtime shall use the Docker or image
  default. This includes older profiles without these settings and launches
  without a profile. Only settings from the selected profile shall control the
  limits.
- **AC-EXECUTORS-STORAGE-001.5:** When a profile sets a log size or file count,
  Docker shall use the daemon's logging driver and inherit its other log
  options. If Docker rejects the requested options, Kandev shall report the
  daemon error and the launch shall fail.
- **AC-EXECUTORS-STORAGE-001.6:** When a profile sets a `/tmp` size, new
  containers shall use that size limit for a tmpfs with executable files, mode
  1777, `nosuid`, and `nodev`. An explicit mount at `/tmp` or an equivalent
  path shall take precedence and retain its settings.
- **AC-EXECUTORS-STORAGE-001.7:** When a profile supplies a nonblank `/tmp`
  size that Kandev cannot read as a positive byte size, a fresh launch shall
  fail with an error that identifies the setting. Blank or absent values use
  the Docker or image default as defined by AC-EXECUTORS-STORAGE-001.4.
- **AC-EXECUTORS-STORAGE-001.8:** Authorized users shall be able to set and
  clear each limit independently from either Docker profile editor on desktop
  and phone. Discard shall restore saved values, and a successful save shall
  survive reload.

## Exclusions

Persistent task storage, writable-layer quotas, and evidence retention have
their own policies.

## Superseded acceptance criteria

- **AC-EXECUTORS-STORAGE-001.1** Required a 100 MB file size and three files
  with `json-file` for every new container. The opt-in policy replaces it
  with AC-EXECUTORS-STORAGE-001.4 and AC-EXECUTORS-STORAGE-001.5.
- **AC-EXECUTORS-STORAGE-001.2** Required 2 GiB of tmpfs by default. The
  opt-in policy replaces it with AC-EXECUTORS-STORAGE-001.4 and
  AC-EXECUTORS-STORAGE-001.6.
