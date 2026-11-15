---
id: "01-period-relative-reset"
title: "Preserve a relative reset on an OpenCode period limit"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.13
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Preserve a relative reset on an OpenCode period limit

## Scope

- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`: derive a
  missing hint only after the existing OpenCode period-limit classification.
- `apps/backend/internal/agent/runtime/routingerr/opencode_relative_reset.go`:
  parse a bounded, fully understood duration on the sole matching line using
  the supplied diagnostic observation time.
- Keep structured and dated hints ahead of relative timing. Unknown timing,
  malformed units, unrelated lines, multiple period lines, and overflow do not
  create a reset. No candidate order, circuit policy, or replay safety changes.

## Verification

One-off overlay checks exercise the actual `Classify` entry point with a fixed
observation time and existing OpenCode period controls. They cover explicit and
dated precedence, missing time, unrelated lines/providers, malformed or
conflicting durations, and overflow. No permanent test file is introduced.
The source behavior failed the fixed-time check before the change; related
validation results are recorded in the accompanying PR.
