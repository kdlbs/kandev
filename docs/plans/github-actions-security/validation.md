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
