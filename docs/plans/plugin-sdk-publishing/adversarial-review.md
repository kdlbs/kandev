# SDK compatibility adversarial review

Reviewed on 2026-10-24 in a user-requested secondary session. The starting PR
head was `59c9c5603017fd2922168c8d4915a656c79b8627`. These findings concern the
implementation; the earlier [design review](design-review.md) remains a
separate historical record.

## Findings and corrections

1. **P1: the release adapter could not build a publishable candidate.**
   `inspectArchive` computed the packed record digest, but `packCandidate`
   omitted it from its return value. `ReleaseIO.build` therefore rejected every
   fresh build and missing-archive rebuild as a signed-source mismatch. The
   packer now returns that inspected digest. A regression exercises real Git
   archive extraction, packing, the release adapter, and isolated Go/npm
   consumers. It also rejects a conflicting source digest. No production test
   seam or alternate publication path was added.
2. **P2: the checker accepted host minima that the runtime rejects.**
   Above-floor prereleases such as `0.101.0-rc.1` and build metadata such as
   `0.100.0+build.1` passed the author check. The runtime requires a Stable
   minimum. The checker now reuses its Stable-version validator; tests retain
   the accepted lowercase `v` prefix and reject both suffix forms. Runtime
   manifest validation and installed-plugin behavior are unchanged.
3. **P2: the checker rejected valid hoisted npm workspace installs.**
   It required the SDK directly below the plugin directory before consulting
   the workspace lockfile. It now selects the closest installed npm SDK up to
   the nearest supported lockfile root and verifies that exact lock entry and
   consumer importer. An explicit path cannot bypass a closer package. A real
   npm workspace install and CLI check cover default and explicit hoisted
   paths, shadow packages, local symlinks, wrong lock versions, and a nearer
   lockfile boundary. pnpm store handling and the strict lockfile policy remain
   unchanged.

Each bug was reproduced by a failing regression before its production fix.
The npm workspace regression runs in the required SDK packaging CI job, with
the pinned Node/npm tools. The release-adapter regression runs in its existing
release test suite.

## Compatibility assessment

The review inspected Go extraction, protobuf and gRPC identities, host import
migration, the legacy subprocess test, frontend exports, dependency evidence,
release trust/retention/recovery, workflow permissions, and build selection.
It found no additional verified installed-binary or frontend-bundle regression.

Existing compiled plugins retain the same handshake and wire contract. Source
builds against a new checkout still need the documented Go import migration.
The legacy fixture proves its tested handshake and Host state roundtrip, not
every RPC or every community plugin. The author inventory is not a certification.

No SDK was published, no release was dispatched, and no external plugin or
template repository was modified. Registry ownership and trusted-publisher
setup remain release prerequisites. The shared SDK version and SDK-wide host
floor remain the accepted design.

## Verification

The focused red/green tests passed. SDK race tests and vet passed, as did
the SDK CI contracts, public documentation validators, documentation catalog,
and full specification lint. The final validation and follow-up review results
are recorded in the [plan](plan.md#adversarial-review-cleanup).
CI at the starting head is historical evidence; the remediation commit needs
its own current-head checks before delivery is reported complete.

The same secondary reviewer rechecked the fixes and reported no new findings
or blockers. They independently passed the focused Go compatibility/CLI tests,
the real npm workspace regression, and whitespace checks. They inspected the
release-adapter test; the primary session ran its full consumer verification.

## Browser validation follow-up

SDK, backend, and frontend checks passed at `5e6310d4`. E2E run `40129872943`
still failed after one failed-job rerun. Its retained reports contained 4237
first-attempt passes, two retry-pass failures, and 40 skipped tests. These
results did not satisfy the required E2E check.

The browser investigation found three test defects:

1. The multi-file read fixture committed files on the shared checkout's current
   branch. A preceding PR-watcher test left that checkout on `feature/always`,
   while the new task used `main`. Starting the focused test on that branch
   reproduced the same missing-file assertion. The fixture now selects `main`
   and publishes the seed commit through the existing helper. The same branch
   stimulus then passed with retries disabled.
2. The workflow session test accepted a completed synthetic startup turn as
   evidence that its new agent prompt finished. The retained trace and backend
   log showed startup readiness followed by the actual prompt entering
   `RUNNING`. The completion check now excludes `lifecycle_only` turns and
   retains its session identity, state, and real-turn completion assertions.
3. The ordered local replay exposed a PR-watcher test that submitted a user
   message as soon as the task card reached Done. That workflow move precedes
   the session's ready state. The test now uses the shared session-state wait
   before submitting the message once.

The secondary reviewer confirmed both readiness findings from source and
retained evidence. These corrections change test setup and synchronization;
they do not change plugin or runtime behavior. Current-head CI must pass before
validation is complete.

## Container validation follow-up

At `0e029887a1737118597d59a833bf6c62efd583b6`, backend and frontend CI passed.
E2E run `40133977745` failed because two container tests passed only on retry.
The complete report audit found 4237 first-attempt passes, two retry-pass
failures, 40 skips, and no parse errors.

- SSH host-key rotation probed immediately after TCP readiness, before sshd
  necessarily finished reloading its keys. The test now waits for a successful
  probe with a nonempty, changed fingerprint. It rotates once and preserves
  stale-key refusal and explicit re-trust checks. A local simulation of the
  actual helper covered temporary probe failure, the old key, an empty key,
  eventual success, and refusal to accept an unchanged key.
- The Kubernetes foreign-UID test passed task creation and the different-UID
  assertion, then failed during `archiveTask` after 30,036 ms. The retained trace
  covers the successful retry; the first-attempt timing comes from the blob
  report. The exact blocking cleanup step is unknown. Source independently
  shows a two-minute archive work budget behind a 30-second server write timeout.
  The archive handler now allows response writes until its captured work
  deadline plus five seconds, without extending work or changing cleanup,
  identity, ownership, cancellation, or mutation retry rules.

A real HTTP-server regression reproduced EOF before the correction and returned
successful JSON afterward. It covers both the default work budget and an earlier
caller deadline. Existing archive authorization, cancellation, and outcome tests
also passed. This repairs archive response delivery, not plugin compatibility.
The secondary reviewer found no blocking issue in either correction.

Local Docker is unavailable, so the container tests still require current-head
CI. These corrections do not change public commands, API shapes, or documented
plugin migration steps. Durable archive deadline requirements and design remain
aligned with the implementation.

The broad local handler race run reached Go's ten-minute suite timeout while
starting `TestTaskDTOBuilderStampsQueuedPromptCount` in SQLite schema setup.
It reported no assertion failure or data race before that timeout. This is
incomplete validation, not a passing full suite. The archive-context package
passed with the race detector. Focused handler race validation and fresh CI are
recorded separately in the plan.

## Browser fixture validation after container fixes

At `33242d13906ddc6c8cb146470ac1d0e129dcf14c`, backend and frontend CI and
all six container shards passed. E2E run `40190052557` remained failed because
one browser test passed only on retry. The storage cleanup navigation assertion
passed five focused local runs and the single permitted failed-job rerun. No
production or test change was justified for that assertion.

The rerun exposed a different mobile MCP test failure: the saved-report case
expected `deferred_restart` but received the real backend state, `applied`. The
failed-attempt trace showed that the intended fixture response was not served
after the route was removed and installed again. It did not show a selection
write or establish a production state-transition defect. The exact interception
race remains unproven. An unchanged local replay reproduced the same assertion
at 320px with retries disabled: one failure and eight passes across three
viewport widths.

The shared helper now retains one route for all saved-report cases, captures the
selected state before fetching the backend response, and marks that response
with a test-only header. The response wait matches that header before checking
the UI. This avoids reading a response body that reload can discard. All state, copy, keyboard, scrolling,
and touch-target assertions remain. This correction changes only test setup and
synchronization. It does not change plugin support or production MCP behavior.

The failing helper existed only in the current base branch. A conflict-free
merge of `main` at `cc687bdce96bc22fb8a9ca87abe571b8309d3125` made that source
available in this PR. The corrected helper passed nine mobile tests and two
desktop/tablet tests with retries disabled. Formatting, lint, sleep-ratchet,
workflow contracts, and documentation checks passed. The secondary reviewer
found no blocking issue. Fresh CI for the resulting head remains required.

## Static-check job completion budget

At `858ac559a1e74e6c63fe89ab73e09cae5ab458ea`, E2E run `40196132124`
passed all 4239 executed tests on their first attempts, with 40 skips and no
retry-pass failures. Backend run `40196131960` completed every static check,
including SDK artifacts/consumers, SDK tests, and lint with zero issues. Job
`121177329927` then reached its newly integrated 20-minute job limit while
compressing the Go cache. Its checks finished after about 18 minutes; cache
cleanup was cancelled after another two minutes. This was a job-budget failure,
not a failing test or lint finding.

The static-check job now has a finite 30-minute total budget to include cold
builds, SDK consumers, the existing 15-minute lint allowance, and post-job cache
cleanup. Individual validation commands and their assertions are unchanged.
Fresh CI on the delivered correction remains required.

## Browser synchronization follow-up

At `45998286df8e87e181db1e80859410455180ff15`, backend CI completed with the
new total budget. Frontend and container checks passed. E2E run `40198286326`
failed three browser cases that passed only on retry. Review threads remained
clear. Investigation identified these test boundaries:

- The deep-tree mobile test set `scrollTop` once and assumed its section heading
  left the virtualized range. The same assertion failed on the third unchanged
  local run. Its fixture now continues the deliberate bottom scroll across
  rendered frames until that precondition holds. The heading assertion, default
  timeout, and subsequent containment, action, and diff checks remain.
- Quick Chat's reset test interacted before the opening turn completed. CI
  missed the reset dialog; a preserved local failure opened the dialog, then
  lost it before Back. The secondary reviewer confirmed a busy-state transition
  about 130 ms after opening: the composer changed from direct to queued input,
  and the toolbar removed its reset control. The test now awaits the existing
  settled-baseline helper. That helper excludes synthetic `lifecycle_only`
  completions so startup readiness cannot stand in for a completed opening turn.
- The desktop workflow test used a page-wide proceed-button locator, which
  matched two retained panels in CI. Both planned and actual button queries now
  use the existing `SessionPage.activeChat()` scope. The model-preview and
  workflow-route assertions remain. An unchanged local run reached its third
  iteration before SIGTERM; that run is incomplete, not a passing full replay.

These changes affect test setup, readiness, and locator scope only. Production
UI, runtime behavior, and plugin compatibility contracts are unchanged. Targeted
validation and fresh CI remain required before closing this review cleanup.

The existing session-helper unit case now includes a completed lifecycle-only
startup record before an unfinished conversational turn. It failed with the
old predicate because readiness returned at the startup record. All four helper
tests pass with the corrected predicate. Three deep-tree runs, three Quick Chat
reset runs, and two desktop workflow runs passed with retries disabled.

The two existing Quick Chat activity tests also passed with retries disabled,
covering desktop and tablet callers of the strengthened helper. The final
focused browser total is ten passes. Changed-file ESLint, Prettier, the E2E
sleep ratchet, catalog validation (404 decisions and 1734 specifications), and
specification lint passed. Fresh CI on the delivered commit remains required.


## Workspace file and path review corrections

Review of the ready-for-review merge found false failures for lockfiles above
1 MiB and checkout-directory symlinks. Both npm and pnpm regressions failed
before the fix. Separate lockfile limits and canonical checkout-root comparisons
now preserve published dependency checks. SDK race tests, vet, and the real npm
workspace integration passed. See the [plan's correction record](plan.md#workspace-file-and-path-review-corrections)
for boundary coverage and the pending delivery checks.


## Final browser fixture corrections

CI run `40232279840` exposed a floating-point geometry assertion and a Stats
fixture that recovered before the intended manual action. The corrections are
test-only. A controlled foreground refresh proves the Stats competing-read
boundary, but does not identify the exact refresh trigger in the original CI
failure. Secondary review caught a remaining interval before click dispatch;
a second red/green probe verified the actual-click latch.

The workflow-change dialog failure remains unexplained: only the successful
retry has a trace, and three unchanged local runs passed. No speculative fix
was applied. See the [plan's validation record](plan.md#final-browser-fixture-corrections)
for delivery checks and remaining CI requirements.


## Current-main integration and CI fixture isolation

The current base introduces Discord release announcements. Both automatic
delivery and the recovery notice now require public SDK verification and Stable
completion; workflow tests reproduced the missing integration gate before the
correction. The public release guide preserves both publication and announcement
instructions, and the owning Discord requirement/design agree with the gate.

The PR-watcher layout fixture shared one checkout across three PR branches.
A real environment-path regression failed before isolation. Worktree execution
then proved that its PR refs were missing from the offline origin. The corrected
fixture uses an owned repository with published branch/provider refs and verifies
three distinct workspaces while retaining all layout checks. Cleanup attempts
all independent phases and preserves the original test error.

The Go-cache and rename CI failures did not reproduce in three unchanged local
runs each. A slower Go-cache opening animation also passed. The retained traces
belong to successful retries, so they cannot establish the failed attempts'
causes. Their assertions remain unchanged and require fresh CI. See the
[plan's cleanup record](plan.md#discord-integration-and-current-ci-cleanup).
