---
status: draft
system: executors
created: 2026-11-07
updated: 2026-11-07
owners:
  - kandev
---

# Docker executor storage bounds

### REQ-EXECUTORS-STORAGE-001: Bound disposable container writes

New Docker executors need finite log retention and temporary scratch capacity
so completed tasks cannot indefinitely grow the shared host's disk.

#### Acceptance criteria

- **AC-EXECUTORS-STORAGE-001.1:** Normal and interactive container creation
  shall configure rotating JSON logs with a 100 MB file limit and three files.
- **AC-EXECUTORS-STORAGE-001.2:** A newly created container without an explicit
  `/tmp` mount shall receive a 2 GiB tmpfs there, mode 1777, with `nosuid` and
  `nodev`; explicit task mounts shall retain their own storage policy.
- **AC-EXECUTORS-STORAGE-001.3:** Reconnecting to existing executors shall not
  recreate their containers or change their storage. Persistent task data,
  required evidence, bind mounts and writable-layer retention remain separate.
