---
created: 2026-11-19
status: complete
requirements:
  - REQ-WORKSPACES-IMPROVE-KANDEV-001
system_design:
  - ../../specs/workspaces/system-design/improve-kandev.md
  - ../../specs/workspaces/system-design/improve-kandev-bootstrap.md
legacy_specs: []
---

# Fix plan for the Improve Kandev bootstrap timeout

**Improve Kandev** prepares the workspace before it lets the user create a
task. On first use, the setup downloads the repository from GitHub. The
browser waits on the same request for the whole operation. When the clone
takes longer than the 30-second write deadline, Kandev can save the setup
after the connection has expired. The proxy shows 502 because it received
no usable response. The second call can use the repository already on disk.
[Issue #5327](https://github.com/kdlbs/kandev/issues/5327)

## Evidence

At `70875454752757e9a5c131a3df6ceb2c0c386dbd`, a temporary Go integration
test used the real Gin route, a loopback reverse proxy, and isolated SQLite.
With a one-second write deadline and a two-second fake clone, the proxy
received EOF and returned 502. SQLite held the repository and both workflows.
A repeat request returned 200 and reused that repository. The temporary
test file was removed.

A local Git process test terminated a clone while the upload process waited.
The target retained its `.git` directory, and `HEAD` did not resolve to a
commit. The test used a temporary checkout and removed it.

## Scope

1. Acknowledge preparation with `202` and a job ID. Call the existing setup
   in the background. Use one short status request and the existing success
   payload. Keep the synchronous request available for other clients.
2. The dialog shows its existing preparation banner and polls until ready.
   On failure, an inline **Retry** keeps the draft open.
3. Stage the initial managed clone in a temporary sibling directory. Move
   the completed checkout into place after Git succeeds.

The status endpoint needs a small in-memory map for active attempts and
recent results. A shared queue, a job database, and repair of pre-existing
partial checkouts are outside the fix. See the [bootstrap design](../../specs/workspaces/system-design/improve-kandev-bootstrap.md).

Moving the clone to task launch would avoid the bootstrap delay. However,
**Create only** can launch a terminal agent synchronously and encounter the
same delay. See the [decision](../../decisions/2026-11-19-improve-kandev-background-bootstrap.md).

## ASCII UI preview

### UI-01 Improve Kandev preparation

Open the action from the desktop footer or phone **Utilities**. The status
slot stays in the shared, scrollable task form. The phone uses a touch-sized
Retry button. The text and spacing below are illustrative.

```text
Pending
Preparing kandev repository in background
[Start task disabled]

Failed
Could not prepare Improve Kandev  [Retry]
[Start task disabled]

Ready
Repository and workflow details
[Start task available when normal checks pass]
```

The progress and error states cover
`AC-WORKSPACES-IMPROVE-KANDEV-001.14` and `.15`.

## Tests

- The Go integration regression uses the real handler behind a proxy.
  It holds the clone and requires the opt-in POST and status reads to
  respond while the clone is still pending. It also covers retry, job
  ownership, and legacy synchronous callers.
- A local clone test interrupts preparation before publication.
  A later attempt clones to the original managed path.
- The dialog component test covers polling, retry with a draft, and close.
  Desktop and phone Playwright tests cover submission after a pending
  result through the existing entry points.

## Work order

- [x] [Task 01 — Run bootstrap outside the HTTP request](task-01-background-bootstrap.md)

## Verification results

The temporary Go reproduction, local Git cancellation test, specification
lint, and PR documentation coverage passed. Implementation verification also
passed: the race-enabled backend regression suite, focused web unit tests,
web typecheck and production build, i18n checks, desktop and phone Playwright
tests, and public documentation tests and validation.

## Risk

Older callers can still hit the synchronous deadline. After a backend
restart or result expiration, the current dialog can retry. A forced
process exit can leave a staging directory, which the next attempt ignores.
