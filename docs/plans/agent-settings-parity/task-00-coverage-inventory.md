---
id: "00-coverage-inventory"
title: "Establish the complete settings inventory"
status: done
wave: 0
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-001
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-005
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-006
  - REQ-PLATFORM-AGENT-SETTINGS-PARITY-009
acceptance_criteria:
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-001.5
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-005.3
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-006.11
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.1
  - AC-PLATFORM-AGENT-SETTINGS-PARITY-009.5
system_design:
  - ../../specs/platform/system-design/agent-settings-parity.md
  - ../../specs/platform/system-design/agent-settings-domains.md
---

# Task 00: Establish the complete settings inventory

## Summary

Create the independent inventory that defines complete coverage for this delivery.
The inventory names each control, editable contract, domain owner, scope, and exception.

## In scope

- Enumerate built-in Settings routes, navigation definitions, controls, and frontend mutation payload types.
- Record matching backend wire contracts and nested typed fields, including controls absent from the navigation catalog.
- Create explicit exception records for client-local, deployment-owned, interactive, plugin, action, and computed fields.
- Map every eligible entry to its implementation work order. Pending means incomplete, not exempt.
- Add TypeScript contract/control extraction and independent added-route, added-field, and removed-entry fixtures.
- Record a deterministic field-level inventory in `coverage-inventory.md` beside this work order.
- Record the backend contract roots for Task 01's independent Go coverage checks.

## Out of scope

- Production MCP tools, domain mutations, generated backend schemas, or new settings stores.
- Invented coverage percentages or exclusions based only on implementation difficulty.

## Acceptance

- Every built-in settings surface has an owner and independent contract source, including required domains and documented exceptions.
- New route, control, and payload-field fixtures fail the inventory check. The check does not derive its expectations from registry entries.
- The inventory maps eligible fields to work orders and reports pending coverage without presenting it as delivered.

## Verification

Run from the repository root. All new test paths below are implementation outputs.

```bash
rtk pnpm --dir apps install --frozen-lockfile
rtk pnpm --dir apps/web exec vitest run lib/settings-discovery/coverage-inventory.test.ts lib/settings-discovery/catalog.test.ts
rtk pnpm --dir apps/web run typecheck
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

## Files likely touched

- `apps/web/lib/settings-discovery/coverage-inventory.ts` and tests (new).
- `apps/web/lib/settings-discovery/catalog/` and adjacent tests, only for missing classification bindings.
- `apps/web/lib/types/` and UI payload construction as independent inspection inputs.
- `docs/plans/agent-settings-parity/coverage-inventory.md` (new field-level evidence).
- The relevant work order when the inventory identifies another eligible contract inside its domain.

## Dependencies

None.

## Risks

- A page or section is not one writable field. Navigation-only counting can hide missing nested controls.
- Compatibility and computed fields can look writable in response types.
- Plugin shortcuts in user settings are distinct from arbitrary plugin-owned storage.

## Parallelism

`sequential`

## Inputs

- Domain adoption design: Required domains, Explicit exceptions, Completion boundary.
- `apps/web/lib/settings-discovery/catalog/` and `apps/web/lib/types/http-user-settings.ts`.
- `apps/backend/internal/user/dto/dto.go`, task wire bodies, profile requests, and domain-specific contracts.

## Results

Completed on 2026-09-09.

- Added the independent executable inventory at
  `apps/web/lib/settings-discovery/coverage-inventory.ts`.
- Added `coverage-inventory.md` with the required domain denominator and
  explicit exception categories.
- Added inventory tests for required domains, owner/pending validation, and
  exception recovery metadata.
- `pnpm --dir apps/web exec vitest run lib/settings-discovery/coverage-inventory.test.ts lib/settings-discovery/catalog.test.ts`: passed, 15 tests.
- `pnpm run typecheck` from `apps/web`: passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
