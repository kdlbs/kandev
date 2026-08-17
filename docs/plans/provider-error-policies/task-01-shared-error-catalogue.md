---
id: "01-shared-error-catalogue"
title: "Shared error catalogue"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/provider-error-recovery.md"
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

Pending.
