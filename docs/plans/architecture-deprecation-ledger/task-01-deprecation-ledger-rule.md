---
id: "01-deprecation-ledger-rule"
title: "Add explicit deprecation ledger rule"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-ARCHITECTURE-LINT-DEPRECATION-001
acceptance_criteria:
  - AC-ARCHITECTURE-LINT-DEPRECATION-001.1
  - AC-ARCHITECTURE-LINT-DEPRECATION-001.2
  - AC-ARCHITECTURE-LINT-DEPRECATION-001.3
  - AC-ARCHITECTURE-LINT-DEPRECATION-001.4
  - AC-ARCHITECTURE-LINT-DEPRECATION-001.5
system_design:
  - ../../specs/architecture-lint/system-design/deprecation-ledger.md
---

# Task 01: Add explicit deprecation ledger rule

## Outcome

New canonical production Go and TypeScript deprecation annotations require a matching compatibility-ledger registration with existing owner, reason, introduction, removal-condition, and target metadata. Existing unregistered annotations remain exact baseline findings.

## Acceptance

- `AC-ARCHITECTURE-LINT-DEPRECATION-001.1`: The rule detects attached Go `Deprecated:` comments and TypeScript JSDoc `@deprecated` tags with exact path, declaration, and marker identities, while excluding generated/test/fixture/third-party sources, ordinary prose, and strings.
- `AC-ARCHITECTURE-LINT-DEPRECATION-001.2`: A valid matching ledger registration passes; a missing, mismatched, or stale registration fails with deterministic actionable diagnostics.
- `AC-ARCHITECTURE-LINT-DEPRECATION-001.3`: Tests prove member identities include their owners. They prove overload identities stay stable after formatting and reordering. They reject indistinguishable duplicates.
- `AC-ARCHITECTURE-LINT-DEPRECATION-001.4`: Baseline bootstrap/shrink behavior remains exact, and removed declarations require baseline or ledger cleanup.
- `AC-ARCHITECTURE-LINT-DEPRECATION-001.5`: Date targets remain valid through their stated date and expire after it; SemVer targets remain review checkpoints.
- Tests cover same-named members in different containers, grouped Go declarations, and embedded fields.
- Tests cover nested or decorated TypeScript declarations, non-identifier member keys, and stale entries after overload removal.
- Compatibility tests cover current ledger date, version, staleness, and marker checks.
- The initial baseline contains exactly the current unregistered production declarations on the refreshed main head.

## Scope and exclusions

Owned files include the architecture rule/helper/tests, the deprecation baseline, narrowly required optional `locator.declaration` validation, the architecture rule registry and guide, and this decision/work-order package.

Do not add compatibility-keyword discovery, a global deprecation ban, automatic ledger or baseline rewrites, unrelated baseline changes, product behavior, or the separate PR documentation-coverage evaluator. Do not require immediate removal or registration of baseline declarations.

## Requirements and system design

The architecture-lint requirement defines this internal tooling contract. The system design describes its technical boundaries. The [ADR](../../decisions/2026-09-26-architecture-deprecation-ledger.md) records the durable decision. This change does not alter product behavior or customer-facing API contracts.

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

Implemented and verified on refreshed main `c735b678863ba64e31bd78cd1a6e3c845be3b4e9`. Review regressions cover grouped and embedded Go declarations, nested and decorated TypeScript declarations, string/numeric/computed member keys, local-variable exclusions, and apostrophes in JSX text. A follow-up identity regression also proves distinct TS overloads keep identities when reordered or reformatted, stale registrations fail when the referenced overload is removed, and indistinguishable repeated signatures are diagnosed.

- The initial baseline contains exactly 15 unregistered declarations: 4 Go and 11 TypeScript.
- The two existing compatibility-ledger entries are unchanged.
- `python3 scripts/lint-architecture.test.py` — passed, 95 tests after identity hardening.
- `make lint-architecture` — passed after identity hardening.
- `python3 scripts/lint-architecture.py --all --baseline-base-ref origin/main --allow-missing-base-baseline` — passed after identity hardening.
- `python3 scripts/list-docs.py validate` — passed with the internal requirement/design pair; 310 decisions and 1188 specifications validated.
- `python3 scripts/lint-spec-files.py --all` — passed after identity docs updates.
- `python3 scripts/lint-harness-files.test.py` — passed, 19 tests; `make lint-harness` passed for all 199 harness files.
- `git diff --check` — passed.
- Coverage correction: code-only head `65f56956cb6b2f800485031caaec3fa8b30de097` failed `PR documentation coverage` (run `36259598400`). Its error was “Linked delivery package is incomplete.” The internal requirement/design pair then linked the rule to this work order. Trusted-base evaluation passed at exact head `a2cf91d63db6398a5f3eb9fa1730e1fbfed5a0d2` (run `36261418199`).
