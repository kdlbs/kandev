---
id: "01-deprecation-ledger-rule"
title: "Add explicit deprecation ledger rule"
status: done
wave: 1
depends_on: []
plan: "plan.md"
decision: "../../decisions/2026-09-26-architecture-deprecation-ledger.md"
---

# Task 01: Add explicit deprecation ledger rule

## Outcome

New canonical production Go and TypeScript deprecation annotations require a matching compatibility-ledger registration with existing owner, reason, introduction, removal-condition, and target metadata. Existing unregistered annotations remain exact baseline findings.

## Acceptance

- `ARCH-DEPRECATION-LEDGER` detects Go `Deprecated:` declaration comments and TypeScript JSDoc `@deprecated` declarations with stable `(path, declaration, marker)` identities.
- A valid matching ledger registration passes; a missing or mismatched registration fails with deterministic actionable diagnostics.
- Tests cover baseline bootstrap/shrink behavior, repeat declarations, and generated/test/fixture/third-party/string/prose exclusions while preserving existing ledger date, version, staleness, and marker checks.
- The initial baseline contains exactly the current unregistered production declarations on the refreshed main head.

## Scope and exclusions

Owned files include the architecture rule/helper/tests, the deprecation baseline, narrowly required optional `locator.declaration` validation, the architecture rule registry and guide, and this decision/work-order package.

Do not add compatibility-keyword discovery, a global deprecation ban, automatic ledger or baseline rewrites, unrelated baseline changes, product behavior, or the separate PR documentation-coverage evaluator. Do not require immediate removal or registration of baseline declarations.

## Requirements and system design

No product `REQ-*`, `AC-*`, or system-design document applies. This internal tooling decision is recorded in `docs/decisions/2026-09-26-architecture-deprecation-ledger.md`.

## Verification

```bash
python3 scripts/lint-architecture.test.py
python3 scripts/lint-architecture.py --all
python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main --allow-missing-base-baseline
make lint-architecture
python3 scripts/list-docs.py decisions --format paths
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

Implemented and verified on refreshed main `b82bfd2cead7c3e74b99fbc47ee2202c92a36c39`.

- The initial baseline contains exactly 15 unregistered declarations: 4 Go and 11 TypeScript.
- The two existing compatibility-ledger entries are unchanged.
- `python3 scripts/lint-architecture.test.py` — passed, 80 tests.
- `python3 scripts/lint-architecture.py --all` — passed.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main --allow-missing-base-baseline` — passed.
- `make lint-architecture` — passed.
- `python3 scripts/list-docs.py validate` — passed; 310 decisions and 1185 specifications validated.
- `python3 scripts/lint-spec-files.py --all` — passed.
- `python3 scripts/lint-harness-files.test.py` — passed, 19 tests; `make lint-harness` passed for all 199 harness files.
- `git diff --check` — passed.
