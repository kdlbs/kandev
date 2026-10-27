# Validation evidence

Historical investigation baseline: `6677802efa4fac6c2ecc909cb433477c90f7fb2d`,
2026-10-25. The first three sections record checks before workflow remediation.
The later implementation and adversarial-review sections record the updated results.

## Existing workflow and helper tests

Commands ran directly from the repository root. No workflow was dispatched.
No credentials were printed or supplied to test fixtures.

| Command | Final result |
| --- | --- |
| `python3 .github/scripts/release-workflow-contract_test.py` | 65 passed after adding installed GnuPG to PATH. Initial run had two missing-gpg errors. |
| `python3 .github/scripts/claude-code-review-workflow-contract_test.py` | 10 passed. |
| `python3 .github/scripts/opencode-code-review-workflow-contract_test.py` | 3 passed. |
| `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` | 30 passed, including reconciliation contracts. |
| `python3 .github/scripts/preview-env-workflow-contract_test.py` | 7 passed. |
| `python3 .github/scripts/revoke-ready-to-merge-workflow-contract_test.py` | 10 passed. |
| `python3 .github/scripts/backend-tests-workflow-contract_test.py` | 25 passed. |
| `python3 .github/scripts/e2e-tests-workflow-contract_test.py` | 26 passed, 1 failed because the fixture invokes missing `/bin/sleep`. |
| `python3 .github/scripts/announce-discord-release-workflow-contract_test.py` | 4 passed. |
| `python3 .github/scripts/notify-release-contributors-workflow-contract_test.py` | 4 passed. |
| `python3 .github/scripts/lint-action-pinning_test.py` | 9 passed. |
| `node --test scripts/notify-docs-workflow.test.mjs` | 2 passed. |
| `bash scripts/opencode-code-review.test.sh` | Passed all shell assertions. |
| `python3 .github/scripts/claude-read-pr-file_test.py` | 6 passed. |
| `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` | 7 passed. |
| `python3 scripts/opencode-code-review.test.py` | 20 passed. |

The E2E contract failure is
`test_image_digest_resolver_times_out_stalled_registry_lookup`. Its fake Docker
script calls `/bin/sleep`. On this Nix host, the command fails immediately instead
of reaching the expected 0.5-second timeout. No test or global filesystem change
was made to hide this result. It remains a verification limitation for implementation.

The release recheck used:

```bash
PATH=/nix/store/xf4hkkqv4xlcikiwisd8fdry6v9ncgzs-gnupg-2.4.9/bin:$PATH python3 .github/scripts/release-workflow-contract_test.py
```

## Historical baseline scan

Zizmor was absent from PATH. The official v1.25.2 Linux binary was downloaded to
`/tmp/kandev-security-tools`. Both the text command and `--format json` scan ran
against `.github/workflows`. The text scan exited 14 with the 40 unsuppressed
findings recorded in [findings.md](findings.md). A repeat baseline scan reproduced
the same counts and exit code after the investigation's document-only edits.
The post-remediation result is 10 unsuppressed findings, as recorded below.

The scanner binary and raw reports are temporary session artifacts. The complete
source/rule/severity/disposition inventory is durable in the finding ledger.

## Document checks

`python3 scripts/list-docs.py validate` passed: 404 decisions and 1740
specifications. `python3 scripts/lint-spec-files.py --all` passed.

The local exported `validateCoverage` function in `.github/scripts/pr-docs.cjs`
classifies this document-only change as exempt. CI paths are also exempt, so
that result alone does not validate work-order links. A second in-memory call
added a synthetic runtime path to force reference validation. It returned
`covered` for all three work orders with no errors. No synthetic source file was
created and no GitHub status was published.

All package document links resolve. All 40 finding IDs appear exactly once.
`git diff --check` passed. Package status shows the new untracked work orders,
as expected. All package files remain unstaged and uncommitted.

## Implementation verification, 2026-10-26

The tests and non-scanner checks below passed. Zizmor returned the expected exit
code 14 for the reviewed residual findings. The test-only timeout fixture now
resolves the installed system sleep before adding its fast retry stub to PATH.
This fixes the historical Nix failure while retaining the 0.5-second timeout assertion.


| Command | Result |
| --- | --- |
| `python3 .github/scripts/automatic-review-inputs_test.py` | 6 passed |
| `python3 .github/scripts/claude-code-review-workflow-contract_test.py` | 11 passed |
| `python3 .github/scripts/claude-read-pr-file_test.py` | 9 passed |
| `python3 .github/scripts/opencode-code-review-workflow-contract_test.py` | 4 passed |
| `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py` | 30 passed |
| `python3 .github/scripts/release-workflow-contract_test.py` | 69 passed |
| `python3 .github/scripts/image-publication-workflow-contract_test.py` | 3 passed |
| `python3 .github/scripts/e2e-tests-workflow-contract_test.py` | 27 passed |
| `python3 .github/scripts/preview-env-workflow-contract_test.py` | 7 passed |
| `python3 .github/scripts/revoke-ready-to-merge-workflow-contract_test.py` | 10 passed |
| `python3 .github/scripts/backend-tests-workflow-contract_test.py` | 25 passed |
| `python3 .github/scripts/announce-discord-release-workflow-contract_test.py` | 4 passed |
| `python3 .github/scripts/notify-release-contributors-workflow-contract_test.py` | 4 passed |
| `python3 .github/scripts/lint-action-pinning_test.py` | 9 passed |
| `python3 scripts/opencode-code-review.test.py` | 20 passed |
| `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` | 8 passed |
| `bash scripts/opencode-code-review.test.sh` | Passed |
| `node --test scripts/notify-docs-workflow.test.mjs` | 3 passed |
| `python3 .github/scripts/lint-action-pinning.py` | Passed |
| `python3 scripts/list-docs.py validate` | Passed |
| `python3 scripts/lint-spec-files.py --all` | Passed |
| `git diff --check` | Passed |

The listed Python/Node workflow and helper suites contain 249 passing cases. The shell
contract also passed. The action-ref linter checked all 30 workflows.

Zizmor 1.25.2 reports the expected 10 reviewed residual findings, exit 14:
6 high trigger warnings, 3 low CMD limitations, and 1 informational action
suggestion. There are no remaining interpolation, checkout-persistence, or docs
cache findings. Existing suppressed findings decreased from 229 to 223 without
adding or changing suppressions. See the [finding ledger](findings.md).

No application UI or public operator contract changed. Screenshots and public
documentation edits are not required. PR CI and bot-review evidence remain
pending until the delivery workflow completes.

## Adversarial review, 2026-10-26

An independent session reviewed the initial implementation at `5b069a78953`.
It reproduced one blocker with the real pinned OpenCode v1.19.3 CLI: reading
prepared source automatically loaded nearby PR-owned instruction filenames.
All six root/nested `AGENTS.md`, `CLAUDE.md`, and `CONTEXT.md` cases loaded
instructions. Controls with neutral filenames did not load those instructions.

The shared helper now writes neutral blob names and a version 2 manifest mapping.
All automatic review and walkthrough prompts consume that mapping. The new
context regression and all three automatic-review subcases failed before the
fix. They pass after it. Coverage includes suffix collision names, exact bytes,
omissions, budgets, deterministic output, and partial clones.

The independent reviewer repeated the actual preparation pipeline with the
modified helper and checksum-verified OpenCode binary. All 10 real CLI reads
returned `metadata.loaded=[]` with no `system-reminder`. They covered ordinary
source, root/nested instruction files, and suffix collision names. File bytes
matched immutable Git blobs. Identical blobs shared a filename without losing
their separate original-path entries. The reviewer found no remaining defect.
The CLI probe used `debug agent github-pr-review --tool read` with the workflow
agent definition, isolated HOME/XDG directories, and no provider credentials.

Structural comparison found unchanged trigger sets, permissions, dependencies,
environments, outputs, and matrices across the original seven changed workflows.
All 217 existing release/image/docs step identities and conditions were unchanged.
Actionlint 1.7.12 reported only the same unsupported `queue` key on base and head.
Diagnostic copies with that existing key removed passed on both revisions.
No repository configuration was changed for the linter.

The initial head completed CI with 62 passed, 25 skipped, and one neutral check.
Its mobile queue test failed the strict flake gate, passed local reproduction
checks without retries, and passed one failed-job rerun without source changes.
These CI results are historical after remediation. Fresh head CI and bot reviews
are required after the fix is pushed. No live provider or publication was tested.

## Delivery CI fixture corrections

CI run `40317924466` at `9e2d3fe6bd7` reported three flaky mobile tests and a
failed aggregate gate. Each case passed its retry, but the strict flake gate
correctly rejected the run. Local investigation identified test synchronization
and cleanup defects. No production UI change or timeout increase is required.

- The transcript test captured a temporary post-submit clamp as its baseline.
  A two-CPU run recorded the original offset `4988`, the captured offset `4959`,
  and the restored offset `4988` before the reply. Capture the owned position
  before submission. The original failure and one of three instrumented repeats
  reproduced the same 29px assertion error.
- The breadcrumb test measured the loading placeholder after its final folder
  tap. A controlled 500ms final-listing delay reproduced the old assertion failure.
  Wait for the disabled current-directory breadcrumb before measuring. Retain
  every containment, trailing-edge, overflow, visibility, and touch assertion.
- The MCP clarity helper removed its route while a GET callback was still active.
  The failed blob report places that callback across the helper's final assertion.
  Playwright continues an active request when its last handler is removed, so
  the callback's later fulfillment fails. Drain this helper's completion barriers
  before removing its exact handler. Preserve other routes and propagate errors.

The original baseline and current main contained the same MCP clarity helper.
The CI fixes preserve the existing transcript, directory-selection, and MCP
contracts. Product requirements, public docs, and screenshots need no change.
Main `b4e9cf5ede3` was integrated locally with normal hooks before final checks.

Both scroll and breadcrumb corrections passed three repetitions on two CPUs
with retries disabled. A fresh managed build against the combined source passed
all three exact failed cases before the final MCP cleanup change. After that
change, the full affected suites passed: 16 mobile and 9 desktop browser cases,
on two CPUs with retries disabled. The independent reviewer found no remaining
defect in the three corrections. The diagnostic route delays and console logs were removed.

The full web typecheck passed with process-local
`NODE_OPTIONS=--max-old-space-size=8192` on two CPUs. Its first attempt exceeded
Node's default heap. Scoped ESLint passed for the three changed E2E files.
The 249 workflow/helper cases, shell contract, action pinning, document checks,
and diff checks passed again after main integration. Zizmor returned the expected
exit code 14 with the same 10 reviewed residual findings. Local PR-documentation
coverage validation returned `covered` with no errors.

The next CI run, `40326593499` at `e8c8718d1d5`, reported two more flaky
fixtures. Both passed their retries, so the strict flake gate remained active.
The tablet workflow test measured a 44px control as `43.99993896484375px`
during drawer animation. It now waits for the actual drawer animation and uses
the existing touch-control helper's 0.01px precision allowance. The mobile
sidebar test measured Retry before that button was visible. Both recovery cases
now wait for the Retry button within their own error row before measuring it.
The 44px touch requirement and product rendering are preserved.

With retries disabled on two CPUs, all six repeated mobile recovery cases and
all five desktop/tablet workflow-navigation cases passed. These are additional
browser cases, separate from the 249 workflow/helper cases above.
The independent reviewer found no weakened behavioral contract or remaining
defect in these two corrections. Scoped ESLint, formatting, document validation,
and the full web typecheck with the same process-local 8 GiB heap passed again.

CI run `40330795890` at `a7b70c02593` passed 64 checks and skipped 19, but
failed two E2E shards and their aggregate. An audit of all 118 blob reports and
the retry summary found 4261 first-attempt passes, 2 retry passes, and 40 skipped
browser cases. Both flaky verdicts remained failures under the strict gate.
CodeRabbit reviewed the latest delta and reported no actionable comments.

The review checkbox scenario inherited two files from preceding CLI tests.
Its failure screenshot contained four files, while the fresh-worker retry
contained only its intended two. The same CLI, watcher, and review sequence
reproduced the checkbox failure locally with retries disabled. Reset the
disposable seed checkout with the existing helper before task creation, and
assert exactly two review headers. This preserves the shared fixture boundary.

A repeated mobile check then exposed a separate gesture defect. Browser probes
recorded a touch starting on a code node that became disconnected during diff
rendering. The content scrolled, but no touch-move event reached the review root.
The gesture now starts on the persistent file header and keeps its coordinates
within the viewport. It still uses native touch input and checks real scrolling.
All checkbox, manual-uncheck, return-and-pass, and disabled-auto-mark assertions
remain unchanged. The probes were removed after the cause was established.

The manual-port case also failed once in that CI run and passed its retry.
Its exact case passed three local repetitions, and all 11 cases in its spec
passed with retries disabled on two CPUs, including the preceding tests.
The Add operation is synchronous local state. Available evidence does not prove
a product defect, so its original test and product code are retained. Fresh CI
remains the delivery gate for this case.

The corrected native gesture passed five mobile repetitions without retries on
two CPUs. The final ordered CLI, watcher, and review sequence passed all 12
cases without retries. Full web typecheck with the process-local 8 GiB heap, scoped ESLint,
formatting, document validation, and diff checks passed again.

CI run `40346258385` exposed two further synchronization defects. The approval
test read persisted status after the optimistic picker displayed Done, before
its PATCH completed. A controlled 500ms request delay reproduced the exact
`in_review` assertion failure. The test now arms `waitForHttp` before clicking,
requires a successful PATCH response, and keeps the original immediate backend
Done assertion. The delayed case and all three approval-flow cases passed.
The injected delay was removed.

The oversized-message task switch tapped a row while its drawer was opening.
The failed run tapped the second row shortly after visibility. A local probe
measured taps at 250ms and 33ms of the 500ms drawer animation. The helper now
uses the existing finite-animation wait before its real tap. Navigation,
message storage, bounded preview, download bytes, queue behavior, and touch-size
assertions remain unchanged. The probe was removed.
All three repeated oversized-message cases passed with retries disabled on two
CPUs. Full web typecheck with the process-local 8 GiB heap, scoped ESLint,
formatting, document validation, and diff checks also passed.

That CI run finished with 64 passed checks, 19 skipped checks, and 3 failures
(the two shards and their aggregate). All 118 blob reports and the retry
summary agree: 4261 first-attempt passes, 2 retry passes, and 40 skipped browser
cases. No other failure was found. CodeRabbit reviewed that head with no
actionable comments.

Main `7fb28fc022d` was integrated with normal hooks. Its Copilot change
has no overlapping files and does not change the workflow/helper verification
inputs. A fresh managed build passed the oversized-message case, and all three
approval-flow cases passed with retries disabled on two CPUs. The first office
attempt hit an occupied port before setup; a checked explicit test-port offset
resolved that infrastructure collision. Full web typecheck passed again with
the process-local 8 GiB heap. Fresh-head CI remains the external delivery gate.

CI run `40355035994` at `be8d2ab3974` finished with 65 passed checks, 19 skipped,
and 2 failures: the branch-selector shard and its aggregate. All earlier fixes
passed. The 118 blob reports and retry summary agree on 4264 first-attempt
passes, 1 retry pass, and 40 skipped browser cases.

The branch-refresh fixture opened the executor picker immediately after
repository selection. The trace shows its Worktree option becoming detached
before any click was dispatched. It proves premature menu closure, but does
not establish its exact cause. Fresh-worker repetitions and controlled motion
probes did not reproduce the timeout. The fixture now waits for focus to return
to the repository trigger before opening the executor picker. The adjacent
filter case uses the same readiness precondition. The independent reviewer
confirmed that this serializes the supported popover lifecycle without weakening
the profile, branch, filtering, or refresh-request assertions.

Three focused repetitions and all 13 cases in the file passed on two CPUs with
retries disabled. The three describe-level retry settings were temporarily
disabled for this check and restored before commit. Motion and focus probes
were removed. Full web typecheck with the process-local 8 GiB heap, scoped
ESLint, formatting, and diff checks passed. Targeted open-PR searches found no
branch-refresh fix. Open PR #3968 covers other flaky tests and does not modify
this fixture or its popover components.

The base then advanced to `6a1a2ae1000`. Its worktree-placement change has no
overlapping files. Current-base compatibility remains a separate delivery check.

The committed picker fix `0bff99cfd84` was recovered unchanged into an independent
checkout after the shared repository developed an empty HEAD object. It was
pushed with its existing three-repeat and full 13-case verification evidence.
Run `40372137482` passed shard 27, including the previously failing picker case.

That run exposed three retry-passing cases in shards 6 and 19. The mobile
silent-prompt test spent over 12 seconds navigating and establishing readiness,
but its mock emitted output after 10 seconds. The failed snapshot already showed
the progress text; the backend log confirmed that the real watchdog had fired.
A temporary 12-second navigation delay reproduced the same missing-notice
assertion. Replacing the initial timed delay with the existing mock-agent
`e2e:await_file` command passed under the same probe. The fixture releases output
after asserting the notice and its touch target, and cleans up the release file.
The output, ongoing-turn, reload, and overflow assertions remain unchanged.

The mobile routing test read configuration immediately after clicking Save.
A temporary 500 ms delay before its PUT reproduced the missing role-tier value.
Waiting for the existing save-confirmation toast passed with that delay. The
toast is emitted after the awaited update. Persistence and reload assertions
remain unchanged. Both diagnostic delays were removed before full verification.

The quick-chat failure retained its finished indicator while the navigation
menu stayed open. Its first failure has a screenshot, while the trace covers
only the successful retry. The exact case passed three local repetitions and
both cases in its full spec passed without retries. No speculative product or
fixture change was made for that case; the next complete CI run remains its
delivery check.

On two CPUs with retries disabled, all 8 cases across the affected mobile specs
and all 5 desktop watchdog cases passed. Full web typecheck with the process-local
8 GiB heap, scoped ESLint, formatting, document checks, and diff checks passed.
The local runner required removal of inherited launcher-config variables and
explicit loopback binding, plus the host's Chromium library paths. Setup failures
before those corrections were not counted as test evidence. Product code,
timeouts, retry policies, and negative assertions were not changed.

Run `40380935160` at `f9b518abed5` exposed a different shard-19 failure: the
Office title-edit fixture read the original backend title after the optimistic
heading had already changed. The first attempt failed and the CI retry passed.
The production title commit deliberately patches local state before awaiting
`PATCH /api/v1/tasks/:id`. The test is unchanged on main `fb4c6f19c00`.

Three unmodified local repetitions passed. A temporary 500 ms PATCH delay then
reproduced the exact original-title persistence failure. The fixture now arms
the existing HTTP-response helper before Enter, retains both immediate render
assertions, and requires a successful PATCH before its original backend read.
It passed under the same delay. After removing the diagnostic route, all five
cases in the title/description spec passed on two CPUs with retries disabled,
including rollback and cross-client synchronization. No product behavior,
timeout, retry policy, or original assertion changed.
Full web typecheck, scoped ESLint, formatting, document checks, and diff checks
passed. The remaining CI jobs were still pending when this fix was verified.


## Current-main integration and product races found by CI

On 2026-10-27, merge commit `5afaf501` integrated main
`d549fa694b7d5f8e1dd0d6bade40d4e2b41e71d3`. The two conflicts were in the
sidebar-recovery and review-checkbox fixtures. Resolution keeps main's drawer
animation wait and exact review-file identities, plus this PR's owned Retry
locators, canonical touch measurement, and stable native-swipe target. A fresh
build passed all four selected mobile cases and both selected desktop cases
with retries disabled on two CPUs.

Run `40398620748` at `df6cf03ec1174183a3a233cdeea9cbdf0dd3dd9e`
reported flaky verdicts in Canvas mobile sizing, Azure work-item browsing, and
Kubernetes task-pod restart. The complete 118-report audit found three retry
attempts and three first-attempt errors; the retry summary records 4,262
first-attempt passes, three passes after retry, and 40 skipped tests.

The Canvas Cancel control measured `43.99993896484375` pixels while its drawer
was entering. Its existing responsive class requires 44 pixels for coarse
pointers. The test now waits for finite drawer animation and uses the existing
0.01-pixel measurement allowance in `expectTouchControl`. The detail install
button uses that same helper. No control size or minimum target changed.

The Azure failure was a product race. An assignment callback unconditionally
selected its work item after the user had closed detail. The deterministic
regression holds the PATCH until detail closes, then requires the updated
assignee on the board and the continued absence of detail. It failed on the
original code after the board update proved that the response had been applied.
The callback now updates detail only when that same item is still selected.
The regression passed three desktop repetitions. All four cases in the full
mobile Azure and Canvas specs passed. All runs used two CPUs and zero retries;
the existing drag, persistence, navigation, and mobile assertions remain.

The Kubernetes backend log showed a retained runtime becoming available during
cold resume. The duplicate launch returned `ErrExecutionAlreadyRunning` after
writing `STARTING`; its recovered availability event had already been rejected
because the session was no longer idle. The accepted prompt was requeued and
never dispatched. Executor and SQLite-backed queue regressions reproduced the
same state and missing-prompt failures without a Kubernetes cluster.
The fix restores the previous idle state only when the recovered agent is ready
for input, using the existing startup-attempt and state comparison. An unready
runtime retains startup state. Cancellation, a running turn, or a newer attempt
wins over restoration. The original Kubernetes E2E test is unchanged.

Targeted executor resume/rollback, recovered-availability, and queued-prompt
checks passed under `go test -race -trimpath -p 1 -count=1 -timeout=5m` on two
CPUs, including the production rollback callback and exactly-once queue dispatch.
A local Kind run was unavailable because this host has no Docker daemon; the
fresh CI container shard remains the real-cluster verification gate.

The security helper digest is unchanged. All 249 workflow/helper Python and
Node cases, the OpenCode shell contract, and pinning of all 30 workflows passed
on the merged tree. Zizmor 1.25.2 retains exactly the same 10 reviewed findings
without new suppressions. These fixes enforce existing behavior; no public
configuration, API, publication gate, timeout, or retry policy changes.

Full web typecheck, scoped ESLint/formatting, pinned golangci-lint 2.9.0 for the
two changed Go packages, document catalog/spec lint, and diff checks passed.
Recovery fix `07ed9feb` and Azure fix `6a820eb2` passed normal commit hooks.
Fresh exact-head CI and review evidence remain the delivery gate after push.

A follow-up caller-level regression found that a lost state comparison could
still reach generic launch-failure handling. It reproduced `RUNNING` and a newer
`STARTING` attempt becoming `FAILED`. The executor now retains both the
already-running and superseded-state error identities when another owner wins;
other rollback errors still propagate. The caller then applies its normal
current-state prompt gate. SQLite-backed cancellation, running-session, and
newer-attempt cases all pass under the race detector, preserve the successor
state, send no prompt, and retain queued messages for active successors. The
broader targeted resume/rollback/availability/queue race suite also passes.


## Mobile Stats recovery fixture

Delivery CI exposed a race in the mobile Stats retry fixture. The original test
failed locally on its third repetition with retries disabled. A request/event
probe reproduced the failure again: the first daily-activity response returned
503, then the initial browser `pageshow` event triggered the supported foreground
recovery. The fixture allowed that second request to succeed, so the error panel
was gone before its manual Retry assertion.

The fixture now keeps daily activity unavailable until the enabled Retry control
is ready to be tapped. It retains the 44px touch-size check, successful response,
error-panel removal, enabled Copy Stats control, and no horizontal overflow.
Five mobile repetitions passed with retries disabled on two CPUs. The unchanged
desktop companion passed three repetitions under the same limits. Scoped ESLint,
formatting, document checks, and diff checks passed. No production UI or recovery
behavior changed; public guides and screenshots remain current. Fresh CI after
the next push remains the delivery gate.


## Current-main integration after delivery CI

Head `7c12c83d9143824c6e9a33fc4a696182cd66411b` finished with 86 passing,
13 skipped, and two failed checks (one E2E leaf and its aggregate). All 118
Playwright reports were inspected: 4,281 passed attempts, 40 skipped attempts,
and one failed Azure attempt followed by a passing retry. No other test retried.
The Azure failure seeded saved views before the configuration POST completed;
the store correctly returns not-configured when that configuration row is absent.

Main `d27d8e59302f983a424b7147a37b405ffaa81d99` already contains the causal
configuration-response wait and a delayed-save regression stimulus. Integration
retains that fix and this PR's independent late-assignment selection regression.
The two mobile test conflicts preserve main's screenshot/scroll coverage and
clock-driven automatic Stats retry coverage, together with this PR's scoped
Retry locator, canonical touch measurement, and enabled manual-retry gate.
The recovery and privileged-workflow security changes remain intact.

A fresh managed build of this merged tree passed the full Azure desktop spec
three times with retries disabled. The resolved sidebar and Stats mobile specs
passed all nine repetitions. Each browser run used two CPUs. The targeted
executor, queued-prompt, native-resume lifecycle, and SQLite terminal-proof tests
also passed under the Go race detector, including the newly integrated
interrupted-settlement recovery cases. Scoped ESLint, formatting, catalog/spec
checks, and diff checks passed. The security context helper SHA-256 remains
`8cb3ddc50e47ff21fe15d0484d3f76fdbca1d57fc12a785872bc5479f9aa5bb8`.


The branch was then rebased onto the same main revision at the user's request.
The rebased source and test trees are byte-identical to the validated merged
tree. Main leaves `E2E_FAIL_ON_FLAKY` unset; the Playwright CI configuration
fails on retry passes only when that variable equals `1`. No diagnostic probe
or additional workflow-test change is retained. New-head CI will rerun after
pushing the rebased history.
