---
created: 2026-09-05
status: done
requirements:
  - REQ-UI-PR-WALKTHROUGH-001
system_design:
  - ../../specs/ui/system-design/pr-walkthrough.md
legacy_specs: []
---

# Implementation Plan: PR Walkthrough Description Integrity Fix

## Overview

Make the walkthrough and preview description writers converge on one current
pull request body, preserve each other's marker-owned sections, and verify
their result. Add a non-generating reconciliation path for edited pull
requests so an obsolete full-SHA walkthrough URL is repaired when its current
short-SHA object is available.

The work is sequential. First, harden the shared description-write behavior
and canonical URL enforcement. Then add the edited-event repair path on top of
that protocol.

## Root cause

PR #3427 exposed a mismatch between the public object key and the URL in the
pull request body. The publisher and successful link job used
`pr/3427/990bca847b1c.html`, while the body contained
`pr/3427/990bca847b1c0c19c34f243df5a8b0a3b882a155.html`. The short URL returns
the walkthrough; the full URL returns 404. The Sprites preview is independent
and healthy.

The current repository has no active walkthrough producer that intentionally
emits the full-SHA URL. The body was changed after the successful link job by
an unidentified writer or stale full-body update. The exact external writer is
not recoverable from public GitHub history, but the current Kandev writers
permit the recurrence: they read a body, merge one section, PATCH the complete
body, and do not compare a fresh snapshot or verify the result.

The smallest reproduction is to publish the canonical short object, then
replace the owned walkthrough block with the legacy full-SHA URL. The object
lookup fails even though publication and the preview environment succeed.

## Scope

### In scope

- Enforce the 12-character public URL contract in every current walkthrough
  description path.
- Make walkthrough and preview body writes compare fresh snapshots, retry
  boundedly, preserve other marker-owned sections, and verify after PATCH.
- Serialize Kandev-controlled description writers with one per-PR concurrency
  group.
- Reconcile an existing stale walkthrough block on `pull_request_target`'s
  `edited` event when the current canonical object is available.
- Add focused unit, workflow-contract, and Go HTTP tests for the regression.

### Out of scope

- Identifying or changing an external GitHub App that may have overwritten the
  body; that requires organization or application audit access.
- Renaming or deleting existing full-SHA R2 objects.
- Replacing the PR description with a full template.
- Adding a Kandev application UI for walkthrough links.
- Adding a host-level legacy URL redirect. That remains optional defense in
  depth after the writer and reconciliation controls are deployed.

## Technical approach

### Description write protocol

Update `scripts/pr-walkthrough-pr-body` and
`apps/backend/cmd/preview/github.go` callers to follow the design's bounded
fresh-read, merge, compare, PATCH, and readback sequence. The walkthrough link
job continues to consume the publication job's validated URL and rejects any
URL that does not match the 12-character canonical form. The walkthrough link,
reconciliation, and short preview-description jobs use the shared
`pr-description-<number>` concurrency group with cancellation disabled for
the write section. Long-running preview lifecycle jobs do not hold that lock;
they pass their preview result to a trusted description job, which performs the
body write after deployment or cleanup.

The Go path will use an HTTP transport fake in tests and a bounded retry loop
around the body snapshot protocol. The workflow path will keep the
trusted helper and minimum GitHub permission boundary, but will stop retrying a
stale payload blindly. Both paths will verify their owned marker after the
PATCH and retry from the body that actually won.

### Edited-event reconciliation

Add a base-controlled, non-generating reconciliation workflow or workflow
boundary for `pull_request_target` `edited` events. It will use trusted
repository files, no model or R2 credentials, and the existing authorization
rules. It will check the current canonical URL with a public HTML request,
read the live body, and invoke the walkthrough helper only when an existing
walkthrough marker block is present. Missing objects or missing markers are
no-ops. Malformed markers fail closed.

The canonical no-op result prevents a body PATCH from creating a repair loop.
The reconciliation path uses the same post-write readback and shared
per-PR description concurrency group as the normal link job.

## Tests

- `scripts/pr-walkthrough-pr-body.test.py` will cover required-existing
  marker mode, legacy full-SHA replacement, canonical no-op behavior, and
  malformed-marker failure.
- `apps/backend/cmd/preview/github_test.go` or a focused sibling test will
  use a fake GitHub API to change the body between reads and to change it
  after PATCH. It will prove retry, marker preservation, cleanup behavior,
  and bounded failure.
- `.github/scripts/pr-walkthrough-workflow-contract_test.py` will assert the
  edited event boundary, non-generating reconciliation, trusted checkout,
  public-object check, canonical URL rule, shared concurrency group, fresh
  snapshot comparison, and post-write readback.
- Existing workflow and action-pinning tests will continue to enforce trusted
  inputs and minimal permissions.

## E2E tests

There is no Kandev UI flow. The workflow contract and fake GitHub API tests
cover the local regression. A same-repository pull request after deployment
must provide live evidence that an edited description with a legacy URL is
repaired and that a concurrent preview update preserves the walkthrough
callout.

## Work orders

- [x] [Task 01: Make PR description writes race-safe](task-01-race-safe-description-writes.md)
- [x] [Task 02: Reconcile stale walkthrough links](task-02-reconcile-stale-walkthrough-links.md)

## Verification results

Task 01 and Task 02 checks passed. The current Kandev writers are
race-safe, and edited covered pull requests can repair an existing stale
walkthrough callout when the canonical public object is available.

## Risks

- GitHub body updates replace the complete body, so an external writer can
  still race after the final local read. Shared concurrency, compare/retry,
  readback, and edited-event reconciliation reduce this risk but cannot lock
  writers outside Kandev.
- A missing current object prevents repair by design. The next successful
  publication must create the object before reconciliation can link it.
- The preview fork path executes contributor code with deployment credentials;
  this fix does not widen or redesign that existing trust boundary.
