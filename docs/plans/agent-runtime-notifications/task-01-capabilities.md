---
id: "01-capabilities"
title: "Capabilities and sources"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-NOTIFY-001
acceptance_criteria:
  - AC-AGENTS-RUNTIME-NOTIFY-001.1
  - AC-AGENTS-RUNTIME-NOTIFY-001.2
system_design:
  - ../../specs/agents/system-design/runtime-update-notifications.md
---
# Task 01: Capabilities and sources

## Summary and scope

Resolve every registered agent through trusted capability metadata. Extend cached read-only status and bounded release sources, including manual/native/unsupported states and single-flight source deduplication.

## Out of scope

No model discovery, worker delegation, or global developer CLI/login changes. Native unverified activation remains manual.

## Acceptance

- The linked acceptance criteria hold across multiple registered identities.
- Failures preserve authoritative state and expose truthful recovery.
- Exact verification below passes, with results recorded.

## Verification

```bash
(cd apps/backend && go test -race ./internal/agent/agents ./internal/agent/settings/controller ./internal/agent/settings/handlers -count=1)
```

## Files likely touched

apps/backend/internal/agent/agents/; apps/backend/internal/agent/settings/controller/agent_update_status*; settings/dto/; settings/handlers/

## Dependencies and inputs

None. Read the linked requirements/design and nearest source/tests.

## Risks

See the plan for native ownership, source failure, consent/selection races, and overlapping PRs.

## Parallelism

sequential

## Results

Capability coverage, native ownership, vendor metadata validation, single-flight and cancelled slot wait tests passed after behavioral RED failures. Existing agent/controller/handler suites passed with -race (see plan command evidence).

A final red/green boundary test demonstrates that Amp vendor version 1.7.0 cannot be compared with adapter releases 0.2.0/0.3.0. External npm observations must match the published stable catalogue before claiming current or newer status; native OpenCode matching-version coverage remains green.
