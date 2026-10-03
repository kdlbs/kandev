---
id: "01-select-available-remote-source"
title: "Select a cloneable source after local deletion"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REMOTE-RESOLUTION-001
acceptance_criteria:
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.1
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.2
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.3
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.4
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.5
  - AC-WORKSPACES-REMOTE-RESOLUTION-001.6
system_design:
  - ../../specs/workspaces/system-design/remote-repository-resolution.md
---

# Select a cloneable source after local deletion

## Summary

Repair remote selection at the task service boundary and prove that the selected
registration reaches managed checkout preparation without modifying the
deleted local source. Execute sequentially in the primary session.

## Scope

- Add the failing real-checkout/deletion service regression before implementation.
- Add a private candidate eligibility/selection helper; invoke it from remote
  `FindOrCreateRepository` requests under the existing resolution mutex.
- Preserve current creation, backfill, identity, and rollback contracts.
- Cover every candidate and failure case listed in the plan's test matrix.
- Prove real service resolution, persisted attachment, managed preparation, and
  worktree creation with a local Git origin and fake final agent boundary.

## Exclusions

- Production executor changes, SQL/schema changes, and rendered UI changes.
- Existing-task relocation, credential redesign, and local folder restoration.
- Copying skipped local registration settings or secrets into managed fallback.

## Acceptance conditions

1. The deletion regression fails before the correction and passes afterward;
   the integration reaches a valid managed workspace with the requested branch.
2. Mixed, repeated, and concurrent selections converge on an eligible matching
   source while preserving skipped local rows, links, paths, and contents.
3. Existing explicit-local and provider-clone controls pass; identity and
   inspection failures cannot select a foreign candidate or start an agent in
   the deleted path.

## Verification

Run from the repository root. The first command is the Red-stage check and
must fail for deleted-local adoption before the implementation change.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^TestResolveRepositoryRef_RemoteSelectionSkipsDeletedLocalCheckout$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service ./internal/orchestrator/executor -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record the Red result, final package results, and documentation cross-reference
coverage preflight in Results. Use `validateCoverage` from
`.github/scripts/pr-docs.cjs` against the changed work order, linked plan,
requirement/design contents, and actual changed application paths. Promote the
paired specs only after implementation and all checks pass.

## Files likely touched

- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/remote_repository_resolution.go` (new)
- `apps/backend/internal/task/service/remote_repository_resolution_test.go` (new)
- `apps/backend/internal/task/service/remote_repository_admission_test.go` (review regressions)
- `apps/backend/internal/orchestrator/executor/executor_remote_selection_integration_test.go` (new)
- `apps/web/e2e/helpers/github-origin.ts` and its test (offline provider fixture setup).
- Desktop GitHub URL, subtask, and external file-link E2E fixtures.
- Mobile external file-link fixture and shared fork-PR launch fixture.
- Navigation fixture preparation and history spacing E2E assertions (CI remediation).
- This work order, `plan.md`, and the paired requirement/design lifecycle fields.
- `docs/public/tasks-and-workflows.md` (remote selection recovery guidance).

## Dependencies

None.

## Risks

Use raw workspace rows for fallback, preserving existing SQL identity semantics
and creation ordering. Only a confirmed absent directory qualifies for this
repair. The integration fixture must use owned temporary files, close its
database, and stop every service it starts. No developer runtime or internet
clone is needed. Skip permission assertions when the host bypasses permissions.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/workspaces/requirements/remote-repository-resolution.md)
- [Design](../../specs/workspaces/system-design/remote-repository-resolution.md)
- `service_resources_test.go` and `provider_scope_test.go` for service patterns.
- `executor_resume_clone_default_branch_test.go` for clone/worktree fixtures.
- `.agents/skills/tdd/references/backend-tests.md` for fixture isolation.

## Results

Completed in the primary session without delegated agents.

- Red: the documented deletion-regression command failed because it selected
  the original deleted local registration (`created=false`, `source=local`).
- Green: `go test -trimpath -tags fts5 ./internal/task/service -run
  '^TestResolveRepositoryRef_RemoteSelection' -count=1` passed.
- Final package check: the documented `go test -trimpath -tags fts5 -race
  ./internal/task/service ./internal/orchestrator/executor -count=1` passed
  after the final production refactor (service: 60.026s; executor: 10.871s).
- Additional scope case: `go test -trimpath -tags fts5 -race
  ./internal/task/service -run
  '^TestResolveRepositoryRef_RemoteSelectionScopedFallback$' -count=1 -v` passed.
- `golangci-lint run ./internal/task/service ./internal/orchestrator/executor
  --new-from-rev=HEAD --timeout=5m`: zero issues.
- `python3 scripts/list-docs.py validate`: passed (343 decisions, 1313 specifications).
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: all 62 tests passed.
- `node scripts/validate-public-docs.mjs`: passed (47 published pages).
- `validateCoverage` from `.github/scripts/pr-docs.cjs`, with actual tracked
  and untracked changed paths plus the linked document contents: `covered`.
- `git diff --check`: passed.

Node commands used the installed v24.18.0 binary because Node was absent from
the shell PATH. Public docs retain their task-oriented how-to format. No
production executor, API, schema, or UI changes were necessary.

### Review remediation results

- Red: `go test -trimpath -tags fts5 ./internal/task/service -run
  '^TestResolveRepositoryRef_RemoteSelection(UnscopedAdmission|RejectsChangedOrigin)$'
  -count=1` failed on both findings before the correction.
- Green: all `TestResolveRepositoryRef_RemoteSelection` cases passed.
- Final: `go test -trimpath -tags fts5 -race ./internal/task/service
  ./internal/orchestrator/executor -count=1` passed (65.532s and 11.317s).
- `golangci-lint run ./internal/task/service ./internal/orchestrator/executor
  --new-from-rev=HEAD --timeout=5m` before the remediation commit: zero issues.
- Spec catalog validation, full spec lint, validation of 47 public docs pages,
  and whitespace checks passed after the requirement/design/public-doc updates.

Added `remote_repository_admission_test.go` for initial and fallback scope
isolation, changed origin rejection with row preservation, and compatible
SSH/HTTPS reuse. Local fixtures now carry a matching origin. Remote CI and
review for the remediation remain pending until publication.

### CI fixture remediation results

E2E run 37072282028, attempt 1 at head `a8a89ba9`, reported three failures
in shard 14. The file-link and pasted-URL subtask fixtures advertised GitHub
identities without matching checkout origins. The subtask trace confirmed the
same HTTP 400 origin-validation error as both file-link tests.

- Red: the three exact failing tests reproduced in the CI runtime image with
  one worker and `--retries=0` against a fresh managed production build.
- Green: the same three tests passed (24.5s) after adding matching origins to
  their disposable repositories. Production validation remains unchanged.
- Command: from `apps/web`, run `pnpm e2e:run --docker --no-build
  tests/review/external-vcs-file-link.spec.ts tests/task/subtask.spec.ts --
  --grep 'opens a linked pull request file|uses the base branch for an existing
  file|user creates subtask via pasted GitHub URL' --retries=0`, with the grep
  pattern on one line. The initial Red run omitted `--no-build`.

The relevant fixtures and validation code are identical on the PR head and the
CI merge result (`2eaf4230`, head plus base `8a1229e6`). No public behavior,
requirements, or system-design change is needed for this fixture correction.

The final CI report identified 13 failed tests across shards 1, 9, 13, and 14,
all with this origin mismatch. Both aggregate failures were downstream shard
gates, with no additional assertion. The remaining desktop GitHub URL and
mobile fixtures reproduced their failures with retries disabled before setup
changes. Offline provider fixtures now expose canonical origins while using
checkout-local URL rewrites for push/fetch against their disposable bare
repositories. Shared checkout origins and rewrite entries are restored after
each test. The missing-PR-snapshot case uses an empty offline origin and still
asserts launch failure.

- Full desktop GitHub URL spec: 10 passed (1.4m) with the describe-level retry
  temporarily set to zero and CLI `--retries=0`; restored the existing retry
  policy after verification.
- Mobile file-link and fork-PR launch cases: 2 passed (15.4s), using
  `pnpm e2e:run --docker --no-build --project mobile-chrome
  tests/task/mobile-external-vcs-file-link.spec.ts
  tests/task/mobile-create-task-remote-repo.spec.ts -- --grep
  'opens the provider file|starts a target-attached fork PR' --retries=0`.
- `pnpm exec vitest run e2e/helpers/github-origin.test.ts`: one passed, proving
  offline push and cleanup while preserving an unrelated rewrite entry.
- Existing phone touch-size, overflow, branch, and persisted-state assertions
  remain intact; no rendered UI or mobile composition changed.
- Changed-file ESLint and Prettier checks passed. Web typecheck passed.
- Playwright discovery for Chromium, mobile, and containers reported zero errors.
- Documentation coverage preflight returned `covered`; catalog validation,
  full specification lint, and whitespace checks passed.

### Additional CI remediation

Run 37081307204 on fixture commit `a8cfb8d13` exposed two setup/readiness
defects after the origin failures were corrected. Navigation tasks started
concurrent branch preparation in one local checkout, producing `index.lock`
failures in all three CI attempts. The fixture now settles each initial turn
before starting the next task. A helper regression failed before this change
and passed afterward (`pnpm exec vitest run
e2e/helpers/task-navigation-helpers.test.ts`, one test).

The history spacing test sampled rows before observer measurement. Holding
timeline ResizeObserver delivery reproduced the CI geometry: four 8px sibling
gaps and a 16px section gap. Releasing delivery restored 2px/10px/-4px. The
diagnostic was removed; the assertion now polls the same expected geometry
and precision, without adding sleeps or increasing timeouts.

- The preceding shard-14 spec sequence passed all 56 tests with one worker,
  retries disabled, and the CI runtime limited to two CPUs and 4GiB.
- All six affected history/navigation desktop and mobile specs passed all 17
  tests in 2.7 minutes under the same limits and with retries disabled.
- The helper regression, web typecheck, changed-file ESLint, Prettier, and
  whitespace checks passed.

These are test setup and measurement corrections. Production behavior, public
copy, and the durable repository-selection contract are unchanged. Final
remote CI and review evidence are recorded in the task plan and PR checks.

### Unit-test discovery boundary

Full CI catalog discovery exposed that the new Vitest fixture regression was
inside Playwright's browser-test root. It was relocated to `e2e/helpers/`,
alongside the existing unit-helper tests. The moved regression passes, and
full Chromium/mobile/container discovery reports zero errors. Duration-aware
manifest generation also passes with CI's selected timing profile. This
changes test ownership only; the browser fixtures and runtime are unchanged.
