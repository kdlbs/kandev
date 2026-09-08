---
id: "04-lock-owner-diagnostics"
title: "Ownership-lock owner diagnostics"
status: done
wave: 2
depends_on: ["01-proclive-shared-helper"]
plan: "plan.md"
spec: "../../specs/executors/requirements/port-collision-safety.md"
parallelism: parallel-safe
---

# Task 04: Ownership-lock owner diagnostics

Make a runtime-state ownership conflict name the process holding the lock, so an
operator with no leftover terminal can identify what to stop instead of being
told only that the home is busy.

Parallel-safe with task 02: this task touches only
`internal/backendapp/ownershiplock`, task 02 touches only `internal/launcher`.
Both depend on task 01.

**Read the boundary before writing code.** ADR
[2026-08-09-exclusive-runtime-state-ownership](../../decisions/2026-08-09-exclusive-runtime-state-ownership.md)
rejected "Use a PID file" as an ownership mechanism, and
`docs/specs/executors/requirements/port-collision-safety.md` lists PID identity matching as out of
scope. This task stays inside that boundary: the `flock` remains the sole
ownership proof, and the recorded PID is text in an error message. If you find
yourself reading owner metadata to decide whether a lock is held, stale, or
takeable, stop. That is the rejected design.

## Inputs

- Plan sections "Lock owner metadata" and "Ownership-lock owner diagnostics".
- Spec: `docs/specs/executors/requirements/port-collision-safety.md`, the "Exclusive runtime-state
  ownership" paragraphs on advisory owner metadata, its narrowed out-of-scope
  bullet, and scenarios 6 to 8 under "Concurrent backend startup".
- Current code: `apps/backend/internal/backendapp/ownershiplock/lock.go`,
  especially `Acquire` (line 40), `openLockFile` (line 67), and `ConflictError`
  (line 15).
- Consumer of the message:
  `apps/backend/internal/backendapp/main.go:220`. It prints `%v`, so no change
  is required there; confirm that rather than editing it.

## Implementation

Create `apps/backend/internal/backendapp/ownershiplock/owner.go` with:

```go
// OwnerRecord is advisory diagnostic metadata about the process that acquired a
// lock. It is never consulted to decide ownership, staleness, or takeover: the
// operating-system lock is the only proof. See ADR 2026-08-09.
type OwnerRecord struct {
    PID        int64  `json:"pid"`
    Executable string `json:"executable"`
    StartedAt  string `json:"started_at"` // RFC3339Nano
}
```

plus `writeOwner(file *os.File) error` and `readOwner(path string) *OwnerRecord`.

`writeOwner` builds the record from `os.Getpid()`, `os.Executable()` (empty
string on error, not a failure), and `time.Now().UTC().Format(time.RFC3339Nano)`,
then marshals to a single line. Write it with `file.Truncate(0)` followed by
`file.WriteAt(data, ownerMetadataOffset)` on the handle already held. The first
byte is reserved for the Windows lock range, so a conflicting process can read
the metadata through a separate handle while the lock is held. Truncating first
is what guarantees a concurrent reader sees either nothing or a valid prefix,
never a stale tail from a longer previous record.

`readOwner` opens the path separately for reading, reads after the reserved
byte, unmarshals, and returns `nil` on any error, on an empty file, or on a
non-positive PID. It must never block and never take a lock.

In `lock.go`:

- Open each sidecar with platform no-follow semantics and reject a symlink,
  reparse point, or non-regular file before returning the handle. The opened
  handle, rather than a second path lookup, is the regular-file proof used for
  the later metadata write.
- After `lockFile(file)` succeeds in `Acquire`, call `writeOwner(file)` and
  discard the error. A metadata write failure must not fail acquisition; the
  process owns the lock either way.
- On the `errors.Is(err, ErrConflict)` branch, call `readOwner(target.LockPath)`
  and put the result on a new `Owner *OwnerRecord` field of `ConflictError`.
- Suppress the detail when it cannot be trusted: if `proclive.Alive(rec.PID)`
  returns `(false, true)`, meaning positively gone, set `Owner` to `nil`. Keep it
  when liveness is unknown, `(false, false)`, which is what makes this useful on
  Windows.

Extend `ConflictError.Error()` rather than replacing it, so the existing sentence
and the `use a separate KANDEV_HOME_DIR` guidance appended by `main.go` still
read correctly:

```text
home target "/Users/cfl12/.kandev" is already owned by another backend (pid 51229, /path/to/kandev, started 2026-08-19T09:14:35Z)
```

With no trustworthy owner, the message must be byte-identical to today's.

Omit an empty executable from the parenthetical instead of rendering an empty
string.

## Acceptance

- A second `Acquire` against a held lock returns a `*ConflictError` whose `Owner`
  carries the first process's PID and executable, and whose `Error()` includes
  them.
- Empty, unparseable, or known-dead metadata produces exactly today's message.
- A failed metadata write still yields a successful `Acquire`.
- A symlinked lock path is rejected without modifying its target.
- Owner metadata remains readable while the lock is held on Windows.
- No code path reads `OwnerRecord` to decide whether a lock may be acquired.

## Verification

```bash
cd apps/backend && go test ./internal/backendapp/...
```

```bash
cd apps/backend && gofmt -l internal/backendapp && golangci-lint run ./internal/backendapp/... --new-from-rev=origin/main --timeout=5m
```

## Files likely touched

- `apps/backend/internal/backendapp/ownershiplock/owner.go` (new)
- `apps/backend/internal/backendapp/ownershiplock/owner_test.go` (new)
- `apps/backend/internal/backendapp/ownershiplock/lock.go`
- `apps/backend/internal/backendapp/ownershiplock/lock_test.go`

## Dependencies

Task 01 (`proclive.Alive`).

## Tests to write

In `lock_test.go`, extending the existing conflict coverage:

- Acquire a lock, attempt a second acquire on the same target, assert the
  returned `*ConflictError` has a non-nil `Owner` with this process's PID and
  that `Error()` contains both the path and the PID.
- Table-driven fallback: pre-write lock-file contents of `""`, `"not json"`,
  `"{}"`, and a valid record whose PID an injected probe reports as
  `(false, true)`; assert `Owner` is nil and the message equals the pre-change
  wording in every case.
- Unknown liveness, `(false, false)`, keeps the owner detail.
- Metadata write failure still returns a usable owner from `Acquire`.
- Create a symlinked sidecar and assert opening it fails without changing the
  target bytes.

In `owner_test.go`:

- Round-trip `writeOwner` then `readOwner`.
- Truncation: pre-fill the file with a longer valid record, write a shorter one,
  read back and assert no trailing bytes survive.
- Hold the OS lock and read owner metadata through a second handle.
- `readOwner` on a missing file, a directory, and a non-positive PID returns
  `nil` without error.

Inject the liveness probe through a package-level `var` so tests do not depend on
real PID lifetimes. `internal/backendapp/main_test.go`'s existing
`TestBackendStartupConflictStopsBeforeSharedStateInitialization` must pass
unchanged; it is the regression guard that this task did not disturb the
fail-closed startup ordering.

## Output contract

Report the new files, the changed `ConflictError` shape, exact commands with
outcomes, a sample of the enriched message, and explicit confirmation that no
acquisition decision reads owner metadata. Update this file's `status` and
`## Results`, and the matching checkbox in `plan.md`.

## Results

- Added advisory `OwnerRecord` JSON metadata with PID, executable, and RFC3339Nano
  start time; acquisition ignores metadata write failures.
- Enriched `ConflictError` with owner details only when metadata is parseable and
  liveness is not positively known to be dead. Path-only wording remains the
  fallback.
- Added regression coverage for owner rendering, truncation, invalid/dead
  metadata, unknown liveness, metadata-write failure, and existing startup
  conflict behavior.
- `cd apps/backend && go test -run 'TestOwnershipLockConflict|TestOwnershipLockMetadataWriteFailure' ./internal/backendapp/ownershiplock/...`
  passed.
- `cd apps/backend && go test ./internal/backendapp/...` passed for backendapp
  and ownershiplock.
- `gofmt -w` completed for all changed ownership-lock Go files.
- Cleanup: all in-process owners are closed with test cleanup or explicit close;
  no lock handles remain.
- Security/trust and external side-effect boundaries: owner metadata never
  participates in acquisition, staleness, or takeover decisions; the OS lock
  remains authoritative.
- Fixup hardening: platform no-follow sidecar opens reject symlinks/reparse
  points before metadata writes, and the reserved lock byte keeps Windows
  conflict diagnostics readable while the lock is held.
