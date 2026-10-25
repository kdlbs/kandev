---
spec: docs/specs/agents/requirements/dynamic-agent-routing-rollout-blockers.md
created: 2026-08-15
status: completed
---

# Plan: Dynamic Agent Routing Rollout Blockers

This is a repair package for the in-progress
[Dynamic Agent Routing](../dynamic-agent-routing/plan.md) plan. It is limited
to the seven rollout blockers and two review suggestions. Telemetry paths stay
untracked and untouched.

## Work packages

1. [Continuation-safe fallback](task-01-continuation-safe-fallback.md)
   adds effect evidence, fail-closed classification, bounded durable
   continuation delivery, and all-candidate launch fallback.
2. [Atomic route actions](task-02-atomic-route-actions.md) moves Retry and Try
   next handoff ownership into one backend operation.
3. [Utility and shared health](task-03-utility-and-shared-health.md) isolates
   utility route identities, rejects partial results, wires binding health,
   circuit opening, and exclusive probes.
4. [Settings and picker contract](task-04-settings-and-picker-contract.md)
   restores the Dynamic profile list, complete candidate rules, profile kind
   propagation, flag-off filtering, and the duplicate-action guard.

## Verification gate

- Focused Go tests cover each package before production changes are retained.
- Focused web tests cover profile option filtering and dynamic editor state.
- `gofmt`, backend package tests, web typecheck, i18n checks, and the relevant
  settings tests pass before this package is marked complete.
- The original dynamic-routing task files are updated with the repair results.

## Results

Completed on 2026-08-15. The seven rollout blockers and two review
suggestions are implemented. Telemetry-routing files remain separate and were
not included in the implementation.

Verification:

- `go test ./internal/orchestrator ./internal/task/repository/sqlite ./internal/agent/runtime/dynamic ./internal/backendapp ./internal/utility/...` passed: 2695 tests in 13 packages.
- Focused web routing, settings, picker, utility, handoff, quick-chat, and session tests passed: 61 tests in 11 files.
- Backend `make lint` and web `pnpm run lint` passed with zero issues and zero warnings.
- `pnpm run typecheck` passed.
- `pnpm run i18n:check` passed with the repository's existing advisory catalog parity warnings.
- `pnpm run i18n:ratchet` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed: 61 tests.
- `gofmt` and `git diff --check` passed.

## Out of scope

The Office routing handoff, observability migration, dedicated dynamic-routing
Playwright projects, and `dynamic-agent-telemetry-routing` package remain
pending in their original plan.
