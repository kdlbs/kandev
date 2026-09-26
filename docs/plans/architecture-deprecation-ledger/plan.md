---
created: 2026-09-26
status: done
requirements:
  - REQ-ARCHITECTURE-LINT-DEPRECATION-001
system_design:
  - ../../specs/architecture-lint/system-design/deprecation-ledger.md
---

# Implementation Plan: Explicit Deprecation Ledger Rule

## Scope

Add one modular architecture rule that requires newly annotated production Go and TypeScript declarations to match a valid compatibility-ledger registration. Preserve existing unregistered annotations in an exact shrink-only baseline. Keep all existing architecture rules and compatibility entries intact.

This changes repository engineering tooling. The internal requirement and design define its contract. Product behavior and customer-facing compatibility promises remain unchanged. See [REQ-ARCHITECTURE-LINT-DEPRECATION-001](../../specs/architecture-lint/requirements/deprecation-ledger.md) and its [system design](../../specs/architecture-lint/system-design/deprecation-ledger.md).

Decision: [Architecture deprecation ledger](../../decisions/2026-09-26-architecture-deprecation-ledger.md).

## Work order

- [x] [Task 01 — Deprecation ledger rule](task-01-deprecation-ledger-rule.md)

## Integration order

PR #3974 registers 31 Office aliases through `locator.declaration`. This PR
adds the schema and validator for that field. Merge #3975 first. Then rebase
#3974 on updated `main` and run `make lint-architecture`.

## Verification

Run the architecture-lint suite and full lint, compare the initial baseline against current `origin/main` with the bootstrap allowance, validate the decision and plan records, run relevant harness checks, and check the final diff.

## Results

Completed on refreshed main `c735b678863ba64e31bd78cd1a6e3c845be3b4e9`. The exact initial baseline contains 15 unregistered declarations: 4 Go and 11 TypeScript. Review regressions expanded the scanner to grouped and embedded Go declarations, nested and decorated TypeScript declarations, canonical non-identifier member keys, and quote termination across JSX text.

- `python3 scripts/lint-architecture.test.py` — passed, 95 tests after identity hardening.
- `make lint-architecture` — passed.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main --allow-missing-base-baseline` — passed; the new baseline bootstraps at 15 exact current findings.
- `python3 scripts/list-docs.py validate` — passed with the internal requirement/design pair; 310 decisions and 1188 specifications validated.
- `python3 scripts/list-docs.py decisions --format paths` — includes the new decision.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `python3 scripts/lint-harness-files.test.py` — passed, 19 tests; `make lint-harness` passed for all 199 harness files.
- `git diff --check` — passed.
- Coverage correction: code-only head `65f56956cb6b2f800485031caaec3fa8b30de097` failed `PR documentation coverage` (run `36259598400`). Its error was “Linked delivery package is incomplete.” The later head `a2cf91d63db6398a5f3eb9fa1730e1fbfed5a0d2` passed after the requirement/design pair was added (run `36261418199`).
- No product E2E or public-documentation changes were required.
