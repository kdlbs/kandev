---
id: "01-gate-chunk-classification"
title: "Gate per-chunk classification"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-PREFILTER-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.1
  - AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.2
  - AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.3
  - AC-AGENTS-PROVIDER-ERROR-PREFILTER-001.4
system_design:
  - ../../specs/agents/system-design/provider-error-chunk-prefilter.md
---

# Task 01: Gate per-chunk classification

## In scope

- `routingerr.MayClassify` and the catalogue-derived gate tokens.
- The ACP message-chunk diagnostic-candidate call site.

## Out of scope

Other `Classify` call sites and rule changes.

## Acceptance

1. Every catalogue rule has gate tokens and the gate is on.
2. Every package string fixture that `Classify` matches passes `MayClassify`
   for every provider, and every catalogue rule is matched by a fixture.
3. Structured inputs bypass the gate, and ordinary prose is rejected.
4. The adapter marker equals the marker computed from `Classify`.

## Verification

```bash
(cd apps/backend && go test -tags fts5 -count=1 ./internal/agent/runtime/routingerr -run 'MayClassify|PatternGateTokens')
(cd apps/backend && go test -tags fts5 -count=1 ./internal/agentctl/server/adapter/transport/acp -run 'Diagnostic')
```
