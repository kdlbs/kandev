# Validation evidence

Source baseline: `6677802efa4fac6c2ecc909cb433477c90f7fb2d`, 2026-10-25.
These results validate the investigation and document package. They do not
claim that pending workflow remediation is implemented.

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

## Scanner

Zizmor was absent from PATH. The official v1.25.2 Linux binary was downloaded to
`/tmp/kandev-security-tools`. Both the text command and `--format json` scan ran
against `.github/workflows`. The text scan exited 14 with the 40 unsuppressed
findings recorded in [findings.md](findings.md). The final scan reproduced the
same counts and exit code after the document-only edits.

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

All final commands below passed. The test-only timeout fixture now resolves the
installed system sleep before adding its fast retry stub to PATH. This fixes the
historical Nix failure while retaining the 0.5-second timeout assertion.


| Command | Result |
| --- | --- |
| `python3 .github/scripts/automatic-review-inputs_test.py` | 5 passed |
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
| `python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py` | 7 passed |
| `bash scripts/opencode-code-review.test.sh` | Passed |
| `node --test scripts/notify-docs-workflow.test.mjs` | 3 passed |
| `python3 .github/scripts/lint-action-pinning.py` | Passed |
| `python3 scripts/list-docs.py validate` | Passed |
| `python3 scripts/lint-spec-files.py --all` | Passed |
| `git diff --check` | Passed |

The Python/Node suites contain 247 passing test cases in total. The shell
contract also passed. The action-ref linter checked all 30 workflows.

Zizmor 1.25.2 reports the expected 10 reviewed residual findings, exit 14:
6 high trigger warnings, 3 low CMD limitations, and 1 informational action
suggestion. There are no remaining interpolation, checkout-persistence, or docs
cache findings. Existing suppressed findings decreased from 229 to 223 without
adding or changing suppressions. See the [finding ledger](findings.md).

No application UI or public operator contract changed. Screenshots and public
documentation edits are not required. PR CI and bot-review evidence remain
pending until the delivery workflow completes.
