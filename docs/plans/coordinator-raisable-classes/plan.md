---
created: 2026-11-07
status: draft
requirements:
  - REQ-COORDINATOR-AUTOMATIC-001
  - REQ-COORDINATOR-AUTOMATIC-005
  - REQ-COORDINATOR-AUTOMATIC-006
  - REQ-COORDINATOR-AUTOMATIC-007
  - REQ-COORDINATOR-AUTOMATIC-008
system_design:
  - ../../specs/coordinator/system-design/automatic.md
  - ../../specs/coordinator/system-design/automatic-classes.md
legacy_specs: []
---

# Implementation Plan: Raisable coordinator classes

## Overview

Lets a manager raise `resume` and `start_agent` to `automatic` per
coordinator and turns the eligibility gates into bounded per-coordinator
settings. Decision recorded in the
[phase 3 ADR amendment](../../decisions/2026-09-29-coordinator-phase-3-autonomy.md).
Defaults are unchanged for existing coordinators.

## Work orders

| Order | Result | Depends on |
| --- | --- | --- |
| [01](task-01-eligibility-settings.md) | Settings, generalised eligibility, reviews, routes, raise gate | none |
| [02](task-02-automatic-path.md) | Automatic path for resume and start_agent, admission, lowering triggers | 01 |
| [03](task-03-settings-ui.md) | Settings UI, labels, copy in seven locales | 01 |

## Verification

`make -C apps/backend test` limited to `./internal/coordinator/... ./internal/mcp/...`;
`cd apps/web && pnpm run i18n:check`; E2E decision: the coordinator settings
page changes, so one Playwright spec covers the eligibility group.

## Risks

- `start_agent` evidence joins activity rows to proposals; check the index.
- Containment fails on tank until an isolated executor and auth exist; the
  feature is held, not broken, there.
