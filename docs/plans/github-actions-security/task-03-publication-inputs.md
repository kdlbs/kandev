---
id: "03-publication-inputs"
title: "Harden docs and image publication"
status: done
wave: 3
depends_on:
  - 02-release-inputs
plan: "plan.md"
requirements:
  - REQ-CI-ACTIONS-SECURITY-002
acceptance_criteria:
  - AC-CI-ACTIONS-SECURITY-002.1
  - AC-CI-ACTIONS-SECURITY-002.2
  - AC-CI-ACTIONS-SECURITY-002.3
  - AC-CI-ACTIONS-SECURITY-002.4
system_design:
  - ../../specs/ci/system-design/privileged-workflow-inputs.md
---

# Task 03: Harden docs and image publication

## Summary

Remove the documentation preview's cache dependency and both image checkout
credential defaults. Preserve image promotion gates and reviewed target triggers.

## In scope

`notify-docs.yml`, `ci-base-image.yml`, `universal-rebuild.yml`, their contract
tests, required CI registration, and final finding dispositions.

## Out of scope

Changes to landing publisher behavior, Docker cache policy, Windows launch
semantics, review triggers, or the existing GitHub release action.

## Acceptance

1. The docs preview has no explicit or automatic Node/package cache restore/save.
   Frozen install, trusted publisher checkout, fork exclusion, and the separate
   comment job remain intact.
2. Both image checkouts disable persisted credentials. The weekly date is passed
   as quoted data. Stubs prove it cannot execute shell text. The size gate still
   precedes promotion, and existing image push event gates remain unchanged.
3. Tests preserve the four reviewed target-trigger boundaries. The ledger records
   every residual by source, severity, condition, and evidence. The final scan
   introduces no unexplained findings or suppressions.

## Implementation notes

Add failing tests to `scripts/notify-docs-workflow.test.mjs`. Add
`.github/scripts/image-publication-workflow-contract_test.py` for both image
workflows. Register it in the unfiltered `lint-action-pinning.yml` required workflow.
Check all checkout occurrences in these jobs, not a single matching line.
Preserve CI image push/dispatch gates and weekly size-before-promotion order.

Use existing walkthrough, preview, and revocation suites for reviewed triggers.
Add precise assertions only where missing, such as reconciliation's workflow-SHA
checkout or no checkout in metadata cleanup. Keep all Windows CMD invocations
and the pinned release action. Do not suppress their scanner warnings.

After all tasks pass, update the finding ledger and final command results.
Promote the new requirement/design statuses only when implementation conforms.
Mark the plan implemented only after every required check has a recorded result.
An environment failure must remain explicit until resolved or accepted by the user.

## Verification

Run from the repository root:

```bash
node --test scripts/notify-docs-workflow.test.mjs
python3 .github/scripts/image-publication-workflow-contract_test.py
python3 .github/scripts/e2e-tests-workflow-contract_test.py
python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
python3 .github/scripts/preview-env-workflow-contract_test.py
python3 .github/scripts/revoke-ready-to-merge-workflow-contract_test.py
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning_test.py
zizmor .github/workflows
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The E2E timeout fixture resolves the installed system `sleep` before adding its
retry-sleep stub to PATH. The same timeout behavior now runs on the Nix host.
No browser E2E suite or publishing workflow dispatch is required.

## Files likely touched

- `.github/workflows/{notify-docs,ci-base-image,universal-rebuild,lint-action-pinning}.yml`
- `scripts/notify-docs-workflow.test.mjs`
- `.github/scripts/image-publication-workflow-contract_test.py`
- `.github/scripts/e2e-tests-workflow-contract_test.py` for the portable timeout fixture.
- Existing walkthrough/preview/revocation contract tests only for missing boundary assertions.
- This package and its paired requirement/design lifecycle metadata.

## Dependencies

Task 02 precedes this task. Final accounting includes both prior work orders.

## Risks

Disabling the docs cache increases installation time. A syntactic cache-key
change would leave the original restore path active. Removing an intentional
trigger would break contributor automation. Preserve each explicit boundary.

## Parallelism

`sequential`

## Inputs

The paired design's documentation and preserved-boundary sections, current raw
contract suites, the finding ledger, and baseline validation evidence.

## Results

The targeted tests and non-scanner checks passed: docs contracts 3, image contracts 3, E2E
workflow contracts 27, walkthrough contracts 30, preview contracts 7, revocation
contracts 10, backend workflow contracts 25, and action-pinning tests 9. The Nix
timeout-fixture failure was corrected by resolving system sleep before PATH
stubbing. Specification and diff checks passed. Final Zizmor: 10 reviewed
residuals. See [validation.md](validation.md).
