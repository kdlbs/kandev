---
spec: docs/specs/office/routing.md
decision: docs/decisions/2026-07-15-office-agent-execution-profile-routing.md
created: 2026-07-15
status: completed
---

# Implementation Plan: Office Execution Profile Routing

## Overview

Preserve the rich Office `agent_profile_id` as the durable role identity while resolving a complete `execution_profile_id` for every Office launch. Make provider-tier mappings reference profiles, persist the concrete selection through sessions and executors, and make cross-provider fallback start a fresh provider-native session in the same task/worktree with an explicit durable-state continuation prompt.

## Contract

- Office identity owns role metadata, hierarchy, instructions, skills, Office permissions, budgets, status, executor preference, task/run ownership, costs, and capabilities.
- Execution profile owns provider CLI/account, model, mode, ACP config options, CLI flags and permission behavior, environment, passthrough, and MCP configuration.
- Tier profile resolution is always active. `enabled=false` selects only the first candidate; `enabled=true` permits health-aware fallback.
- Cross-profile fallback never reuses the previous profile's ACP/session token.
- Non-Office launches retain `execution_profile_id = agent_profile_id` compatibility.

## Data And API

- Make `execution_profile_ids` authoritative in `office_workspace_routing.provider_profiles`; decode legacy `tier_profile_ids` and migrate unambiguous model-only mappings.
- Add concrete execution profile summaries to routing APIs and replace raw tier model/config editing with profile selection.
- Add `execution_profile_id` to `task_sessions`, `executors_running`, `runs`, and `office_run_route_attempts` through replayable SQLite/Postgres migrations.
- Include the execution profile in route previews, run details, route attempts, and diagnostics.

## Runtime

- Carry stable Office ID and concrete execution ID from resolver through scheduler, orchestrator, executor, and lifecycle.
- Resolve process config, profile environment, CLI arguments, config options, passthrough, and MCP from the execution profile.
- Resolve skills, instructions, Office JWT/capabilities, budgets, and ownership from the Office identity.
- On profile change, clean up the previous process, suppress its resume token, retain task/worktree state, and launch with a continuation instruction.

## Migration

- Accept and rewrite legacy `tier_profile_ids` as `execution_profile_ids`.
- Match legacy model-only mappings to an active profile only when provider/model/mode identify exactly one profile.
- Surface missing or ambiguous profile mappings as actionable settings errors.
- Seed absent legacy workspace routing from existing Office configuration so upgrades do not silently change providers.
- Reconcile and supersede the current copy-on-select changes; preserve unrelated restart-name and session-assignee fixes.

## Implementation Waves

Wave 1:
- [x] [task-01-routing-profile-contract](task-01-routing-profile-contract.md)

Wave 2:
- [x] [task-02-resolution-and-routing-persistence](task-02-resolution-and-routing-persistence.md)
- [x] [task-03-dual-id-session-plumbing](task-03-dual-id-session-plumbing.md)

Wave 3:
- [x] [task-04-lifecycle-profile-ownership](task-04-lifecycle-profile-ownership.md)

Wave 4:
- [x] [task-05-cross-provider-continuation](task-05-cross-provider-continuation.md)

Wave 5:
- [x] [task-06-frontend-migration-and-qa](task-06-frontend-migration-and-qa.md)

## Verification

Run formatting before checks:

```bash
make -C apps/backend fmt
make -C apps/backend test
make -C apps/backend lint
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web test
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e --grep "Office routing"
```

Focused tests run within each task before the full commands. Container-backed Codex-to-Claude fallback E2E runs when the existing Office E2E fixture can inject classified provider limits; otherwise a lifecycle integration test must cover the real profile/config/token boundary and the browser E2E covers configuration and telemetry.
