---
spec: docs/specs/ui/requirements/pr-only-commit-details.md
created: 2026-08-04
status: implemented
---

# Implementation Plan: Repair PR-only commit details

## Overview

Make commit provenance a first-class part of the Changes panel. Preserve the
local session path for SHAs known to the worktree, but route PR-only SHAs to a
new workspace-authorized GitHub individual-commit action. Hide unmeasured row
statistics and local mutation controls for remote commits, then cover the
force-pushed/stale-worktree case on desktop and mobile.

## Confirmed root cause

For task `8531031a-9a98-439f-ab0a-c25b480158dc`, the stored session worktree
was created at local head `5c2873d`, while linked PR #2253 was later
force-pushed to six different commits ending at `df3c05e`.

- `mergeCommits` in
  `apps/web/components/task/changes-panel-helpers.ts` appends PR-only SHAs to
  the local commit feed without preserving their source.
- GitHub's pull request commit-list response does not include commit stats.
  `convertRawPRCommits` leaves Go integer fields at zero, so the frontend
  presents unknown values as `+0 -0`.
- Every row click is reduced to a SHA and repository name, then sent through
  `session.commit_diff`. The backend runs `git show` in the task worktree even
  for PR-only SHAs.
- The shared row also exposes local reset/revert/amend callbacks for PR-only
  commits, while `CommitDetailPanel` resolves metadata only from local session
  commits.

The repair invariant is: row statistics, detail data, metadata, and available
actions must all derive from the same explicit commit source.

## Backend

### Workspace-scoped GitHub commit detail

- Add a `github.pr_commit.get` WebSocket action in
  `apps/backend/pkg/websocket/actions.go` and register it in
  `apps/backend/internal/github/handlers.go`.
- Add `StatsAvailable bool` with JSON field `stats_available` to `PRCommitInfo`.
  Define `PRCommitDetail` in `apps/backend/internal/github/models.go` with SHA,
  message, author login/name/date, exact additions/deletions, files changed,
  and the merged `[]PRFile` returned by the individual GitHub commit endpoint.
- Extend the GitHub client interface and the `gh`, PAT, noop, and mock clients
  with an exact-SHA commit-detail method. The CLI client should call
  `gh api repos/{owner}/{repo}/commits/{sha}`; the PAT client should call the
  equivalent REST path. Parse both through one helper so their response
  semantics stay aligned.
- Request the maximum supported page size and merge every returned file page
  for both clients (`gh api --paginate --slurp` for the CLI representation and
  Link-header pagination for PAT). Preserve the first page's metadata/stats,
  append file records in provider order without duplication, and test a
  multi-page response.
- Add a service method that reuses `ensureRepositoryInWorkspaceScope` and the
  existing personal read-client resolution. Validate that the SHA is a safe
  exact object ID before interpolating it into either client path.
- Do not fetch, persist, or import the commit into the task worktree.

### Unknown list statistics

- Use `PRCommitInfo.stats_available` to explicitly represent whether additions,
  deletions, and files-changed values are available. The normal GitHub list
  parser must set it to false rather than relying on zero values.
- Keep exact integers mandatory on `PRCommitDetail`, where the individual
  endpoint provides them. This lets a genuinely empty commit remain
  distinguishable from an unmeasured row.

## Frontend

### Source-aware commit targets

- Introduce a discriminated commit-detail target: local targets contain the
  existing SHA/repository routing data; GitHub targets additionally contain
  workspace, owner, and repository identity.
- Make `mergeCommits` return that target plus a statistics-availability flag.
  A SHA found in both feeds remains one local row and retains its pushed state.
  A PR-only SHA becomes a GitHub target and does not acquire zero statistics.
- Thread the full target through the Changes panel callbacks, desktop dockview
  panel params/actions, and mobile diff target. Include source and repository
  identity in dockview panel identity so equal SHAs from different repositories
  cannot collide.

### Unified detail loading and rendering

- Add a source-aware commit-detail hook/request layer. Local targets continue
  to use `session.commit_diff`; GitHub targets call `github.pr_commit.get` and
  map `PRFile` records into the current diff-view model.
- Have `CommitDetailPanel` obtain metadata and files from that unified result,
  fixing the missing PR-only header as well as the patch source.
- For GitHub targets, disable worktree-dependent file actions and local context
  expansion. Preserve the current rendering for available, binary, empty, or
  truncated GitHub patches; never retry through the local git path.
- Gate reset/revert/amend row actions on local provenance. Render additions and
  deletions only when the row marks them as measured.

## Desktop and mobile behavior

The visual composition does not change. Desktop continues to open the existing
dockview commit panel. Mobile continues to use
`MobileChangesPanel` -> `MobileDiffSheet`, the current full-height drawer and
closest native-mobile exemplar. Both surfaces consume the same discriminated
target and detail hook. The mobile drawer keeps its existing single scroll
owner, close affordance, and touch behavior; only its data source and local
action eligibility change.

## Tests

- Backend parser tests prove list statistics are unavailable and individual
  commit responses preserve exact metadata, stats, file status, and patch.
- `gh` and PAT client tests prove the exact repository/SHA path and shared
  response parsing.
- Handler/service tests prove workspace/repository authorization, exact-SHA
  validation, response routing, and error propagation.
- Frontend merge tests prove PR-only provenance, hidden unknown stats,
  multi-repository identity, and local precedence for a SHA found in both
  feeds.
- Request/hook tests prove mutually exclusive local and GitHub actions and no
  local fallback after a GitHub error.
- Panel/row tests prove remote metadata is visible and reset/revert/amend,
  worktree-file actions, and local expansion are absent for GitHub targets.
- Dockview and mobile target tests prove source/repository identity survives
  serialization and selection.

All production changes follow Red-Green-Refactor: add each focused regression,
record the expected failure against current behavior, add the smallest source
change, then rerun the focused and package-level checks.

## E2E tests

Extend `apps/web/e2e/tests/git/git-changes-panel.spec.ts` with a linked PR whose
mocked current commits are absent from the stale local worktree. Assert that the
row omits false `+0 -0`, its local mutation menu is absent, and opening it shows
the GitHub-specific commit header and patch marker. The scenario must fail if
the UI invokes `session.commit_diff` for that SHA.

Extend `apps/web/e2e/tests/task/mobile-changes-panel.spec.ts` with the same
PR-only source. Assert that tapping the row opens the existing full-height
commit sheet, renders the remote marker patch, exposes no local action, closes
normally, and introduces no horizontal overflow. Add deterministic mock GitHub
individual-commit seeding to the existing mock controller/API helper rather
than depending on live GitHub.

## Implementation waves and parallel candidates

Wave 1 (sequential):

- [x] [Task 01: GitHub commit detail contract](task-01-github-commit-detail.md)
  — add the authenticated individual-commit backend contract and distinguish
  unknown list statistics.

Wave 2 (depends on Wave 1):

- [x] [Task 02: Source-aware commit UI](task-02-source-aware-commit-ui.md) —
  route local and GitHub targets correctly across desktop and mobile.

Wave 3 (depends on Waves 1-2):

- [x] [Task 03: Commit-source E2E coverage](task-03-commit-source-e2e.md) —
  exercise stale local history against deterministic remote commit details.

The tasks are intentionally sequential. Task 02 consumes the backend wire
contract from Task 01, and Task 03 validates the integrated behavior and adds
mock support for that settled contract.

## Verification

Run the focused commands recorded in each task, then finish with:

```bash
cd apps/backend && go test ./internal/github
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/git/git-changes-panel.spec.ts -- --grep "PR-only commit"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep "PR-only commit"
git diff --check
```

## Verification results

- Backend: `go test ./internal/github` passed (1081 tests).
- Frontend: focused source/target suite passed (7 files, 72 tests), followed by
  `pnpm run typecheck`, web lint, `i18n:check`, and `i18n:ratchet`.
- E2E: the desktop PR-only scenario passed once; the mobile scenario passed
  once after narrowing an assertion to the opened sheet. Both scenarios cover
  stale local history, hidden unknown statistics/actions, remote metadata, and
  remote patch rendering.
- `git diff --check` passed.
- Review remediation: complete PR identity now gates visible commit-list
  results; unavailable or malformed detail responses show a persistent retry
  state; and remote detail loading is independent of local agent readiness.
  The broader focused frontend suite passed (8 files, 69 tests), plus the
  remediation request/hook/panel tests (4 files, 15 tests), typecheck, lint,
  i18n checks, and ratchet.
- Review follow-up after rebasing onto `main`: root repository identity is now
  matched exactly, invalid/null commit payloads are rejected at both backend
  and frontend boundaries, target switches cannot expose prior detail state,
  and serialized GitHub targets require complete identity. Backend GitHub tests
  passed (1,086); focused frontend tests passed (8 files, 73 tests), with
  typecheck, lint, i18n checks, ratchet, and diff checks green.

## Risks and out of scope

- Individual GitHub commit requests add a lazy network round trip and can be
  rate-limited. The existing loading/error treatment must remain responsive,
  and no eager N+1 list loading is allowed.
- GitHub can omit patches for binary or oversized changes. That state must be
  presented honestly and must not trigger a local-data fallback.
- The source target becomes part of desktop panel identity and mobile
  selection state. Tests must prevent stale serialized targets or
  cross-repository SHA collisions.
- No database migration, public documentation change, worktree synchronization,
  or non-GitHub provider is part of this repair.
