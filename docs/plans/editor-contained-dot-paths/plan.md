---
created: 2026-10-06
status: in_progress
requirements:
  - REQ-WORKSPACES-EDITOR-CONTAINMENT-001
system_design:
  - ../../specs/workspaces/system-design/editor-file-containment.md
legacy_specs: []
---

# Implementation Plan: Contained Editor Dot Paths

## Overview

Correct the existing editor service's lexical containment guard so contained
names beginning with `..` reach normal editor dispatch. One sequential work
order owns permanent real-service regression tests, the guard correction, and
targeted verification. This package is design only until a later same-primary
ROOT implementation interrupt after concrete review and actual design END.

Workspaces owns target membership before editor dispatch. The searched
file-tree-path-scope, hidden-folder-browsing, embedded-vscode availability,
open-task-folder, and attach-workspace-sources contracts own different operations.
None provides an acceptance criterion for this guard. The new vertical pair
defines reusable behavior rather than duplicating an incident or child48's
embedded target-encoding contract. No system boundary/index change is needed.

## Confirmed evidence

ROOT supplied an actual `Service.OpenEditor` temporary-filesystem reproduction,
not a copied predicate. Its archived source at
`/tmp/kandev-editor-dot-prefix-repro_test.go` has SHA256
`2868e7e842c6c5ef5be31f378ae38aae80a37f5552ccf0c0147b35446bfccff4`.
Read-only audit confirmed the checksum, raw failure log and
`/tmp/kandev-root-editor-dot-prefix-proof-{identity,receipt,classification}.json`.
ROOT native handle 10320 actually joined exit 1 in 2.721s, with package time
0.022s and no timeout/setup failure. `..notes.go` and `..notes/inside.go` were
rejected with `ErrEditorConfigInvalid`; ordinary `src/plain.go` and true
`../outside.go` rejection controls passed. The proof is retained untouched and
must never be replayed, modified, or removed here.

At design audit, own HEAD and actual remote main were
`670847f0a48cf36c14bdbd929a2fb96a3a565eea`. The current service blob
`ba2c7d8e5db1fc9ffef03b1475c09add363b8dd0` matches the accepted proof's source
at base `9885f0e558eb0dfd763e5f2106caadf43975d20d`.
The causal guard is `strings.HasPrefix(rel, "..")` after `filepath.Rel`.

## Scope

- Own `resolveFilePath` admission and a new adjacent regression-test file.
- Preserve normalization, folder/missing-workspace behavior, error categories,
  editor eligibility and selected-worktree resolution.
- Qualify actual native-platform coverage and preserve escape controls.
- Exclude symlink realpath/security-policy changes, generic path frameworks,
  native CLI colon interpretation, API/schema/transport changes, native launches,
  builds, browser/E2E, session lifetime, and broad editor-registry audits.
- Child48 independently owns `buildInternalVscodeURL` and the frontend parser.
  Do not edit its worktree/docs, change its contract, or rebase for main movement.

## Source and test inventory

| Boundary | Existing source | Disposition |
| --- | --- | --- |
| Admission/dispatch | `apps/backend/internal/editors/service/service.go` | Only `resolveFilePath` changes |
| Service fixtures | `apps/backend/internal/editors/service/service_test.go` | Reuse minimal repositories/settings; leave URL-builder tests owned by their existing contract |
| Selection/folder | `service.go`, `service/folder.go`, `service/folder_test.go` | Preserve; no native launch |
| HTTP controller/request | `editors/controller/controller.go`, `editors/dto/dto.go` | Read-only field/error inventory |
| HTTP registration/mapping | `editors/handlers/handlers.go`, `handlers/handlers_test.go` | Existing error mapping test; no registered service fixture today |
| Embedded eligibility | `editors/capabilities/capabilities.go` | Preserve Local Docker capability fixture without Docker |
| Browser caller | `apps/web/lib/api/domains/session-api.ts`, `apps/web/hooks/use-open-session-in-editor.ts` | Read-only; no frontend changes |
| Integration providers | `dispatchEditorKind`, URL/args/placeholder builders | Preserve; compatibility matrix in design |

## Tests

| Criteria | Permanent evidence |
| --- | --- |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.1` | `TestOpenEditor_ContainedDotPaths`: actual files, real service, full decoded target and position, exact bytes |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.2` | Same real-service controls and `TestResolveFilePath_ContainmentControls`: native normalization and dot names |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.3` | Both new tests: exact parent/nested escape error and empty target; existing HTTP 400 mapping |
| `AC-WORKSPACES-EDITOR-CONTAINMENT-001.4` | New empty-root/file controls plus existing session, folder and embedded availability tests |

Real-service/file-system integration is the operation's end-to-end evidence.
Registered routing remains conditional on a cheap existing fixture; no browser
or native app claim is made. No rendered UI/copy changes: the mobile internal
state/data exception applies. Public documentation was assessed under
`/docs-maintainer`; `docs/public/developer-tools.md` already covers opening
worktree files, so no public edit is needed.

## Work orders

- [ ] [Task 01: Preserve contained dot paths](task-01-preserve-contained-dot-paths.md)

Task 01 is `in_progress` under ROOT's later same-primary implementation release
following the accepted design and test-authoring checkpoints. ROOT's concrete
four-file review receipt is `/tmp/kandev-root-child49-design-review.json`.
Permanent RED, the one-line guard correction, affected GREEN and scoped lint
have completed. Delivery is authorized under the exclusive GLOBAL LOCAL-HEAVY
lease; hosted verification and separate ROOT merge admission remain pending.
This task holds no MERGE lease.

## Execution and delivery boundaries

No local-heavy or merge lease is granted by design. Later execution remains in
the same primary/profile, without delegates or extra tasks/sessions. ROOT owns
one GLOBAL LOCAL-HEAVY lease across children and a separate serial MERGE lease.
Every shell needs retained native handle, owned PID/group, start, absolute cutoff,
argv and log; actually join and prove the group gone before advancing.
Routine affected-only fixture/lint repairs are allowed. Timeout, resource,
transport, unknown, or out-of-scope failures checkpoint ROOT without auto retry.

After later standing-authorized READY publication, freeze the head except for
actual corrective findings. Use one original 45-minute all-terminal
`scripts/pr-await` collector under GNU 46-minute timeout/kill10, retaining its
upfront handle, PID/group/start/cutoff/log. Preserve and join it before any
ROOT-explicit replacement; no self replacement, duplicate query timers or retry.
Require all six actual required contexts and actual Backend/Frontend/E2E parents
SUCCESS, fresh complete `errors[]` empty at exact head/current PR/governance and
resolver state, and no visible/hidden actionable changes-requested or human
findings. Require authenticated CodeRabbit App347564 substantive FULL current-head
all-file review; automatic review suffices. Inspect completed/processing state
before at most one necessary full request for a real gap. No ACK, optional wait,
settings change, main-only rebase, synthetic merged tests or weakened checks.
Inspect and ground findings while CI runs. Only ROOT admits a bounded hosted
exact failed job plus normal causal dependencies retry after fresh terminal
eligibility, with name-keyed budgets; no whole/passing/unrestricted replay.

MERGE-ready END precedes a separate ROOT serial MERGE interrupt after actual
current-main/head/merge-tree/owned-blob/shared-contract review. Use normal
expected-head squash without admin. Independently verify REST/Git merge,
parent/tree/owned blobs and remote inclusion, all owned handles joined/gone,
only-owned cleanup and explicit lease RETURN. Retain managed worktree/deps/logs.
ROOT independently proves merge before archive. No foreign/shared mutation,
indiscriminate kills/prunes/refs/FETCH_HEAD, or unproved cleanup. Machine loss
means NO VERDICT and unknown-mutation reconciliation. Callback/question queue
FULL means version-safe preservation of the exact existing Kandev plan,
system marker, identities and user edits, checkpoint, then END WAITING; no retry
or new session. Critical question calls end the turn. Design END is not task
completion; the task stays pending until verified merge and joined cleanup.

## Verification results

Design validation passed on 2026-10-06: catalog validated 355 decisions and
1398 specifications; all-file specification lint passed; all 36 spec-linter
tests passed. The repository PR documentation validator accepted the actual
REQ/AC/design/plan references with the planned production path included:
`status=covered`, one work order, `errors=[]`. Actual docs-only classification
is `exempt`; neither result claims production or regression-test coverage.
Four-file untracked inventory, empty index, diff whitespace and full artifact
whitespace checks passed. Bounded native command receipts/logs are retained in
`/tmp/kandev-design49/`; completed groups were actually joined and gone.
At the historical design checkpoint, implementation and publication were pending
and no local-heavy or merge lease had been consumed.

Historical post-design test-authoring checkpoint: ROOT's limited release marked
Task 01 `in_progress` and authored only the new
`apps/backend/internal/editors/service/editor_file_containment_test.go`.
The two permanent service/resolver tests are ready but unexecuted, including
the exact legacy and ROOT-confirmed new sentinel tuples. Production source and
ROOT proof remain unchanged. Read-only fixture audit found no registered route
fixture. No product checks, install, hooks, publication, or merge ran; no active
owned handle or lease remains. END WAITING for ROOT's later same-primary heavy
release before meaningful permanent RED and the guard correction.

ROOT subsequently granted exclusive implementation on 2026-10-06. Permanent
real-service RED failed on seven contained-name cases while eleven ordinary,
folder and escape controls passed. The original authored test bytes are retained
in `/tmp/kandev-child49/red-test-source.go`; the ROOT disposable proof was not
replayed. The guard now rejects exactly `..` or a native parent-component prefix.
Affected service and handler race tests passed (1.032s and 1.035s package times),
including all 18 native Linux service cases, resolver and empty-root controls,
existing eligibility/session cases and HTTP error/status mapping. Scoped
golangci-lint reported zero issues. These results prove Linux service behavior
and mapping, not registered HTTP routing, Windows execution or native apps.
One conditional frozen install completed using pnpm 9.15.9 and Node 24.18.0.
Raw source/results and bounded process receipts are retained in
`/tmp/kandev-child49/`. Publication, hosted gates, merge and cleanup remain
externally pending; Task 01 stays `in_progress` until verified completion.

Implementation documentation checks passed: catalog validated 355 decisions and
1398 specifications, all-file specification lint and all 36 linter tests passed.
The real six-path changed inventory has production documentation coverage
`covered`, one work order and `errors=[]`; the exempt test/docs paths do not
substitute for the executed service tests. Public docs already describe opening
worktree files; no UI, copy or public documentation edit is required.

## Risks

- Both children touch different functions in `service.go`; ROOT must inspect
  current main/head/merge-tree, owned blobs and the shared contract near merge.
  Genuine conflicts checkpoint ROOT; do not mutate the sibling or rebase just
  because main advances.
- Native separators differ. Report the executing platform; Linux cannot prove
  Windows behavior. Existing symlink policy and native CLI interpretation remain
  residuals, not expanded claims.
