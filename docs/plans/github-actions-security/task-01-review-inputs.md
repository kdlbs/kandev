---
id: "01-review-inputs"
title: "Isolate automatic review inputs"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-CI-ACTIONS-SECURITY-001
acceptance_criteria:
  - AC-CI-ACTIONS-SECURITY-001.1
  - AC-CI-ACTIONS-SECURITY-001.2
  - AC-CI-ACTIONS-SECURITY-001.3
system_design:
  - ../../specs/ci/system-design/privileged-workflow-inputs.md
---

# Task 01: Isolate automatic review inputs

## Summary

Keep trusted workflow files in the privileged review worktree. Supply exact PR
data without loading PR-owned agent configuration or review instructions.

## In scope

- Claude fork review and both OpenCode automatic review paths.
- Optional event-head binding in the existing Claude GET-only file helper.
- Trusted history/context preparation and agent policies from the system design.
- Shared neutral context filenames and manifest mapping for review and walkthrough agents.
- Raw contracts, behavioral fixtures, CI registration, and ADR reconciliation.

## Out of scope

Models, providers, generic Claude mentions, same-repository Claude authorization,
review event sets, label persistence, and OpenCode evidence classification.

## Acceptance

1. A fixture PR containing root OpenCode plugin configuration, Claude settings,
   changed review skills, and symlinks cannot affect execution files or run a
   marker command. The agent stub still receives the exact event diff and bounded
   text for a newly added regular file. Head mismatch fails before the stub runs.
2. Raw contracts assert workflow-SHA checkouts, exact-head object fetches, trusted
   setup/publisher provenance, and no PR-head checkout before privileged execution.
   The Claude helper rejects a mismatched pinned head and preserves unpinned manual use.
3. Existing event gates, protected App producer identity, full-PR evidence,
   advisory fork posting, and diagnostic failures retain their tested behavior.

## Implementation notes

Start with failing regression tests. Use the walkthrough's bounded context
helper directly where suitable. Keep it at its trusted path. Do not copy PR
files into `.opencode`, `.claude`, `.agents`, or the worktree root.

Add `.github/scripts/automatic-review-inputs_test.py` for temporary Git fixtures
and a stub agent. Test preparation and shell invocation without a real model or
credential. Verify source and output paths, not just matching YAML strings.
Update `scripts/opencode-code-review.test.sh` assertions that currently require
two head checkouts. Preserve its immutable-head and producer assertions.

The pinned Claude action restores selected settings but not the review skill.
Record that mitigation. Do not claim a demonstrated Claude hooks exploit.
Review the proposed ADR and update the accepted OpenCode checkout clause after
tests pass. Update the shared CI design to reference the new execution boundary.

## Verification

Run from the repository root:

```bash
python3 .github/scripts/automatic-review-inputs_test.py
python3 .github/scripts/claude-code-review-workflow-contract_test.py
python3 .github/scripts/claude-read-pr-file_test.py
python3 .github/scripts/opencode-code-review-workflow-contract_test.py
bash scripts/opencode-code-review.test.sh
python3 scripts/opencode-code-review.test.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-context.test.py
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Zizmor remains nonzero until all findings receive their recorded disposition.
Keep the six target triggers. Do not suppress them to make this command pass.

## Files likely touched

- `.github/scripts/prepare-pr-review.sh`
- `.github/workflows/{claude-code-review,opencode-code-review,lint-action-pinning}.yml`
- `.github/scripts/{claude-code-review-workflow-contract_test,opencode-code-review-workflow-contract_test,claude-read-pr-file,claude-read-pr-file_test,automatic-review-inputs_test}.py`
- `scripts/opencode-code-review.test.sh` and, if behavior coverage requires it, `scripts/opencode-code-review.test.py`
- The new ADR, the accepted OpenCode evidence ADR, shared CI design, and this package's evidence files.

## Dependencies

None. Before edits, repeat the current-main/open-PR check and read the current pins.

## Risks

Agent startup behavior must be checked at the pinned version. A string-only
contract cannot prove safe materialization. Do not loosen posting permissions
or move the dedicated App credential into the model step.

## Parallelism

`sequential`

## Inputs

The paired design's review sections, [findings.md](findings.md), existing manual
Claude helper tests, walkthrough context tests, and OpenCode posting tests.

## Results

The targeted tests and non-scanner checks passed. Automatic review fixtures: 6 passed. Claude
contracts: 11 passed. File reader: 9 passed. OpenCode contracts: 4 passed. Posting
parser: 20 passed. Context helper: 8 passed. Walkthrough contracts: 30 passed.
Action-pinning tests: 9 passed. The shell contract passed. Specification and diff
checks passed. Zizmor retains the 10 reviewed residual findings, with no new
suppressions. See [validation.md](validation.md).

The adversarial review reproduced reserved-name instruction discovery with the
real pinned CLI. Red regressions failed for all three automatic review paths.
The shared helper now uses neutral blob names and a version 2 original-path
mapping. All consumers read through that mapping. Tests retain exact bytes,
omission reasons, limits, deterministic output, and partial-clone behavior.
