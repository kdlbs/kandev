---
id: "19-delivery-coverage"
title: "Complete core settings coverage"
status: done
wave: 19
depends_on:
  - "18-storage-settings"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-006
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-009
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-006.11
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.2
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.4
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.5
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
  - ../../specs/platform/system-design/agent-settings-domains.md
---

# Task 19: Complete core settings coverage

## Summary

Close the required-domain inventory and prove the complete agent configuration workflow.
Publish accurate guidance for the delivered tools, domains, and exceptions.

## In scope

- Reject pending eligible fields, missing domain adapters, unclassified controls, and stale generated snapshots.
- Add negative fixtures for a new domain, route, nested field, removed descriptor, and weakened CI path filter.
- Freeze the five shared tool envelopes. Prove an added setting changes discovery and validation without changing tool definitions.
- Finish configuration-profile alias removal only for equivalent adopted operations. Retain external compatibility and explicit lifecycle tools.
- Complete the cross-domain search corpus and configuration prompt examples.
- Prove action boundaries, authentication exclusions, and explicit lookup from the assembled tool profile.
- Update public tool coverage and domain guidance. Record exact supported/exception/pending counts from the independent inventory.

## Out of scope

- Generic QA, broad repository verification, unrelated cleanup, commits, pushes, or PR creation.
- Relabeling an eligible missing field as an exception to pass the gate.

## Acceptance

- Every eligible field in the required domains has a working adapter and passing evidence. No required entry remains pending.
- Shared tool schemas remain stable and all supported lifecycle calls remain compatible. Exceptions have reasons and recovery destinations.
- The complete desktop/mobile flow succeeds through real MCP, visible UI state, and reload persistence. Public docs match actual coverage.

## Verification

Run from the repository root after the prior work-order checks pass.

```bash
rtk proxy go -C apps/backend test ./internal/settingscatalog ./internal/mcp/server ./internal/mcp/handlers ./internal/mcp/profile ./internal/backendapp
rtk proxy go -C apps/backend run ./cmd/settings-catalog --check
rtk node --test scripts/settings-contract-ci.test.mjs
rtk pnpm --dir apps/web exec vitest run lib/settings-discovery/coverage-inventory.test.ts lib/settings-discovery/delivery-coverage.test.ts
rtk pnpm --dir apps/web e2e:run --project chromium tests/settings/settings-agent-configuration.spec.ts
rtk pnpm --dir apps/web e2e:run --project mobile-chrome tests/settings/mobile-settings-agent-configuration.spec.ts
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.test.py
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

Planned methods in `settings_delivery_test.go` are `TestRequiredSettingsCoverage`, `TestSettingsActionBoundaries`, and `TestStableSettingsToolDefinitions`.

## E2E scenario

Search for task behavior, identify the current user, and update archive confirmation.
Find a disposable workflow and profile, then update a step binding through the compact tool.
Update a repository setting and notification event subscription through their exact targets.
Observe each page and reload it. Reject a wrong-workspace reference without changing state.
Use isolated resources and restore personal settings after failure. No external accounts or real notifications are required.

## Files likely touched

- `apps/backend/internal/mcp/server/settings_delivery_test.go` (new).
- `apps/backend/internal/settingscatalog/search_quality_test.go` and independent coverage tests.
- `apps/backend/internal/mcp/server/server.go` and profile assembly tests.
- `apps/backend/config/prompts/config-context.md`.
- `apps/web/lib/settings-discovery/delivery-coverage.test.ts` (new).
- `apps/web/lib/settings-discovery/coverage-inventory.ts` and generated snapshots.
- The two E2E files named in Verification and their shared helper.
- `scripts/settings-contract-ci.test.mjs` and existing CI workflows if a missing trigger is found.
- `docs/public/automation-and-mcp.md`, `coverage.json`, and the affected domain guides.
- `coverage-inventory.md` and this implementation manifest for final evidence.

## Dependencies

Tasks 00 through 18, with each work order's targeted evidence recorded.

## Risks

- Registry-only checks can pass while a real UI field remains absent.
- Installed provider availability must not change the static completeness denominator.
- Lifecycle tools keep the total MCP surface larger than five tools.

## Parallelism

`sequential`

## Inputs

- Both system designs, all requirement IDs, and the independent field inventory.
- Prior work-order results, MCP profile assembly tests, public-docs coverage conventions, and `.github/AGENTS.md` before CI edits.

## Results

Completed on 2026-09-09.

- Added the independent inventory delivery gate and backend registry completeness test.
- Added deterministic generated snapshots, CI freshness checks, public coverage evidence, and docs.
- Added real MCP desktop and mobile scenarios covering search, description, read, update, and resource lookup.
- Focused implementation gates passed; the full backend-suite caveat is recorded in the parent plan.
