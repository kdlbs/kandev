# ADR-2026-08-01-architecture-lint-budgets: Architecture Lint Budgets and Compatibility Expiry

**Status:** accepted (amended 2026-08-02)
**Date:** 2026-08-01
**Area:** infra

## Context

Kandev documents important package and state-ownership boundaries in `AGENTS.md` and ADRs, but
documentation alone does not prevent a fast-moving codebase from adding new violations. Existing
runtime, task, and frontend state-composition debt is too large to fail all at once. Compatibility
aliases and fallbacks also need consistent ownership and a concrete removal trigger.

This is internal CI and repository-process behavior. It changes no product behavior, so no product
spec is needed.

## Decision

Accepted architecture boundaries may be enforced with deterministic, dependency-free repository
checks backed by explicit reviewed finding allowlists under `config/architecture-lint/`.
Existing debt is grandfathered by exact path and import or source-marker identity. New findings
fail, removed findings make their exemptions stale, and CI compares each baseline with the target
branch so its reviewed allowlist may only shrink. Normal lint never rewrites baselines.

Each rule owns one scanner module under `scripts/architecture_lint/rules/`, one focused
test module under `scripts/architecture_lint_tests/`, and one baseline JSON file. A small explicit
registry is the only central rule list. Shared runner modules own repository discovery, baseline
comparison, diagnostics, and compatibility-ledger validation and contain no rule-specific scan
conditions. A rule whose baseline is absent from the target branch may bootstrap its initial
reviewed entries; after merge, that per-rule baseline becomes shrink-only.

Architecture-lint executable code is repository tooling owned under `scripts/`; reviewed
baselines and the compatibility ledger are durable repository configuration owned under `config/`.
The file under `.github/workflows/` is only a CI adapter that invokes the same codebase-owned
entrypoint used by Make and pre-commit. GitHub-specific directories do not own the linter's
implementation or policy data.

The initial checks enforce the documented agent-runtime import seam, the rule that shared task code
must not depend on Office models, and the narrow root Zustand composition boundary. Diagnostics
must identify the rule, source location, and intended replacement seam.

Intentional compatibility exceptions are registered in
`config/architecture-lint/compatibility-ledger.json`. Every entry requires a stable identifier and
source locator, reason, owner, introduction date or version, removal condition, and target removal
date or version. Date-based entries fail after expiry, and entries fail when their tracked path or
marker disappears. New compatibility behavior must register explicitly; broad keyword discovery
is not part of this foundation.

## Consequences

Pull requests cannot silently increase these known forms of architecture debt, while existing
cleanup can land incrementally. Baseline and ledger edits become visible review decisions, and
contributors receive migration guidance at the violating line. Source refactors that move an
allowed finding or compatibility marker require an intentional metadata update, which adds some
maintenance but prevents dead exemptions from accumulating.

Additional rules should be added only after their canonical contract is accepted. This decision
does not establish an aggregate architecture score or speculative checks for transport catalogs,
typed events, or generic compatibility keywords.

## Alternatives Considered

- **Documentation-only enforcement.** Rejected because it depends on every contributor and
  reviewer remembering every boundary on every change.
- **Immediately fail all current violations.** Rejected because it would couple this safety net to
  a large cleanup and make adoption impractical.
- **Use a raw aggregate architecture score.** Rejected because deleting one violation could hide a
  different newly introduced violation.
- **Allow permanently growing exception lists.** Rejected because allowlists without shrink-only
  comparison turn temporary debt into an unbounded bypass mechanism.
- **Keep all scanners, tests, and findings in central files.** Rejected because progressive
  migrations would make the linter and baseline increasingly difficult to review and would create
  merge-conflict hotspots between unrelated architecture rules.
- **Place linter implementation and policy under `.github/`.** Rejected because the checks are
  repository tooling used locally, in pre-commit, and in CI; GitHub Actions is only one caller and
  should not own the code or durable configuration.
