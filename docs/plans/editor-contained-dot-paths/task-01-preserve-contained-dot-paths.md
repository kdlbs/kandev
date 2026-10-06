---
id: "01-preserve-contained-dot-paths"
title: "Preserve contained dot paths"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-EDITOR-CONTAINMENT-001
acceptance_criteria:
  - AC-WORKSPACES-EDITOR-CONTAINMENT-001.1
  - AC-WORKSPACES-EDITOR-CONTAINMENT-001.2
  - AC-WORKSPACES-EDITOR-CONTAINMENT-001.3
  - AC-WORKSPACES-EDITOR-CONTAINMENT-001.4
system_design:
  - ../../specs/workspaces/system-design/editor-file-containment.md
---

# Task 01: Preserve Contained Dot Paths

## Summary

After ROOT review and a later same-primary implementation interrupt, add
permanent real-service regressions and correct only `resolveFilePath`'s lexical
parent-component guard. Retain ordinary, folder, normalized-contained and true
escape behavior with exact outputs and error categories.

## In scope

- New `service/editor_file_containment_test.go`, reusing existing minimal
  repository/settings fixtures and real temporary files.
- Existing guard in `service/service.go`; no neighboring function changes.
- Required scoped Go race and lint checks, reference/spec/whitespace checks,
  accurate results and native-platform qualification.
- If an existing cheap registered HTTP/service fixture is available, add exact
  200 target and 400 escape controls there; otherwise record its absence and
  retain `TestServiceErrorStatus` as mapping evidence. No fixture framework.

## Out of scope

The exclusions in the paired requirement apply. Child48's URL builder/parser,
worktree and docs are independently owned. No production or permanent tests
change during the design turn. No launches/builds/browser/E2E, broad Go replay,
symlink/security redesign, native CLI semantics, or unrelated cleanup.

## Acceptance

1. Permanent `TestOpenEditor_ContainedDotPaths` first fails for the two causal
   filenames with `ErrEditorConfigInvalid`, while ordinary/escape controls pass.
   GREEN uses actual existing files through real `Service.OpenEditor` and asserts
   exact decoded goto identity, line/column, resolved path and full source/read-back
   bytes. Do not copy or replay ROOT's disposable proof.
2. `TestResolveFilePath_ContainmentControls` proves native separator/normalization
   cases, contained dot names, empty-root/file behavior and exact parent escapes
   using explicit expected outcomes. Only the existing guard changes. No copied
   predicate, source-text assertions, test-only production hook, or weakened
   traversal/error/timeout/race assertions.
3. Exact scoped checks pass; unchanged eligibility/selection/folder/error mapping
   remain covered. Report registered routing and native-app/platform limitations
   candidly; synchronize this work order, manifest and spec statuses only after
   actual implementation evidence.

## Regression matrix

| Class | Cases | Expected result |
| --- | --- | --- |
| Causal contained | `..notes.go`, `..notes/inside.go` | Exact target and position; unchanged existing file bytes |
| Ordinary/dot controls | `src/plain.go`, `.hidden.go`, `...notes.go`, `src/..notes.go` | Contained target accepted |
| Contained normalization | `./..notes.go`, repeated native separators, `src/../..notes.go` | Expected normalized contained target |
| Parent escapes | `..`, `../outside.go`, `src/../../outside.go`, dot-directory followed by enough parents to escape | Exact `ErrEditorConfigInvalid`, empty target |
| Folder/missing root | Empty file, `.`, empty root; existing repository-less embedded folder | Existing folder or missing-workspace result |
| Native separators | `filepath.FromSlash`/native joins; Windows alternate separator only when Windows executes | Host-native expectations; no foreign-platform claim |

Construct separate contained/outside fixture files with distinct sentinel bytes;
verify all remain unchanged after service requests. Decode the returned sentinel
query for target assertions so child48's percent-encoding correction remains
compatible. A URL prefix alone is insufficient evidence. New tests belong in a
new file; `service_test.go` is already near the test-file size limit.

## Verification

Acquire the exclusive GLOBAL LOCAL-HEAVY lease from ROOT before product tests,
lint, installs or hooks. Read the TDD skill and backend fixture reference. Each
command below is a separate supervised shell with upfront retained native
handle, owned PID/group, start, absolute cutoff, argv and raw log. Actually join
and prove its group gone before the next command. No grant exists this turn.

From repo root, RED before the production correction:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 180s go test -trimpath -tags fts5 -race -p=1 ./internal/editors/service -run '^TestOpenEditor_ContainedDotPaths$' -count=1 -v)
```

Require causal assertion failure, not setup/compile/timeout failure. Then correct
the guard and run GREEN once against both affected existing packages:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 180s go test -trimpath -tags fts5 -race -p=1 ./internal/editors/service ./internal/editors/handlers -count=1 -v)
```

Scoped lint, without broad passing replay:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=512MiB /usr/bin/timeout --signal=TERM --kill-after=10s 360s /home/jcfs/.local/bin/golangci-lint run ./internal/editors/service ./internal/editors/handlers --build-tags=fts5 --concurrency=2 --allow-serial-runners --timeout=5m)
```

For an actual later backend code fixup, full CHANGED lint is required once
against the exact current PR base SHA; set `PR_BASE_SHA` from fresh PR evidence:

```bash
(cd apps/backend && GOMAXPROCS=2 GOMEMLIMIT=1GiB /usr/bin/timeout --signal=TERM --kill-after=10s 360s /home/jcfs/.local/bin/golangci-lint run ./... --build-tags=fts5 --new-from-rev="${PR_BASE_SHA:?exact current PR base SHA required}" --concurrency=2 --allow-serial-runners --timeout=5m)
```

Lightweight design/traceability gates are separately bounded to 60 seconds each:

```bash
/usr/bin/timeout --signal=TERM --kill-after=10s 60s python3 scripts/list-docs.py validate
/usr/bin/timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.py --all
/usr/bin/timeout --signal=TERM --kill-after=10s 60s python3 scripts/lint-spec-files.test.py
/usr/bin/timeout --signal=TERM --kill-after=10s 60s git diff --check -- docs/specs/workspaces docs/plans/editor-contained-dot-paths apps/backend/internal/editors
```

Invoke the repository `.github/scripts/pr-docs.cjs` `validateCoverage` preflight
on the actual package bytes, supplying the planned owned production path plus
changed work order so reference validation cannot short-circuit to docs-only
`exempt`. Record this as planned-source reference coverage, not production-test
coverage. After implementation use the actual changed paths. Validate every
REQ/AC/design/plan linkage and inventory all four package files, including
untracked whitespace, before handoff.

Current `apps/node_modules` is absent. Later only, under the exclusive lease,
perform one frozen install from `apps` with pnpm 9.15.9 if still absent, using
existing `/home/jcfs/.nvm/versions/node/v24.18.0/bin` and Bash `login=false`.
Do not install during design. Keep normal hooks and Conventional new commits;
no amend or bypass. Routine own-fixture/lint repairs are affected-only;
timeout/resource/transport/unknown/out-of-scope failures checkpoint ROOT with
saved evidence and no automatic retry.

## Files likely touched

- `apps/backend/internal/editors/service/service.go` (`resolveFilePath` only).
- `apps/backend/internal/editors/service/editor_file_containment_test.go` (new).
- An existing cheap registered-route fixture only if one is found; today only
  `apps/backend/internal/editors/handlers/handlers_test.go` mapping tests exist.
- The four files in this design package for lifecycle/results updates.

## Dependencies and parallelism

No prior work order. `sequential`. Later same-primary ROOT implementation
interrupt and exclusive lease are required; these files do not authorize
delegation. ROOT delivery/merge constraints in the manifest and Kandev plan
persist without extra routine approval prompts.

## Inputs

- [Owning requirement](../../specs/workspaces/requirements/editor-file-containment.md).
- [Owning design](../../specs/workspaces/system-design/editor-file-containment.md).
- [Manifest](plan.md), root/backend `AGENTS.md`, TDD skill and
  `.agents/skills/tdd/references/backend-tests.md`.
- Accepted ROOT proof archives listed in the manifest, read-only.
- Existing service fixtures, URL/args tests and HTTP error mapping inventory.

## Risks

Native-platform semantics and lexical symlink residuals remain qualified. Static
shared-file compatibility with child48 must be checked near merge; genuine
conflicts checkpoint ROOT. No rebase merely for main advancement.

## Results

Historical test authoring completed under ROOT's limited post-design release on
2026-10-06. The new owned file is
`apps/backend/internal/editors/service/editor_file_containment_test.go`, with
`TestOpenEditor_ContainedDotPaths` and
`TestResolveFilePath_ContainmentControls`. The matrix contains 18 native POSIX
cases or 19 native Windows cases, plus direct empty-root controls. These are
authored counts, not execution results. Real service tests construct actual
contained files/directories and an outside sibling file with distinct full
byte sentinels; each request retains an unchanged-byte cleanup assertion.
Folder, dot-name, normalization and complete-parent escape controls are explicit.

ROOT confirmed the approved compatible sentinel forms: legacy `goto` contains
the complete expected native relative name plus `:7:3`; the new query has the
exact expected name in `goto` and separate `line=7`/`column=3`. Assertions parse
the query, reject extra/duplicate keys, and compare the complete tuples without
generic colon splitting or raw percent-encoding equality. No sibling source,
tests or docs were edited or replayed.

Read-only fixture audit found no existing cheap registered HTTP/service fixture.
Routing, native apps and cross-platform execution have not been exercised.

Under ROOT's later exclusive implementation release, permanent real-service
RED failed on seven contained-name cases and passed eleven ordinary, folder
and true-escape controls. The authored test source remained unchanged through
GREEN (SHA256 `37f84670ac7e3935ec0c9fad492bed091303c1c8f727e15dbe12a23f39a31dda`).
The production correction changes only the existing guard to reject an exact
parent component or parent component followed by the native separator.

Affected service and handler tests passed with `-trimpath -tags fts5 -race -p=1`,
GOMAXPROCS=2 and GOMEMLIMIT=512MiB: service package 1.032s, handlers 1.035s.
All 18 native Linux service cases, resolver and empty-root controls, existing
session/eligibility tests and seven error/status mapping cases passed.
Scoped golangci-lint with concurrency 2, serial-runner admission and a five-minute
CLI bound reported zero issues. No production predicate copy, source-text oracle,
registered-route execution, native app or Windows execution claim is made.

One conditional frozen install completed with pinned pnpm 9.15.9 and Node
24.18.0. Evidence is retained in `/tmp/kandev-child49/`; original native handles,
absolute cutoffs, raw logs, actual joins and gone checks accompany the receipts.
The ROOT proof remains untouched. Publication and hosted verification are pending;
Task 01 remains `in_progress` until separate ROOT merge admission and verified
merge/cleanup. Design END and local GREEN are not task completion.

The implementation catalog, all-file specification lint, 36 linter tests and
actual six-path production reference coverage passed. The coverage result is
`covered` with one work order and `errors=[]`. Public documentation impact was
assessed: existing developer-tools guidance already covers this operation.
