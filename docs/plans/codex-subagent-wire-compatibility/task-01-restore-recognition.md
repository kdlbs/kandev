---
id: "01-restore-recognition"
title: "Restore Codex structured subagent recognition"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-AGENTS-CODEX-SUBAGENT-WIRE-001
acceptance_criteria:
  - AC-AGENTS-CODEX-SUBAGENT-WIRE-001.1
  - AC-AGENTS-CODEX-SUBAGENT-WIRE-001.2
  - AC-AGENTS-CODEX-SUBAGENT-WIRE-001.3
  - AC-AGENTS-CODEX-SUBAGENT-WIRE-001.4
system_design:
  - ../../specs/agents/system-design/codex-subagent-wire-compatibility.md
---

# Restore Codex Structured Subagent Recognition

## Scope and owned files

Own the Codex dialect parser, ACP tool tracking, update enrichment, capacity
evidence, and `codex_v2_subagent_test.go` under
`apps/backend/internal/agentctl/server/adapter/transport/acp/`, plus the scoped
agentctl guidance and this documentation package. Preserve existing correlation
and lifecycle semantics. No frontend, runtime catalogue, or database changes.

## Acceptance

1. Producer-shaped start and spawn-result fixtures fail before the repair and
   pass afterward, including terminal and sibling routing.
2. The strict shared resolver preserves legacy metadata, ordinary tools, other
   providers, and the existing capacity/background-work boundaries.
3. The targeted checks below pass; limitations are recorded without claiming
   live application installation or full child-transcript support.

## Verification

From `apps/backend`:

```bash
go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run '^TestCodexV2' -count=1
go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -count=1
```

From the repository root:

```bash
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_codex.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_tools.go apps/backend/internal/agentctl/server/adapter/transport/acp/normalize.go apps/backend/internal/agentctl/server/adapter/transport/acp/adapter_capacity_continuation.go apps/backend/internal/agentctl/server/adapter/transport/acp/codex_v2_subagent_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Evidence and results

- Executed the installed 2.2.2 reporters and standard-client renderer with
  sanitized inputs to verify the JSON fixtures and absent legacy metadata.
- RED: activity recognition, spawn enrichment/correlation, sibling routing, and
  capacity-evidence tests failed on the unchanged implementation.
- GREEN: focused regressions and the complete ACP package with `-race` pass.
- Changed-package Go lint (CI-pinned v2.9.0) reports zero issues. Documentation
  catalogue, specification, scoped harness, architecture, and whitespace checks
  pass. Public docs are unchanged: this repairs existing provider presentation
  without new commands, settings, APIs, or terminology.
- No live Kandev browser/session replay was run; evidence is producer-renderer
  replay and adapter package tests, not a deployed application upgrade.
- No dependencies on other work orders. No live installation replacement.
