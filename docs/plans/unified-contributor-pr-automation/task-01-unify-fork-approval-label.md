---
id: "01-unify-fork-approval-label"
title: "Unify fork approval label gates"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ci/requirements/unified-contributor-pr-automation.md"
system_design: "../../specs/ci/system-design/unified-contributor-pr-automation.md"
---

# Task 01: Unify fork approval label gates

## Acceptance

- The allowlist label job adds exactly `safe-to-review` and no longer adds
  `safe-to-test`.
- The fork preview job uses `safe-to-review` as its label gate and retains its
  existing direct allowlist gates and privileged checkout and deploy command.
- The existing Claude and OpenCode fork review gates retain their current
  provider-specific event and permission behavior.
- No active workflow authorization expression uses `safe-to-test`.
- Approval remains durable across synchronize events. No label cleanup job is
  reintroduced.
- The standalone OpenCode workflow test no longer asserts an obsolete cleanup
  job and passes with the persistent-label contract.

## TDD scenarios

1. RED: Change the Claude and preview contract tests to require one label and
   reject `safe-to-test` authorization.
2. GREEN: Update the Claude label bridge and preview workflow expressions and
   comments.
3. GREEN: Align the standalone OpenCode workflow test with persistent labels.
4. REFACTOR: Keep the existing direct allowlist expressions and minimize the
   workflow diff.

## Verification

```bash
python3 .github/scripts/claude-code-review-workflow-contract_test.py
python3 .github/scripts/preview-env-workflow-contract_test.py
python3 .github/scripts/opencode-code-review-workflow-contract_test.py
bash scripts/opencode-code-review.test.sh
python3 .github/scripts/lint-action-pinning_test.py
python3 .github/scripts/lint-action-pinning.py
zizmor .github/workflows/claude-code-review.yml .github/workflows/opencode-code-review.yml .github/workflows/preview-env.yml
git diff --check
```

## Files likely touched

- `.github/workflows/claude-code-review.yml`
- `.github/workflows/preview-env.yml`
- `.github/scripts/claude-code-review-workflow-contract_test.py`
- `.github/scripts/preview-env-workflow-contract_test.py`
- `.github/scripts/opencode-code-review-workflow-contract_test.py`
- `scripts/opencode-code-review.test.sh`

## Dependencies

None. Task 02 consumes this label contract.

## Parallelism

Sequential. This task owns the shared label meaning and preview gate.

## Risks

- The preview job has deployment credentials and checks out contributor code.
  Keep the existing privileged path visible in the contract tests and decision
  record.
- A label written by `GITHUB_TOKEN` does not start another workflow run. Do not
  remove direct allowlist gates.

## Output contract

Report the exact label expressions, the direct allowlist paths retained, all
contract and security checks, and the old-label migration state. Update this
task and `plan.md` with the result.

## Results

The automatic label bridge now adds only `safe-to-review`, and the fork
preview gate uses that label while retaining its direct allowlists. Focused
workflow contracts, action pinning, the standalone OpenCode workflow test, and
credential-persistence checks passed. Direct `zizmor` still reports the
repository's existing `pull_request_target` trigger findings; no new trigger
type was introduced.

During PR fixup, the legacy specification was narrowed to the single active
label and the standalone artifact-upload assertion was strengthened to compare
every upload reference with the exact required pin. The focused test and
specification lint passed, and the final PR check snapshot was fully green.
