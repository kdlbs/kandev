---
id: "02-release-inputs"
title: "Harden release input and Git authentication"
status: done
wave: 2
depends_on:
  - 01-review-inputs
plan: "plan.md"
requirements:
  - REQ-CI-ACTIONS-SECURITY-002
acceptance_criteria:
  - AC-CI-ACTIONS-SECURITY-002.1
  - AC-CI-ACTIONS-SECURITY-002.2
  - AC-CI-ACTIONS-SECURITY-002.4
system_design:
  - ../../specs/ci/system-design/privileged-workflow-inputs.md
---

# Task 02: Harden release input and Git authentication

## Summary

Consume the 20 flagged release expressions as data. Remove six persisted
checkout credentials while preserving release branch and signed-tag writes.

## In scope

The release workflow's flagged checkout and shell sites, release contract tests,
and directly affected raw workflow tests. Preserve the new Discord release jobs.

## Out of scope

Changing release channels, publication commands, artifact lists, signing policy,
backfill eligibility, external settings, or dispatching any release workflow.

## Acceptance

1. Every reported release expression reaches shell commands through environment
   data and quoted arguments. Stubbed commands receive hostile values literally,
   without executing marker commands. Helper downloads keep the workflow SHA.
2. All six flagged checkouts disable persistence. Branch delete/push and both
   signed-tag attempts authenticate through command-local helpers with the
   ordinary token. Only the existing administrator merge sees the bypass token.
3. Raw tests preserve normal Stable, dry-run, desktop-validation, latest-tag
   backfill, and manual/scheduled Nightly eligibility. Each actual publication
   job and downstream notification retains its required-success dependencies.

## Implementation notes

Write regression tests first in the existing release contract suite. Execute
extracted run blocks using stub `gh`, `git`, `docker`, and publication scripts.
Test quotes, spaces, backticks, command substitutions, and newlines without
real credentials. Confirm annotated-tag dereferencing remains correct.

Use step environment variables for the four high-severity helper-path findings
and 16 informational version/tag expressions. The high findings involve
GitHub repository/SHA metadata. No external injection into those values was
demonstrated. This is hardening, not evidence of four exploitable release paths.

The six checkouts are in `prepare`, `docker-amd64`, `docker-universal-amd64`,
`publish-release`, `publish-npm`, and `update-homebrew-tap`. Only `prepare` needs
Git pushes. Do not replace persistence with a job-wide credential helper.
Keep GPG signing, tag retry, exact merge-commit validation, and recovery guidance.

Trace each publication condition and source ref. Retain
`needs.prepare.outputs.ref` for tagged application inputs and `github.workflow_sha`
for control helpers and the Scoop publisher. If PR #4534 has landed, preserve
its SDK publication jobs and new completion dependencies.

## Verification

Run from the repository root with `gpg` available on `PATH`:

```bash
python3 .github/scripts/release-workflow-contract_test.py
python3 .github/scripts/announce-discord-release-workflow-contract_test.py
python3 .github/scripts/notify-release-contributors-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
git diff --check
```

Use raw workflow tests, not a YAML conversion that changes `on` to a boolean.
Do not dispatch publishing workflows. Explain remaining findings against the ledger.

## Files likely touched

- `.github/workflows/release.yml`
- `.github/scripts/release-workflow-contract_test.py`
- The directly affected notification contract tests, only if their data-handling assertions change.
- This package's findings and validation records.

## Dependencies

Task 01 precedes this task as the selected risk order. There is no code dependency.
Shared evidence files require serial updates.

## Risks

Removing credential persistence can break pushes. Incorrect shell quoting can
break tag dereferencing. Changing conditions while moving data can enable unwanted
publication or skip backfill builds. Test each boundary before completion.

## Parallelism

`sequential`

## Inputs

The paired design's publication and Git sections, the finding ledger, existing
release contracts, and ADR 0029 for workflow/application provenance.

## Results

The targeted tests and non-scanner checks passed: release contracts 69, Discord contracts 4,
contributor contracts 4, and action-pinning tests 9. Stubbed command tests verify
literal metadata arguments and branch/tag authentication including retry. The
scanner has no remaining release interpolation or checkout-credential findings.
The 10 reviewed residual findings remain. Diff checks passed. See
[validation.md](validation.md).
