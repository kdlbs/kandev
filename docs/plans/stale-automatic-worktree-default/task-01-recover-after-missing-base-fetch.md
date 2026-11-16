---
id: "01-recover-after-missing-base-fetch"
title: "Recover after a missing automatic base fetch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-WORKTREE-BASE-REFRESH-001
acceptance_criteria:
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.1
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.3
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.6
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.8
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.10
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.13
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.14
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.17
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.18
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.19
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.25
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.26
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.27
  - AC-WORKSPACES-WORKTREE-BASE-REFRESH-001.28
system_design:
  - ../../specs/workspaces/system-design/worktree-base-selection.md
  - ../../specs/workspaces/system-design/worktree-base-refresh.md
---

# Task 01: Recover after a missing automatic base fetch

## Summary

Restore automatic/inherited base recovery after a host fetch proves the selected
remote ref absent. Require successful refresh and verification of the replacement,
and retain strict selection and unrelated-failure behavior.

Issue: [#5335](https://github.com/kdlbs/kandev/issues/5335).
See the [plan](plan.md#confirmed-root-cause-and-evidence) for the confirmed reproduction.

## In scope

- Reuse `ErrRemoteRefMissing` to expose a confirmed missing remote base.
- Route only eligible non-PR host-fetch failures into existing default recovery.
- Continue into existing default recovery without changing candidate selection.
- Require refreshed verified candidates after the required-fetch failure.
- Add focused manager regressions and run existing launch controls.
- Update the two public refresh explanations with the implemented behavior.

## Out of scope

- Changing provenance producers, metadata, persistence, or repository defaults.
- PR, provider-refresh, remote-executor, valid-reuse, or empty-remote redesign.
- New retries, credentials, settings, public fields, dependencies, or frontend work.
- Live task retries, repository repair, branch resets, or instance restarts.

## Acceptance

1. Real `Manager.Create` recovers automatic and inherited stale defaults with
   refresh enabled, records the actual base, and checks out the verified candidate
   commit. Its warning names both branches and final sync progress completes.
2. Only confirmed `ErrRemoteRefMissing` and the exact attachment's permissive
   policy permit substitution. Forbidden original or candidate failures create
   no alternate worktree, callback, stored row, or successful runtime admission.
   Explicit and qualified selections, same-ref local fallback, and existing
   provider/PR routes retain their contracts.
3. Existing launch and mixed-attachment controls pass. Public summaries and
   recovery guidance agree with the corrected behavior. Production edits stay
   in the two existing manager files, without new error types or recovery modules.

## Implementation notes

Load `/tdd` before implementation. Set this work order to `in_progress`.
First add and run
`TestCreateWorktree_AutomaticDefaultMissingRemoteRecovers` in the new manager
test file. The refresh-enabled case must fail with the reported error before
production changes.

Include the existing `ErrRemoteRefMissing` in the safe error cause returned by
`handleBaseFetchFailure` after confirmed absence and context checks.
Keep `ErrGitCommandFailed` available. Add no new error type.
Do not use `newConfirmedRemoteRefMissingError`, since it also introduces
`ErrInvalidBaseBranch` into required-refresh errors.
Do not infer it from `ErrGitCommandFailed`, progress text, or diagnostic-only codes.

In `resolveBaseRefWithFallback`, gate the refresh-error recovery before the
unconditional error return. Keep ordinary strict requests and PR/provider routes
on their existing paths. Continue through the current configured/live-default
sequence in place. Do not extract a new resolver module or add candidate iteration.

Give the existing `resolveFallbackRef` a local argument for required refresh on
this recovery path. Reuse `pullBaseBranchWithPolicy` and its verification checks.
Keep existing candidate ordering and failure behavior.
When recovery reaches the remote default, discover its currently advertised
branch independently of cached `origin/HEAD`. Refresh the replacement into its
explicit remote-tracking ref so a narrow fetch mapping cannot leave an older
candidate commit selected.
Verify the resulting ref and containing commit before using existing finishers.
Do not rebind a base or record success before that verification.
Keep other failure causes credential-safe, including live-default lookup errors.

Cover the following cases with the test names mapped in the plan:

- Automatic and inherited permission, refresh enabled and disabled, and a stale
  saved default equal to the requested base.
- A default commit published after cloning, proving freshness rather than only
  matching the branch label. Include a replacement absent locally.
- A deleted `origin/<base>` whose stale remote-tracking ref remains cached must
  not satisfy a recovery path that requires a refreshed fallback.
- A stale cached `origin/HEAD`, or an absent `origin/HEAD` with an uncached new
  default, must not block discovery of the origin-advertised default.
- A narrow `remote.origin.fetch` mapping must not let a required fallback use
  an older cached tracking-ref commit.
- A local replacement whose fetch fails. It must not bypass required refresh.
- Auth, network, generic error, timeout, cancellation, lock exhaustion, and
  ref/ancestry uncertainty on the requested branch or replacement as applicable.
- Explicit selection, including a same-name saved default, still fails.
  Reuse existing metadata and manual-override tests rather than duplicating them.
- No alternate worktree effects on failure. Credential-safe warnings and errors,
  unchanged causes, and a completed final sync event on successful recovery.

Real `Manager.Create` and native Git provide the new integration evidence.
Run existing `TestBaseBranchSelection_*` backendapp tests for startup admission
and mixed-attachment controls. No new launch harness or producer test is needed
because those paths do not change.

Update the refresh summary and selected-base section in `git-operations.md`.
Update the host Worktree paragraph in `executors.md`.
These are explanation and reference pages respectively. No rendered UI changes
or mobile Playwright scenario are required for this backend correction.

## Verification

Run from the repository root. The first command is the red/green regression.
Record its expected failure before implementation, then its passing result.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/worktree -run '^TestCreateWorktree_AutomaticDefaultMissingRemoteRecovers$' -count=1 -v)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/worktree -count=1)
(cd apps/backend && go test -trimpath -race -tags fts5 ./internal/backendapp -run '^TestBaseBranchSelection_' -count=1)
bash scripts/lint-go-changed apps/backend/internal/worktree/manager_git.go apps/backend/internal/worktree/manager_lifecycle.go apps/backend/internal/worktree/manager_automatic_base_recovery_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
make -C apps/backend build
git diff --check
git status --short
```

Also run the [plan's PR-documentation preflight](plan.md#package-validation).
All tests in changed suites must run. Include any additional changed Go file in
the lint command. Record exact outcomes before marking this task `done` and
synchronizing the plan. Keep package-validation results separate from product tests.

## Files likely touched

- `apps/backend/internal/worktree/manager_git.go`
- `apps/backend/internal/worktree/manager_lifecycle.go`
- `apps/backend/internal/worktree/manager_automatic_base_recovery_test.go` (new)
- `docs/public/git-operations.md`
- `docs/public/executors.md`
- `docs/public/coverage.json`
- `docs/plans/stale-automatic-worktree-default/plan.md`
- `docs/plans/stale-automatic-worktree-default/task-01-recover-after-missing-base-fetch.md`

## Dependencies

None. Reuse the existing selection model, fixture helpers, and preparation wiring.

## Risks

- Recovering from broad errors weakens explicit-selection and failure boundaries.
- Reusing best-effort fallback fetch can admit stale replacement state.
- Repeated candidates can obscure the original failure or create retry loops.
- A task-wide policy can incorrectly authorize a sibling attachment.

## Parallelism

`sequential`

## Inputs

- [Selected base identity](../../specs/workspaces/requirements/worktree-base-refresh.md#selected-base-identity), especially criteria .25 through .28.
- [Host refresh recovery](../../specs/workspaces/system-design/worktree-base-selection.md#host-refresh-recovery).
- [Refresh and fallback design](../../specs/workspaces/system-design/worktree-base-refresh.md#missing-base-fallback).
- [Explicit-selection ADR](../../decisions/2026-11-07-explicit-worktree-base-selection.md).
- [Local-refresh ADR](../../decisions/2026-08-31-local-worktree-refresh-best-effort.md).
- `manager_live_default_test.go`, `manager_explicit_base_test.go`, and `manager_stacked_pr_base_fallback_test.go` in `internal/worktree`.
- `base_branch_selection_integration_test.go` in `internal/backendapp`.

## Results

RED/GREEN:

- Before production changes, the work-order regression command exited 1 on the
  refresh-enabled case with the expected `missing_remote_ref` failure. The
  refresh-disabled control passed.
- After the fix, the same command passed both refresh modes.
- `go test -trimpath -tags fts5 ./internal/worktree -run '^TestCreateWorktree_AutomaticDefault' -count=1 -v` passed the recovery, freshness, failure, and strict-selection cases.
- The deleted-remote regression was red before the review correction: `Create`
  selected cached `origin/retired-base` at the sentinel commit. After skipping
  that cached-ref success during required-fallback recovery, the requested
  targeted command passed, including refreshed `main`, the substitution warning,
  the default commit, and completed final sync progress.
- PR review regressions were red before correction: stale `origin/HEAD` returned
  the deleted request, an absent symref could not name an uncached default, and
  a narrow fetch mapping left the selected fallback at its older commit.
- After adding authoritative origin-default discovery and an explicit tracking
  refspec for remote-only refresh, the targeted `TestCreateWorktree_AutomaticDefault`
  command passed all three review cases and the existing recovery controls.

Required validation:

- `go test -trimpath -race -tags fts5 ./internal/worktree -count=1`: passed.
- `go test -trimpath -race -tags fts5 ./internal/backendapp -run '^TestBaseBranchSelection_' -count=1`: passed.
- Changed-file Go lint reported zero issues for the two manager files and the new regression file.
- `node --test scripts/validate-public-docs.test.mjs`: 63 tests passed.
- `node scripts/validate-public-docs.mjs`: 55 published pages validated.
- `python3 scripts/list-docs.py validate`: 453 decisions and 2,018 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- PR-documentation preflight: `covered`, with no errors.
- `make -C apps/backend build`: passed. Darwin agentctl helper artifacts remained unsigned because codesign tools were unavailable.
- `git diff --check`: passed.

No commit, push, PR, issue update, or live task retry was part of this work.
