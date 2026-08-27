---
id: "01-shared-error-catalogue"
title: "Shared error catalogue"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/provider-error-recovery.md"
---

# Task 01: Shared error catalogue

- **Acceptance:** Add provider-neutral transient/hard classification,
  catalogue version and provenance, trusted timing metadata, exhaustive code
  coverage, and fixture-driven signatures for supported agent families.
  Unknown, non-provider, stale, and low-confidence evidence remains
  unclassified and cannot authorize recovery.
- **Files likely touched:**
  `apps/backend/internal/agent/runtime/routingerr/{routingerr,rules,provider_neutral_rules,runtime_rules,policy}.go`,
  adapter evidence producers under `apps/backend/internal/agent/agents/**` and
  `apps/backend/internal/agentctl/**`, and `routingerr/**/*_test.go`.
- **Dependencies:** none.
- **Parallelism:** sequential foundation.
- **Inputs:** Provider Error Recovery Evidence, Error classes, and Failure modes;
  ADR-2026-08-17; current routingerr fixtures and adapter evidence contracts.
- **Output contract:** Report the code-to-class table, catalogue versioning,
  timing trust rules, provider fixtures added, redaction evidence, files
  changed, exact commands/results, risks, and synchronized task/plan status.
- **Verification:** `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingerr/... ./internal/agent/agents/... ./internal/agentctl/...`
- **Risks:** Broad regexes can misclassify model-authored text. Structured and
  correlated evidence must outrank bounded text signatures. Sensitive values
  cannot enter fixtures, state, logs, or metrics.

## Results

Completed. Added the provider-neutral transient, hard, and unclassified error
classes, persisted `provider-errors.v1` catalogue provenance, and covered the
initial semantic code catalogue with table-driven tests. Existing provider
fixtures and sanitized diagnostics remain unchanged.

Verification:

- `cd apps/backend && go test ./internal/agent/runtime/routingerr -run TestClassifyAssignsSharedProviderErrorClasses -count=1` passed: 6 tests.
- `cd apps/backend && go test ./internal/agent/runtime/routingerr -run 'Test(ClassForCode|ClassifyAssignsSharedProviderErrorClasses|ClassifyPersistsCatalogueVersion)' -count=1` passed: 8 tests.
- `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingerr/... ./internal/agent/agents/... ./internal/agentctl/...` passed: 2,597 tests in 18 packages.

Security/trust boundary: class assignments are deterministic and unknown codes
remain unclassified. No raw provider credentials or unbounded error text were
added.
