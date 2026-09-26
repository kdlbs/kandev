---
decision: docs/decisions/2026-09-26-architecture-deprecation-ledger.md
created: 2026-09-26
status: done
---

# Implementation Plan: Explicit Deprecation Ledger Rule

## Scope

Add one modular architecture rule that requires newly annotated production Go and TypeScript declarations to match a valid compatibility-ledger registration. Preserve existing unregistered annotations in an exact shrink-only baseline. Keep all existing architecture rules and compatibility entries intact.

This is internal repository tooling. The accepted decision is the requirements source; no product requirement or system design applies.

## Work order

- [x] [Task 01 — Deprecation ledger rule](task-01-deprecation-ledger-rule.md)

## Verification

Run the architecture-lint suite and full lint, compare the initial baseline against current `origin/main` with the bootstrap allowance, validate the decision and plan records, run relevant harness checks, and check the final diff.

## Results

Completed on refreshed main `b82bfd2cead7c3e74b99fbc47ee2202c92a36c39`. The exact initial baseline contains 15 unregistered declarations: 4 Go and 11 TypeScript.

- `python3 scripts/lint-architecture.test.py` — passed, 80 tests.
- `python3 scripts/lint-architecture.py --all` — passed.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main --allow-missing-base-baseline` — passed; the new baseline is absent from the target branch and bootstraps at 15 exact current findings.
- `make lint-architecture` — passed.
- `python3 scripts/list-docs.py validate` — passed; 310 decisions and 1185 specifications validated.
- `python3 scripts/list-docs.py decisions --format paths` — includes the new decision.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `python3 scripts/lint-harness-files.test.py` — passed, 19 tests; `make lint-harness` passed for all 199 harness files.
- `git diff --check` — passed.
- No product E2E or public-documentation changes were required.
