---
id: "04-integration-and-docs"
title: "Prove integration and document usage"
status: complete
wave: 4
depends_on:
  - "03-question-image-interface"
plan: "plan.md"
requirements:
  - REQ-TASKS-QUESTION-IMAGES-001
  - REQ-TASKS-QUESTION-IMAGES-002
  - REQ-TASKS-QUESTION-IMAGES-003
  - REQ-TASKS-QUESTION-IMAGES-004
  - REQ-TASKS-WIP-LIMIT-PULL-SYSTEM-001
acceptance_criteria:
  - AC-TASKS-QUESTION-IMAGES-001.3
  - AC-TASKS-QUESTION-IMAGES-001.4
  - AC-TASKS-QUESTION-IMAGES-002.1
  - AC-TASKS-QUESTION-IMAGES-002.2
  - AC-TASKS-QUESTION-IMAGES-002.5
  - AC-TASKS-QUESTION-IMAGES-003.2
  - AC-TASKS-QUESTION-IMAGES-003.4
  - AC-TASKS-QUESTION-IMAGES-004.1
  - AC-TASKS-QUESTION-IMAGES-004.2
  - AC-TASKS-QUESTION-IMAGES-004.3
  - AC-TASKS-QUESTION-IMAGES-004.4
  - AC-TASKS-WIP-LIMIT-PULL-SYSTEM-001.10
system_design:
  - ../../specs/tasks/system-design/question-image-previews.md
  - ../../specs/tasks/system-design/wip-limit-pull-system.md
---

# Task 04: Prove integration and document usage

## Summary

Prove that images preserve clarification ownership and recovery across real
executor boundaries. Document the shipped authoring and operator workflow.

## In scope

- Add real Docker and SSH integration cases using the existing container fixtures.
  Create the image in the executor workspace. Fetch through the backend from a
  browser that cannot read that filesystem or contact executor localhost directly.
- Extend the image E2E suite with page reload, restart reconstruction, terminal
  history, missing files, source removal, and a different selected session.
- Prove timeout/detached answer, rejection, first-writer-wins, superseded-turn,
  choice-only, and image-free behavior. Add only missing regression cases.
- Update operator image-question guidance, the existing public usage guide, and
  tool prompt/schema guidance. Preserve the full hard-barrier wording.
- Record exact validation results and reconcile draft spec statuses only when
  all work orders pass. Do not mark an unavailable container run as passed.

## Out of scope

New product UI, provider capability expansion, broad local audits, new deployment
certification, snapshots, and publishing or GitHub operations.

## Acceptance

1. Actual Docker and SSH images render through authorized source-session transport.
   Remote failure cannot trigger host-path or localhost fallback.
2. Reload, restart, concurrent answers, rejection, stale turns, and late answers
   preserve the existing lifecycle with image-bearing questions.
3. Public docs and agent guidance describe the implemented contract. All prior
   work-order results are recorded before the plan becomes implemented.

## Focused validation cases

Add `e2e/tests/docker/question-image-previews.spec.ts` and
`e2e/tests/ssh/question-image-previews.spec.ts`. Use each directory's existing
executor setup and cleanup. Test a valid image, missing remote file, and revoked
or inaccessible source. Assert no absolute path or remote URL appears in browser data.

Extend `question-image-previews.spec.ts` with two source sessions containing different
bytes at the same relative path. Switch selected sessions and verify the question
still resolves its own source. Reload and restart using existing test fixtures.
Assert metadata survives even if bytes become unavailable. Do not promise unsent
draft persistence across reload. Verify a changed file is read freshly after reload.

Use existing resolver concurrency and retry suites for first-writer-wins evidence.
Add image-bearing fixtures where those suites would otherwise omit the new metadata.
Keep image failures separate from answer failure. Test inspection open during a
concurrent resolution or bundle replacement. Close stale inspection safely.

Run existing image-free clarification, choice-only, submission-failure, mobile,
and rich-output preview suites listed below. They verify this change does not
silently alter related clients or default image disclosure.

## Public documentation

Update `docs/public/agent-communication.md` with a focused operator how-to section.
Include an exact JSON example with two questions about one image, stable question
IDs, and keyed answers. Explain one-based generated option IDs without changing
the answer envelope. Recheck issue #4667 before documenting its current result.

Document PNG/JPEG support, 10 MiB and 16-million-pixel limits, four questions and
four distinct images per bundle, optional repository scope, no external URLs,
and live-file history limitations. Explain unavailable executor behavior and Retry.
Keep parent-question guidance separate.

Add a short operator instruction to `docs/public/use-kandev.md`: inspect the image,
close it, answer, and submit the bundle. Link the detailed authoring guide. Search
README and `docs/screenshots.md` for stale claims. Add screenshots only when existing
capture fixtures produce suitable evidence. Do not add a new public page.

Update `internal/sysprompt` guidance only as needed to advertise image references.
Retain the exact accepted-call question barrier and autopilot capability rules.

## Verification

Run from the repository root. Reuse Task 03's dependency installation. Run E2E
commands sequentially. Managed runs rebuild assets and enforce resource limits.

```bash
(cd apps/backend && go test ./internal/clarification ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/question-image-previews.spec.ts tests/chat/clarification.spec.ts tests/chat/clarification-choice-only.spec.ts tests/chat/clarification-submit-failure.spec.ts tests/chat/rich-output.spec.ts tests/chat/rich-output-evidence.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts tests/chat/mobile-clarification.spec.ts tests/chat/mobile-rich-output-evidence.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/docker/question-image-previews.spec.ts tests/ssh/question-image-previews.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use local `.github/scripts/pr-docs.cjs::validateCoverage` with the complete changed
file set and referenced documents. It must validate every work order's requirements,
acceptance criteria, design, and plan. Do not publish a GitHub status from this check.
Record concrete lint commands for any added Go or E2E files.

If Docker or SSH infrastructure is unavailable, record the exact failed command,
reason, and unverified case. Leave the affected work order incomplete. Do not
replace real-executor evidence with mocks while claiming the same coverage.

## Files likely touched

- New `apps/web/e2e/tests/docker/question-image-previews.spec.ts`
- New `apps/web/e2e/tests/ssh/question-image-previews.spec.ts`
- `apps/web/e2e/tests/chat/question-image-previews.spec.ts` and its mobile counterpart
- Existing clarification retry/concurrency tests only where image fixtures add coverage
- `docs/public/agent-communication.md`, `docs/public/use-kandev.md`
- `apps/backend/internal/sysprompt/sysprompt.go` and focused schema/prompt tests
- This plan and its work orders, paired requirement/design lifecycle fields

## Dependencies

Tasks 01 through 03. Read e2e fixture-state, cleanup, and resource-safety guidance
before runs. Apply docs-maintainer for public updates. No new UI preview is needed
because this task verifies UI-01 through UI-04 without changing their structure.

## Risks

A local file fixture cannot prove remote execution. Restart tests must use durable
messages, not an in-memory request stub. Existing test files may assume no images.

## Parallelism

Sequential. Remote suites share resource budgets, and package results have one owner.

## Inputs

All requirement IDs, the full system design, prior work-order results, existing
clarification tests, and current Docker/SSH fixtures.

## Results

Implementation, public documentation, and feature verification are complete.
The initial local handler/executor limits below were closed by CI on head
`8745e76a84536a5fff3f238d2fcdb4bfa23029b2`. PR #4766 is open and ready for review.
Additional CI fixture follow-up is recorded at the end of this work order.

### Passing evidence

- Final focused frontend run: 97 tests. See Task 03 for its exact command.
- Full clarification and system-prompt packages pass. Full MCP server package and
  focused handler/retry tests pass. An explicit race run exercises the real REST
  and MCP answer entry points for both legacy and image-bearing bundles. A separate
  image-bearing detached-answer test verifies failed delivery, retry, rejection,
  and winner replay. See Task 01 for the commands.
- Desktop image flow: two tests passed, including backend restart, lazy history,
  changed/deleted files, full-size scrolling, keyed answers, and draft/choice/position
  restoration across 767px, 768px, and 1280px widths. Log:
  `/tmp/question-images-checks/kandev-run.e2e.g4MGHDK3.log`.
- Final combined desktop run: all 40 image, clarification, choice-only,
  submission-failure, and rich-output cases passed after the final provider-key
  correction. Log: `/tmp/question-images-checks/kandev-run.e2e.eD0G0INc.log`.
  Light and dark viewer captures, including the 767px boundary, were inspected.
- Final mobile suite: 15 tests passed. Log:
  `/tmp/question-images-checks/kandev-run.e2e.kz1iEI9t.log`.
- Source ownership uses a backend adapter test with two sessions and different
  bytes at the same relative path. Source removal uses the real process/API tests.
  Those are focused integration tests, not a browser-selected-session scenario.
- Public-doc tests and validation pass (48 published pages). Catalog validation
  passes (384 decisions, 1618 specifications). Specification lint passes. Local
  coverage validation against all 70 changed files covers all four work orders
  with zero reference errors. Relative links and `git diff --check` pass.
- Changed Go and frontend lint, formatting, types, and localization pass.

Browser commands used the managed host runner with `KANDEV_SERVER_HOST=127.0.0.1`,
`E2E_PORT_OFFSET=23`, and the host's Chromium shared-library paths. The temporary
Nix library setup is `/tmp/question-images-browser-env.sh`. The final source was
rebuilt by the managed runner; subsequent unchanged-artifact runs used `--no-build`.
Commands from the repository root:

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium tests/chat/question-image-previews.spec.ts tests/chat/clarification.spec.ts tests/chat/clarification-choice-only.spec.ts tests/chat/clarification-submit-failure.spec.ts tests/chat/rich-output.spec.ts tests/chat/rich-output-evidence.spec.ts
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-question-image-previews.spec.ts tests/chat/mobile-clarification.spec.ts tests/chat/mobile-rich-output-evidence.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node /tmp/check-question-image-package.cjs
git diff --check
```

### Initial local validation limits

`go test ./internal/mcp/handlers` reached its default 10-minute package limit.
`go test -timeout 30m ./internal/mcp/handlers` also timed out, during SQLite fixture
setup for `TestHandleMessageTask_DispatchErrorAfterExistingSessionSwitchPreservesQueues`.
No image assertion failed in that run. Log: `/tmp/question-images-handlers-tests.log`.
A further `go test -timeout 10m ./internal/mcp/handlers` with a fresh RAM-backed
`TMPDIR` and `GOTMPDIR=/tmp` still timed out during coordinator fixture setup.
Log: `/tmp/question-images-handlers-tmpfs-tests.log`. The temporary directory was
removed. These attempts do not count as a passing full package run.

This exact container command stopped before executing tests:

```bash
KANDEV_E2E_CONTAINERS=1 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project containers tests/docker/question-image-previews.spec.ts tests/ssh/question-image-previews.spec.ts
```

The managed preflight reported missing `apps/backend/bin/mock-agent-linux-amd64`.
Log: `/tmp/question-images-checks/kandev-run.e2e.5Nbm14z8.log`.
The installed Docker 29.7.2 client's `docker info` also failed because
`/var/run/docker.sock` does not exist. Log: `/tmp/question-images-docker-preflight.log`.
Both Docker and SSH fixtures require that daemon. They were unexecuted locally.
The later CI executor evidence below closes this connectivity gap; agentctl
client/API tests alone do not prove provider connectivity.
Windows and macOS binaries compile but were not run natively; see Task 02.

### Follow-through

The later CI runs below completed the full backend suite and both executor cases.
No further product-scope question is required. Issue #4667 remains open; this
implementation keeps its existing answer envelope and question barrier.


### PR CI executor fixture correction

CI run `40190002697` executed both real executor image cases and reported missing
files. The fixtures used the executor-only `prepare_script` field on a repository
PATCH, so neither wrote the image. They now set the repository `setup_script`,
which runs in the authorized checkout, and restore its captured original value
in `finally`. The image path, rendering assertions, and answer flow are unchanged.
The local host has no Docker daemon; real Docker and SSH execution remains a CI
gate for this correction.


### Additional CI fixture stabilization

CI run `40192890436` passed the original image, executor, layout, and mobile
composer cases. Four other E2E cases passed only after retry and failed the
strict flake gate. Their failure evidence led to these fixture corrections:

- `apps/web/e2e/helpers/mcp-status-clarity.ts` keeps one status route installed
  through all controlled report states. Removing it between states let live
  `applied` responses enter a fixture that expected `deferred_restart` or saved
  changes. Each intercepted read captures its scenario before awaiting the backend.
- `apps/web/e2e/tests/task/plugin-repository-task-create.spec.ts` waits for the
  repository popover to close and restore focus before opening the branch picker.
  The failed trace showed the branch option detach and focus return to the
  repository trigger.
- `apps/web/e2e/tests/task/create-task-branch-selector.spec.ts` uses the same close
  and focus boundary after a pasted repository URL, before opening the executor
  picker. The failed trace waited for options after the selector had closed.

These changes keep the assertions, application behavior, timeouts, and CI retry
policy intact. Changed-file lint passed. Constrained verification uses one worker
and two CPUs, three runs per failed case, with retries disabled.


### Completed feature verification in CI

For head `8745e76a84536a5fff3f238d2fcdb4bfa23029b2`, backend workflow
[40192890300](https://github.com/kdlbs/kandev/actions/runs/40192890300) passed its
complete package shards, including the MCP handler package. E2E workflow
[40192890436](https://github.com/kdlbs/kandev/actions/runs/40192890436) passed both
real Docker and SSH question-image cases, the original layout regressions, and
all desktop/phone image cases. Its complete artifact audit found 4,242 cases
passing on the first attempt, four additional flaky cases listed above, and 40
skips. The flaky cases failed the overall E2E gate and remain part of PR follow-up;
they are not presented as a clean CI run. Windows/macOS native execution remains
outside this Linux validation, as recorded in Task 02.


The constrained follow-up passed all 12 runs with retries disabled: three phone
runs and nine desktop runs across the four affected cases. Each command used
`taskset` to restrict the managed runner to two allowed CPUs and one worker.
Logs: `/tmp/question-images-checks/kandev-run.e2e.8cCskM17.log` (3 phone cases) and
`/tmp/question-images-checks/kandev-run.e2e.fTxRkb3m.log` (9 desktop cases).
The latest full CI and review gates are tracked on PR #4766.


### Quick Chat CI fixture correction

E2E workflow [40195458118](https://github.com/kdlbs/kandev/actions/runs/40195458118)
passed 4,245 cases on the first attempt and skipped 40. One Quick Chat goal
disclosure case passed only after retry, so the E2E gate failed. Its failure
snapshot shows the goal command before the opening prompt, with a rejected
message while the agent was already running. The test now waits for the existing
completed-turn baseline and direct-input state before it sends the goal command.
This changes test setup only. Goal behavior and its UI assertions remain intact.

The original focused case passed three local runs with retries disabled. That
result does not invalidate the recorded CI race. The corrected full goal spec
passed all nine runs under two CPUs and one worker with retries disabled.
Command: `pnpm --dir apps/web e2e:run --host --no-build --project chromium
tests/chat/agent-goal.spec.ts --retries=0 --repeat-each=3`, through the managed
runner with `taskset`. Log: `/tmp/question-images-checks/kandev-run.e2e.ywYEMNB7.log`.
Changed-file ESLint, specification lint, and documentation coverage passed.


### Additional E2E setup boundaries

Workflow [40197079428](https://github.com/kdlbs/kandev/actions/runs/40197079428)
passed 4,240 cases on the first attempt and skipped 40. Five cases passed after
retry. The mobile transcript-controls case failed its single attempt. The corrected
Quick Chat goal case passed. The six additional failures exposed test setup
boundaries outside the image feature:

- Mobile sidebar and workspace tests now wait for the opening drawer animation
  before using controls inside it. CI showed a missing nested drawer and a
  detached workspace option during a tap.
- The mobile file-tree test waits for layout observers after reopening Files,
  matching its initial-open setup. CI sampled a four-pixel gap before measurement.
- The review-scroll test waits for the populated diff and settled dialog before
  its first gesture. A visible header alone does not enable auto-marking while
  the diff is pending.
- The mobile composer-layout test disables automatic scrolling through its UI
  control before setting a programmatic transcript position. Otherwise resize
  following can return the transcript to the end and remove Jump to latest.
- The delayed port-hydration test controls browser time while holding its response.
  The normal WebSocket request deadline is five seconds. The test must exercise
  response ordering without racing that independent timeout during UI actions.
  It restores the clock and releases the response in cleanup.

The original five mobile cases passed one focused local run, and the original
port case passed three. These local results do not invalidate CI failures. The
corrected port case passed three runs without retries under two CPUs and one
worker (`--project chromium tests/session/port-forward-dialog.spec.ts --grep
'merges late tunnel hydration' --retries=0 --repeat-each=3`). Log:
`/tmp/question-images-checks/kandev-run.e2e.WL7Vp6BC.log`. Changed-file lint passed.
These corrections preserve product behavior, assertions, and production timeouts.


All 15 corrected mobile checks passed under two CPUs and one worker, with
`--project mobile-chrome --retries=0 --repeat-each=3` and the five affected
scenarios selected by title. Log:
`/tmp/question-images-checks/kandev-run.e2e.kGjfcgey.log`. Together with the port
case, all 18 constrained checks passed. Documentation catalog, specification
lint, and changed-file coverage checks passed.


### SPA recovery CI boundary

The next CI run exposed a Settings chunk-recovery fixture race in
`apps/web/e2e/tests/layout/spa-resilience.spec.ts`. The failure snapshot still
showed `Loading Settings…` during the replacement navigation. The test now
waits for the successful replacement chunk response to finish before asserting
the rendered Updates title. The exact two-document and two-chunk counts remain
unchanged. This is a test-only readiness correction, with no timeout increase
or change to SPA recovery behavior.

The original case passed three focused local runs under two CPUs with retries
disabled. The corrected full three-case spec passed all nine runs under the same
constraint (`--project chromium tests/layout/spa-resilience.spec.ts --retries=0
--repeat-each=3`). Log: `/tmp/question-images-checks/kandev-run.e2e.zQr5lN6X.log`.
Changed-file lint passed.


Workflow [40199411519](https://github.com/kdlbs/kandev/actions/runs/40199411519)
finished with 4,244 first-attempt passes, two flaky cases, and 40 skips. The second
flaky case was mobile Changes-panel setup: a late timeline hydration restored
the top position after a one-shot scroll, leaving the section heading mounted.
The fixture now repeats its idempotent scroll until the heading is unmounted,
then retains the existing recovery, filename-width, action-containment, and
file-opening assertions. The complete artifact audit found no other failures.


The original mobile Changes case passed three constrained local runs. After the
setup correction, three more runs passed with retries disabled under two CPUs
and one worker (`--project mobile-chrome tests/task/mobile-changes-panel.spec.ts
--grep 'deep tree rows keep' --retries=0 --repeat-each=3`). Log:
`/tmp/question-images-checks/kandev-run.e2e.0spo4MBM.log`. The nine SPA checks and
three Changes-panel checks all passed. Changed-file lint, specification lint,
and the documentation catalog passed.

### Storage cleanup and clock setup

The next CI run exposed two fixture defects. The storage cache-reuse case saved
settings twice through the UI, then its cleanup assertion matched two valid
success notifications. It now captures and restores the complete settings through
the existing API helper. The primary UI save and cache-reuse assertions remain.

The delayed port-hydration case used a pause target only 100 milliseconds after
the browser's current time. CI delivered the pause command after that target.
The clock now installs at an explicit time and pauses 60 seconds beyond it,
before the held request begins. Product deadlines and ordering assertions remain.

Each corrected case passed three runs under two CPUs and one worker with retries
disabled. Commands used `--project chromium --retries=0 --repeat-each=3`, selecting
the exact case in `tests/system/storage-maintenance.spec.ts` and
`tests/session/port-forward-dialog.spec.ts`. Logs:
`/tmp/question-images-checks/kandev-run.e2e.k2iLii0M.log` and
`/tmp/question-images-checks/kandev-run.e2e.LvjkqAIm.log`. Changed-file ESLint passed.


### Preserve preload rejection and diagnostic collection time

Workflow [40204675811](https://github.com/kdlbs/kandev/actions/runs/40204675811)
finished with 4,244 first-attempt passes, two flaky cases, and 40 skips. The full
118-report audit found only the Settings recovery and diagnostic ZIP failures.
The previous storage cleanup and port clock corrections passed.

The Settings failure exposed a production error in the first reload attempt.
The Vite helper returns undefined when its preload event is canceled. React can
consume that result before navigation completes and report an unrelated module
TypeError. The listener now preserves the original rejection while retaining the
verified storage marker and one-reload limit. Four updated unit cases failed
before the correction. All nine preload recovery cases passed after it. The
[deployment recovery ADR](../../decisions/2026-07-27-spa-failure-containment-and-deployment-recovery.md)
and its plan now record this amendment to the original completed work order.

The diagnostic ZIP failure spent 27.5 seconds in setup, then had only 13.9 seconds
to wait for a download. Backend collection intentionally takes 15 seconds. The
test's 45-second override ended the test before collection could finish. Removing
that override restores the standard 60-second suite budget. Production collection
time and archive assertions remain unchanged. Both original cases passed three
local runs, which does not invalidate the CI timing evidence.

The corrected full SPA and Logs specs passed all 18 cases under two CPUs and
one worker, with retries disabled: `--project chromium
tests/layout/spa-resilience.spec.ts tests/system/logs-page.spec.ts --retries=0
--repeat-each=3`. Log: `/tmp/question-images-checks/kandev-run.e2e.Upf8Xaqm.log`.
The fresh E2E browser build, changed-file lint, documentation catalog,
specification lint, and work-order coverage passed. No user-facing copy changed.


### Runner selection, picker settlement, and answer history

Workflow [40206656046](https://github.com/kdlbs/kandev/actions/runs/40206656046)
finished with 4,243 first-attempt passes, three flaky cases, and 40 skips. The
118-report audit found no other failures. The preload and diagnostic corrections
passed. Three test assumptions needed correction:

- Runner switching selected the destination with a name-substring regex. CI had
  three matching local profiles. Both switching cases now select the existing
  option value by profile ID and retain their stored-runner assertions.
- Coordinator setup opened the executor picker before the agent picker completed
  focus restoration. The executor option detached, then the click waited until
  the test deadline. Setup now waits for the prior option to unmount and focus
  to return to its trigger before opening the next picker.
- Mobile autopilot treated a second turn row as proof that its answer message was
  already projected. It now polls the history for both the answered question and
  exactly one parent response before checking the child UI.

Each original failing case passed three constrained local runs, which does not
invalidate the CI evidence. Logs: `/tmp/question-images-checks/kandev-run.e2e.FzqaodAE.log`,
`/tmp/question-images-checks/kandev-run.e2e.fT1p1ziq.log`, and
`/tmp/question-images-checks/kandev-run.e2e.gpJvhND1.log`. These changes affect test
synchronization and selection only. Product behavior and answer semantics remain.

All 21 corrected checks passed under two CPUs and one worker with no retries.
The full coordinator and runner-switch specs passed 18 desktop checks
(`--project chromium tests/task/runner-switch.spec.ts
tests/coordinator/settings.spec.ts --retries=0 --repeat-each=3`). The mobile
autopilot spec passed three checks (`--project mobile-chrome
tests/task/mobile-autopilot-mode.spec.ts --retries=0 --repeat-each=3`). Logs:
`/tmp/question-images-checks/kandev-run.e2e.EnFcwgLl.log` and
`/tmp/question-images-checks/kandev-run.e2e.wSPFdWp3.log`. Changed-file ESLint,
specification lint, documentation catalog, and work-order coverage passed.

### Mobile guidance and live-row geometry

Workflow [40208567506](https://github.com/kdlbs/kandev/actions/runs/40208567506)
finished with 4,244 first-attempt passes, two flaky cases, and 40 skips. All 118
blob reports parsed. The only failures were the mobile WIP guidance tooltip and
the saved fallback-summary geometry check. Prior corrections passed.

The tooltip could open on focus and close on the same tap. WIP guidance now opts
into the existing `HelpTip` drawer, as used by the completion-setting help. Phone
and coarse-pointer users get the same localized guidance in a temporary bottom
drawer with a 44-pixel minimum trigger. This includes narrow fine-pointer windows
and coarse-pointer tablets. Desktop hover and focus retain the tooltip. The
existing drawer owns scrolling, safe-area spacing, dismissal, and focus return.
Reading guidance does not change the selected feeder or the workflow draft. Its
description uses a div so the existing block content remains valid HTML. The WIP
requirement, design, and public workflow guide record this behavior.

The saved fallback-summary test measured a row during replacement and received a
zero-width rectangle after its visibility assertion. It now retries the complete
geometry assertions against the live row. Positive size, wrapping, and containment
assertions remain. This is test synchronization, with no product-layout change.

The new narrow fine-pointer unit case failed before the helper correction. All
15 helper and step-action unit cases passed afterward. The original mobile
cases each passed three constrained runs; that does not invalidate the CI
evidence. The revised drawer test failed against old assets at the 44-pixel
assertion, then passed against rebuilt assets. Browser checks cover both feeder
states, readable persistent guidance, Escape dismissal, focus return, and no
horizontal overflow. A new PR capture documents the mobile drawer; existing
question-image captures remain applicable because their UI did not change.

The final rebuilt assets passed all six focused mobile checks with retries off
under two CPUs and one worker: `--project mobile-chrome
tests/workflow/mobile-workflow-settings.spec.ts
tests/settings/mobile-agent-profile-layout.spec.ts --grep 'optional feeder
guidance|wraps the saved fallback' --retries=0 --repeat-each=3`. Log:
`/tmp/question-images-checks/kandev-run.e2e.BoMTuxXi.log`. Changed-file ESLint,
specification lint, documentation catalog, and work-order coverage passed.

### Recovery disclosure and independent browser fixtures

Workflow [40210898134](https://github.com/kdlbs/kandev/actions/runs/40210898134)
finished with 4,243 first-attempt passes, two flaky cases, one failed case, and
40 skips. The complete 118-report audit found no other failures. The mobile
guidance and fallback-layout corrections passed. These failures require
test-only corrections:

- Session recovery renders the same disclosure in its action message and startup
  card. After reload, the broad locator matched both. The assertion now scopes to
  the startup recovery card, which the scenario already verifies and captures.
- The multi-file read case could not find its newly committed file in the shared
  seed workspace. Its original CI snapshot showed only the baseline file. The
  case now uses an owned repository and publishes both fixture files to its local
  origin. It deletes its task, repository, profile, and directory afterward.
  Both individual links and opening the second file remain required assertions.
- The Sentry case opened Workflow immediately after dismissing its project
  selector. The original attempt reached that tap, then found no workflow option.
  It now waits for each prior listbox to unmount and focus to return to its trigger
  before opening the next selector. Held-write, draft, caret, and persistence
  assertions remain unchanged.

The original desktop cases passed six constrained local checks. The original
mobile Sentry create case passed three checks. Logs:
`/tmp/question-images-checks/kandev-run.e2e.urgrOmbY.log` and
`/tmp/question-images-checks/kandev-run.e2e.WZDOwONU.log`. These local passes do not
invalidate the original-attempt CI evidence. No production behavior, public
contract, copy, or screenshot changes are required for these fixture corrections.

The corrected desktop recovery and file-link cases passed all six runs with
retries disabled under two CPUs and one worker (`--project chromium
tests/task/chat-read-multi-file.spec.ts
tests/session/session-resume-settings-recovery.spec.ts --grep 'splits a
comma|one-attempt settings omission' --retries=0 --repeat-each=3`). Log:
`/tmp/question-images-checks/kandev-run.e2e.1T1Ik6Tb.log`.

The complete mobile Sentry ownership spec passed six checks with retries disabled
under the same limits (`--project mobile-chrome
tests/integrations/mobile-sentry-watch-dialog-ownership.spec.ts --retries=0
--repeat-each=3`). Log: `/tmp/question-images-checks/kandev-run.e2e.k8yN4UhW.log`.
Changed-file ESLint, specification lint, documentation catalog, and work-order
coverage passed. All 12 corrected repeated browser checks passed.

### Confirm preference persistence before reload

The next CI run exposed a task-list preference test race. Its original attempt
completed three optimistic UI assertions in six milliseconds, then reloaded
without observing the asynchronous settings write. The test now waits until
`tasks_list_show_details` is true in the backend before reloading, and retains
all after-reload row and persistence assertions.

The original case passed three constrained local runs. The corrected full
preference spec passed six checks with no retries under two CPUs and one worker:
`--project chromium tests/task/task-listing-view-preferences.spec.ts --retries=0
--repeat-each=3`. Logs: `/tmp/question-images-checks/kandev-run.e2e.9OiWV2RB.log`
and `/tmp/question-images-checks/kandev-run.e2e.MR9IhWWi.log`. This is test
synchronization; the asynchronous product persistence contract is unchanged.

The same run exposed a storage navigation setup race. The original attempt read
accordion content immediately after opening it, then issued its cleanup-review
click while the opening animation could still move the control. The case now
uses the existing finite-animation helper on that resource before capture and
click. It still requires the cleanup accordion to expand, focus to move to its
trigger, and no storage mutation request. The original case passed three local
runs (`/tmp/question-images-checks/kandev-run.e2e.K5Xih33o.log`); the CI attempt's
click completed in 49 milliseconds, shortly after its text assertions. No fixed
sleep, retried click, or product change is introduced.

Workflow [40213317681](https://github.com/kdlbs/kandev/actions/runs/40213317681)
finished with 4,244 first-attempt passes, these two flaky cases, and 40 skips.
The 118-report audit found no other failures. The corrected storage case passed
three checks with retries off under two CPUs and one worker (`--project chromium
tests/system/storage-temporary-folders.spec.ts --grep 'bounded entry ownership'
--retries=0 --repeat-each=3`). Log:
`/tmp/question-images-checks/kandev-run.e2e.91hi7WpL.log`. All nine corrected
checks passed, as did changed-file lint, specification lint, documentation catalog,
and work-order coverage. Existing UI captures remain applicable.

### Bounded SQLite contention in the retention fixture

Workflow [40214715871](https://github.com/kdlbs/kandev/actions/runs/40214715871)
exposed a direct fixture write failing with `database is locked` in the mobile
retention recovery case. The test inserts an independent recovery block while
its disposable backend remains active. That connection had no busy timeout.
The fixture now sets `PRAGMA busy_timeout=5000` before its single insert, matching
the adjacent tool-payload retention fixture. It still requires exactly one row
and retains all history, recovery, and prompt-authority assertions. This changes
only the test connection; production database handling is unchanged.

The original mobile case passed three constrained local checks with retries off:
`--project mobile-chrome tests/session/mobile-delivery-retention.spec.ts
--retries=0 --repeat-each=3`. Log:
`/tmp/question-images-checks/kandev-run.e2e.Tvl3nDpE.log`. Passing without
contention does not invalidate the CI lock evidence.

The completed 118-report audit found 4,245 first-attempt passes, this one flaky
case, and 40 skips. No other failure was reported. The corrected mobile retention
case passed all three repeated checks under the same limits and with retries off.
Log: `/tmp/question-images-checks/kandev-run.e2e.3BO7QYKA.log`. Changed-file lint,
specification lint, documentation catalog, and work-order coverage passed. No UI
capture or public documentation change is needed for the fixture-only correction.

The post-commit desktop check reproduced `database is locked` in the same helper's
historical-gap write, before recovery began. Log:
`/tmp/question-images-checks/kandev-run.e2e.oPRumBzN.log`. Its separate write
connection now uses the same five-second busy timeout. Both fixture connections
write while the disposable backend is active; read-only probes are unchanged.
The correction remains test-only and does not alter retention deadlines.

With both write connections configured, all three desktop retention checks passed
under two CPUs and one worker with retries off (`--project chromium
tests/session/delivery-retention.spec.ts --grep 'preserves saved history'
--retries=0 --repeat-each=3`). Log:
`/tmp/question-images-checks/kandev-run.e2e.PlXJpd2t.log`. Changed-file lint,
specification lint, and documentation catalog passed.

### Restore visible motion and await resumed mobile chat

Workflow [40216849404](https://github.com/kdlbs/kandev/actions/runs/40216849404)
reported two intermittent E2E failures. The complete 118-report audit found
4,244 first-attempt passes, two flaky cases, 40 skips, and no parse errors.

The persistent composer animation case moved the glow outside its clipped panel
with a transform, then used `scrollIntoView` without removing that transform.
The original failure retained an active turn, so turn completion did not cause
this failure. The test now removes the transform, asserts viewport intersection,
and then requires the compositor animation to resume. Hidden and offscreen pause
checks and terminal removal remain intact. This avoids incidental panel scrolling
as the mechanism for restoring a deliberately displaced element.

The mobile workflow override case checked backend idleness before reloading both
clients. Its original failure snapshot showed the correct initial session with
`Starting Mock` and a queued composer after reload. The test now waits for an
idle, editable phone composer after reload before inspecting the next-step button.
The task, session, selected profile, workflow preview, and touch assertions remain
unchanged. This uses the existing session readiness helper.

Both original cases passed three constrained local checks with retries off:
`--project chromium tests/chat/persistent-animation-motion.spec.ts --retries=0
--repeat-each=3` and `--project mobile-chrome
tests/task/mobile-create-task-workflow-agent-overrides.spec.ts --retries=0
--repeat-each=3`. Logs:
`/tmp/question-images-checks/kandev-run.e2e.1vFCWCjH.log` and
`/tmp/question-images-checks/kandev-run.e2e.FK3ePJ9o.log`. The original CI artifacts
remain the failure evidence. These corrections change tests only, so no public
behavior, durable contract, or UI capture changes are required.

Both corrected cases passed all three repeated checks with retries disabled,
two CPUs, and one worker, using the same commands above. Logs:
`/tmp/question-images-checks/kandev-run.e2e.HVzZEk3U.log` (animation) and
`/tmp/question-images-checks/kandev-run.e2e.ohgj0b41.log` (mobile workflow).
Changed-file lint, specification lint, documentation catalog, and work-order
coverage also passed. The terminal PR snapshot reported only these two leaf
failures and their E2E aggregate, with no unresolved review threads.

### Await task selection after workspace preparation

Workflow [40218658108](https://github.com/kdlbs/kandev/actions/runs/40218658108)
passed the preceding motion and mobile corrections. Its only failing case was
`workflow-automation.spec.ts`'s non-auto-start task-open check. The original
snapshot showed an empty chat, no selected workspace panel, and a connecting
terminal. The backend already had one `CREATED` session. The test therefore
asserted the start button before the prepared session became active in the UI.

`prepareAndSwitchTask` awaits preparation before selecting the returned session.
The test now uses `waitForActiveTaskSession` with the exact task and session IDs
before asserting the button. It retains the one-session count, `CREATED` state,
start-button visibility, and unchanged workflow-step checks. It adds no reload,
fixed delay, or retry. No product behavior or public documentation changes.

The original case passed three local checks with retries disabled under two CPUs
and one worker (`--project chromium tests/workflow/workflow-automation.spec.ts
--grep 'non-auto-start step blocks' --retries=0 --repeat-each=3`). Log:
`/tmp/question-images-checks/kandev-run.e2e.BAydP0IH.log`. The complete 118-report
CI audit found 4,245 first-attempt passes, this one flaky case, 40 skips, and no
parse errors. The current PR snapshot had no unresolved review threads or hidden
threads, and the grouped-comment refresh found no new suggestions.

The corrected case passed all three repeated checks under the same constraints:
`/tmp/question-images-checks/kandev-run.e2e.HrpzRdcC.log`. Changed-file lint,
specification lint, documentation catalog, and work-order coverage passed.
