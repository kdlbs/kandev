---
id: "02-full-e2e-and-delivery"
title: "Run full E2E and deliver the repair"
status: in_progress
wave: 2
depends_on:
  - "01-restore-measured-geometry"
plan: "plan.md"
requirements:
  - REQ-UI-BOUNDED-CHANGES-001
acceptance_criteria:
  - AC-UI-BOUNDED-CHANGES-001.2
  - AC-UI-BOUNDED-CHANGES-001.6
  - AC-UI-BOUNDED-CHANGES-001.8
system_design:
  - ../../specs/ui/system-design/bounded-changes-rendering.md
---

# Task 02: Run Full E2E and Deliver the Repair

## Summary

Delegate the complete E2E project matrix to CI after the repair's focused checks
pass, following the user's later instruction to stop local testing. Capture the affected desktop/phone surface and carry the
current branch through PR checks, actionable reviews, and authorized merge.

## In scope

- CI desktop, mobile, routing, auth, and container project results.
- Two scoped test-setup repairs identified before local testing was stopped.
- Fresh desktop/phone screenshot evidence with synthetic data.
- Accurate final result records, commit, push, PR, CI/review remediation, merge.

## Out of scope

Broad unrelated refactors, new persistent Kandev tasks/sessions, release work,
extra local review/QA gates, bypassing required checks, or merging with failed
mandatory validation.

## Acceptance

- Required CI checks have terminal results for the final head; unsupported
  prerequisites, skips, and the cancelled local full run are reported explicitly.
- Fresh screenshots prove compact desktop spacing and phone layout/actions;
  PR checks and every actionable review thread are dispositioned.
- The PR merges only after required checks succeed and review requirements are
  satisfied, using the user's existing delivery authorization.

## Likely files

- This work order and `plan.md` for validation results.
- Task 01 files when a concrete E2E or PR finding requires remediation.
- `apps/web/e2e/tests/changes-panel-active-tab-highlight.spec.ts`.
- `apps/web/e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts`.
- Ignored `apps/web/.pr-assets/` for screenshots and its capture manifest.
- Existing `.github/pull_request_template.md` and publication skill instructions.

## Verification

The original matrix below describes the project coverage. The user's later
instruction is authoritative: stop local tests, repair observed failures, open
the PR, and let CI run tests. Do not resume these local commands or perform
local remediation replays. Await required current-head CI and review evidence.
Managed CI builds fresh production assets and retains resource limits.

```bash
(cd apps/web && pnpm e2e:run --project chromium)
(cd apps/web && pnpm e2e:run --project mobile-chrome)
(cd apps/web && pnpm e2e:run --project routing)
(cd apps/web && pnpm e2e:run --project auth)
(cd apps/web && pnpm e2e:run --project containers)
(cd apps/web && pnpm e2e:run --project kubernetes-compat)
```

Containers and Kubernetes require a real Docker daemon and their documented
image/Kind prerequisites. Verify availability before running; unavailable
projects remain a named coverage blocker. Do not label a default Chromium run
as the full suite or count skips as passes. Preserve every process handle,
terminal exit code, failed spec, count, and artifact/log path. Use resource-bounded
shards only when justified by the managed runner's memory budget.

Fix concrete failures using the corresponding skill and validate through CI.
If a remediation changes a shared boundary, reassess which completed full
projects need rerunning so all final claims refer to the final source.

## Capture and publication

1. Mark in progress only after Task 01 is done. Follow `/pr`, `/commit`, `/push`,
   and `/pr-fixup` in the primary session. The original user request authorizes
   delivery through merge and waiting for its checks; no repeated permission is
   needed after the separate repository implementation checkpoint is satisfied.
2. Capture synthetic commit-history screenshots using the existing `prCapture`
   fixture. Desktop and phone captures must follow this repair's rendered
   assertions. Validate/compress assets and preserve both project manifests across
   managed-runner cleanup. Keep binary PR media out of the merge branch.
3. Reconcile the requirement/design and mark the plan implemented after all
   required implementation checks pass. Record full E2E results accurately here.
4. Commit with Conventional Commits, push, and reuse any existing branch PR.
   Build the PR body from the repository template and run its documentation
   coverage preflight. Link the owning contract and both work orders.
5. Publish required screenshot embeds, verify the live body, then use
   `scripts/pr-await <PR>`, `scripts/pr-state --summary <PR>`, and
   `scripts/pr-resolve list <PR>` for terminal checks and review evidence.
   Follow skill limits for helper waits and queue-aware fixup pushes.
6. Address valid CI/review findings, retain the scoped validation evidence, and
   disposition every review thread. Do not bypass required review or CI gates.
7. Merge under repository policy after final evidence is current. Verify the
   GitHub merged state and merge commit, update delivery records, and report
   the merged PR URL plus full E2E outcome. If merge is blocked, state the exact
   blocker and keep this work order open.

## Dependencies

Task 01 completed with focused unit, desktop/mobile E2E, and static checks.

## Risks

Long-running suites, unavailable container prerequisites, flaky unrelated
fixtures, screenshot cleanup between project runs, stale PR evidence after
push/body edits, and merge-queue restrictions. Preserve source identity and
terminal results; do not infer success from another run.

## Parallelism

`sequential`

## Inputs

- [Task 01](task-01-restore-measured-geometry.md).
- `.agents/skills/e2e/SKILL.md` and its resource-safety reference.
- `.agents/skills/pr/SKILL.md`, commit/push/fixup skills, and `.github/AGENTS.md`.
- User instruction: fix changelist spacing, full E2E, lead to merge while AFK.

## Results

Publication handoff snapshot on 2026-09-30. Task 01 focused checks passed before
the user stopped further local testing. The partial full Chromium run was
cancelled at the user's request: 969 tests passed, two failed, and fourteen were
skipped before cancellation. This is not a successful complete-project result.
Both owned runner containers and their launcher process group were stopped;
logs and failure artifacts were preserved outside the repository.

Observed failures and scoped repairs:

- Active-tab highlight could not find `active-a.ts`: the preceding canvas specs
  left eighty untracked files in the shared seed checkout, placing the target
  outside the virtualized mounted range. The spec now uses the existing
  baseline-checkout reset before each scenario.
- Hide-disabled-profile navigation dereferenced `agents[0].profiles[0]` when
  the first API entry was Dynamic with no profiles. The spec now selects the
  Mock agent through the existing helper and correlates the profile with the
  current seeded profile ID.

These repairs preserve the existing UI assertions. No local test replay was
run after the stop instruction; CI must validate both fixes at the published
head. No production behavior outside the spacing repair changed.

The optional full-worker image recipe passed its input check but the build
failed while resolving a pinned Docker Hub image through an unreachable IPv6
registry endpoint. No image was published. The remaining local full projects
and prerequisite retries were cancelled; CI owns their test execution.

Desktop and phone captures were inspected and compressed, with manifests
preserved across managed-runner cleanup. The PR publication, current-head CI,
review dispositions, and authorized merge are tracked in the live task/PR;
those external delivery gates remain pending at this handoff snapshot.

PR #4088 review identified two assertion gaps: the controlled-observer test
could accept unchanged geometry before its refresh callback, and the browser
helper ignored nonconsecutive visible row indices. The test now waits for a
new measurement call after each trigger; the helper requires consecutive
visible indices and checks every visible adjacency. Offscreen retained focus
rows remain excluded by the existing viewport intersection filter. A comment
also states why estimated offsets are rebuilt before restoring mounted sizes.
These scoped review fixes are awaiting CI validation; no local tests were run.

Opt-in auth CI run 36760556845 tested PR head 2e96bc3cd with one worker and
retries disabled: 23 passed, two failed, one skipped, and six did not run.
The screenshot setup test displayed the login form, while share authorization
received setup HTTP 409. The worker-scoped fixture preserves its SQLite data
across backend restarts, so the preceding lifecycle suite had already consumed
single-shot auth setup. Both failing suites now use their own database paths,
matching the existing auth test isolation pattern. Assertions and production
auth behavior remain unchanged. Routing passed in the same CI run. The new
auth fixture repairs require fresh current-head CI; no local replay was run.

The lifecycle, organization-unit, and optional SSO suites also performed setup
on the preserved baseline database. They now own separate database paths too;
otherwise fixing the first failure would prevent Playwright's discarded-worker
restart from masking the next setup collision. All auth setup owners follow
the existing per-suite database pattern. The optional external Google OIDC
capture retains its existing package-absent CI skip. No assertions were relaxed.

The final-head frontend CI run passed 2,450 test files and 21,002 tests with
four skips. The isolated auth/routing run passed 31 auth tests (one existing
optional package-absent skip) and nine routing tests with retries disabled.
Standard E2E completed with 3,618 passes and 47 skips, but artifact auditing
exposed six scenarios that passed after seven failed attempts. External PR
detection inherited earlier LSP commits, and
two review scenarios inherited canvas files, placing expected rows outside the
mounted viewport. Those scenarios now reset the seed checkout before creating
their tasks. Copy-icon geometry now samples the current SVG and its visibility
in one browser operation, including an explicit copied-check-icon assertion.
The managed deletion scenario waits for its closing overlay and body pointer
lock to clear before reopening the task menu. Workflow selection now waits for
its closing picker and scheduled focus restoration before another picker opens.
All existing behavior assertions
remain; no forced clicks, page reloads, extra retries, or local test replays
were introduced. Fresh current-head CI must validate these scoped repairs.
