---
id: "08-failure-e2e-and-docs"
title: "Add failure-flow E2E coverage and operator documentation"
status: complete
wave: 8
depends_on:
  - "07-desktop-and-phone"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-CURSOR-CLOUD-001
  - REQ-EXECUTORS-CURSOR-CLOUD-002
  - REQ-EXECUTORS-CURSOR-CLOUD-003
  - REQ-EXECUTORS-CURSOR-CLOUD-005
  - REQ-EXECUTORS-CURSOR-CLOUD-006
  - REQ-EXECUTORS-CURSOR-CLOUD-004
acceptance_criteria:
  - AC-EXECUTORS-CURSOR-CLOUD-001.2
  - AC-EXECUTORS-CURSOR-CLOUD-001.3
  - AC-EXECUTORS-CURSOR-CLOUD-002.4
  - AC-EXECUTORS-CURSOR-CLOUD-002.5
  - AC-EXECUTORS-CURSOR-CLOUD-003.2
  - AC-EXECUTORS-CURSOR-CLOUD-003.3
  - AC-EXECUTORS-CURSOR-CLOUD-003.4
  - AC-EXECUTORS-CURSOR-CLOUD-003.5
  - AC-EXECUTORS-CURSOR-CLOUD-005.1
  - AC-EXECUTORS-CURSOR-CLOUD-005.2
  - AC-EXECUTORS-CURSOR-CLOUD-005.3
  - AC-EXECUTORS-CURSOR-CLOUD-005.4
  - AC-EXECUTORS-CURSOR-CLOUD-006.2
  - AC-EXECUTORS-CURSOR-CLOUD-002.6
  - AC-EXECUTORS-CURSOR-CLOUD-002.7
  - AC-EXECUTORS-CURSOR-CLOUD-004.5
system_design:
  - ../../specs/executors/system-design/cursor-cloud.md
---

# Task 08: Add failure-flow E2E coverage and operator documentation

## Summary

Isolated E2E proves restart/reconnect does not duplicate paid work or messages and that remote state controls stop and recovery.
Use TDD for changed logic. Keep results pending until the listed checks pass.

## In scope

- Extend the isolated Cursor mock and Playwright fixtures with backend restart, stream loss, retention expiry, rate limits, expired credentials, and uncertain follow-up submission.
- Prove prompt counts, completion guards, scoped MCP rejection, and feature-disabled behavior through composed backend flows.
- Cover recovery outcomes on desktop and phone, including explicit submission resolution and cancellation-pending controls.
- Update public executor, profile, and remote-access docs as how-to/reference sections. Describe HTTPS callback setup, secret scope, supported limits, local-content rules, provider retention, and rollout.
- Update scoped engineering guidance for managed runtime boundaries. Record a reproducible, opt-in live smoke procedure without storing credentials or enabling shipped defaults.

- Own failure fixture controls and E2E unknown-submission resolution; task 07 owns only its rendered components and unit tests.
- Exercise workflow-generated prompts, frozen model controls, and running/unknown archive cleanup on both dedicated cloud projects.
- Document cursor_cloud_dispatch_total, cursor_cloud_reconnect_total, cursor_cloud_submission_unknown_total, and cursor_cloud_cancel_total in root AGENTS.md Observability. Preserve the CLAUDE.md symlink.

## Out of scope

- Work assigned to later tasks, unrelated refactors, and release promotion.
- Paid cloud execution during automated tests.

## Acceptance

- Isolated E2E proves restart/reconnect does not duplicate paid work or messages and that remote state controls stop and recovery.
- Disabled features and invalid callback grants cannot submit work or bypass question/completion barriers.
- Public docs match the implemented limits. A live account smoke result is recorded before installation rollout; missing credentials leave rollout unvalidated, not falsely passed.

## ASCII UI preview

Use [UI-04 in the full preview](plan.md#ui-04-recovery-and-uncertain-submission).
This task extends rendered failure coverage; it preserves the UI-01 through UI-04 composition from task 07.

```text
Desktop: [Reconnecting / Cancelling / Submission unknown]
         [Open in Cursor] [Resolve submission] [Send disabled]
Phone:   < Task
         Submission unknown
         [Open in Cursor]
         [Resolve submission]
         [Send disabled]
```

Map: AC-EXECUTORS-CURSOR-CLOUD-002.5, -003.2 through -003.4, and -006.2.


## Verification

Run from the repository root. New test paths are implementation outputs, not tests available during this planning turn.

```bash
(cd apps/web && pnpm e2e:run --project cursor-cloud tests/session/cursor-cloud-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project cursor-cloud-mobile tests/session/mobile-cursor-cloud-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

### Evidence mapping

- 001.2, 001.3, 002.4, 002.5, 003.2-003.5: `e2e/tests/session/cursor-cloud-recovery.spec.ts: restart, unknown submission, drain, provider errors`.
- 005.1-005.4, 006.2: `e2e/tests/session/mobile-cursor-cloud-recovery.spec.ts: callback rejection, question barrier, recovery controls`.

## Files likely touched

- `apps/web/e2e/fixtures/`.
- `apps/web/e2e/helpers/`.
- `apps/web/e2e/tests/session/cursor-cloud-recovery.spec.ts (new)`.
- `apps/web/e2e/tests/session/mobile-cursor-cloud-recovery.spec.ts (new)`.
- `apps/backend/internal/cursorcloud/ (test mock support)`.
- `docs/public/executors.md`.
- `docs/public/agents-and-profiles.md`.
- `docs/public/mobile-remote-access.md`.
- `apps/backend/AGENTS.md`.
- `apps/web/AGENTS.md`.
- `docs/plans/cursor-cloud/`.

- `AGENTS.md (root Observability; shared through CLAUDE.md)`.

## Dependencies

07-desktop-and-phone

## Risks

Mocks cannot establish account entitlement or remote callback reachability. Keep the feature off until a real installation completes the documented smoke procedure.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/cursor-cloud.md).
- [System design](../../specs/executors/system-design/cursor-cloud.md).
- [Proposed runtime ADR](../../decisions/2026-09-25-managed-remote-agent-runtime.md).
- Source baseline and code patterns listed in the plan.

## Results

Completed 2026-09-26. Desktop recovery E2E passed 4/4 and phone recovery E2E passed 2/2, including restart recovery, unknown submissions, callback rejection, and question/completion barriers. Public-doc validation passed 62/62 tests and all 47 pages; catalog and specification lint passed. `make build`, `make e2e-plugin-package`, and the changed backend package tests passed. The aggregate serial Go suite was interrupted while running `internal/orchestrator`; `internal/agentctl/server/process/probe` fails in isolation in this environment because process-tree probes report `live` where tests expect `settled`. The feature remains disabled by default. Cursor account entitlement and public callback reachability were not tested, so live rollout remains gated.
