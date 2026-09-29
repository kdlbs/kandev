---
id: "03-resume-validation"
title: "Complete-cycle validation and documentation"
status: done
wave: 3
depends_on: ["02-idle-policy"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.1
  - AC-EXECUTORS-IDLE-PARKING-001.2
  - AC-EXECUTORS-IDLE-PARKING-001.3
  - AC-EXECUTORS-IDLE-PARKING-001.4
  - AC-EXECUTORS-IDLE-PARKING-001.5
  - AC-EXECUTORS-IDLE-PARKING-001.6
  - AC-EXECUTORS-IDLE-PARKING-001.7
  - AC-EXECUTORS-IDLE-PARKING-001.8
  - AC-EXECUTORS-IDLE-PARKING-001.9
  - AC-EXECUTORS-IDLE-PARKING-001.10
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 03: Complete-cycle validation and documentation

## Summary

Verify workspace policy and complete suspend/resume cycles on desktop and phone.
Document the user-controlled policy and ACP activity visibility limit without provider-specific delivery gates.

## In scope

- E2E settings defaults/save/validation/workspace isolation and live policy changes.
- Actual process suspension, focus-only resume without a prompt, message-triggered resume, and repeated idle expiry.
- Stable conversation/session/workspace identity and exactly-once delivery with duplicate focus/message triggers.
- Negative cases for pending input, active work, hidden tabs, manual stops, workflow ownership, and disabled policy.
- Restart recovery from durable suspension provenance and recoverable restore failure.
- Public documentation, scoped engineering guidance, and exact work-order results.

## Out of scope

New UI beyond Task 02; mandatory macOS/OpenCode smoke; provider-internal quiescence APIs.

## Acceptance

1. Desktop and phone tests exercise actual lifecycle transitions and all required settings/recovery outcomes.
2. Tests prove that multiple providers use the same policy and executor teardown preserves task resources.
3. Documentation states workspace scope, disabled/120-minute defaults, focus/message resume, and repeated idle suspension.

## UI verification

Compare the rendered settings with [UI-01](plan.md#ascii-ui-preview). No additional composition is introduced.
Phone navigation and touch composer must reach the same recovery outcomes without desktop-only actions.

## Verification

Use isolated fixtures, short test timeouts, and the normal single-worker E2E limits.
Do not seed only a stopped row: observe actual instance termination before asserting resume.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-idle-parking.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-idle-parking.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- New `apps/web/e2e/tests/session/session-idle-parking.spec.ts`
- New `apps/web/e2e/tests/session/mobile-session-idle-parking.spec.ts` and shared helpers
- `docs/public/use-kandev.md` and `apps/backend/AGENTS.md`
- `apps/backend/AGENTS.md`, `apps/backend/internal/agentctl/AGENTS.md` as needed
- Plan and work-order verification results

## Dependencies

Task 02.

## Risks

Focus-only resume must not send an empty prompt. A visible but untouched task must remain eligible for another idle timeout.
A provider-specific live smoke is supplementary evidence, not a substitute for deterministic cross-provider tests.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/idle-runtime-parking.md)
- [Design](../../specs/executors/system-design/idle-runtime-parking.md)
- Existing desktop/mobile session-resume tests and workspace settings fixtures
- `/e2e`, `/docs-maintainer`, `/mobile-parity`

## Results

The Chromium flow saved workspace policy, observed an idle ACP runtime stop, resumed the same session on explicit focus without a prompt, and observed a second suspension after a fresh idle interval. The mobile flow sent a user message while the runtime was parked, observed recovery and exactly-once user/agent content delivery, then verified a touch-composer message was also delivered once.

The mobile suspension wait includes the persisted executor, session, and workspace snapshot for actionable failures. It passed after the workspace policy stopped depending on the OS process-descendant probe; Kandev-tracked background work remains protected.

The public workspace settings guide now documents the opt-in policy and defaults. `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py validate`, and `python3 scripts/lint-spec-files.py --all` passed. No macOS-specific provider smoke was required by the clarified requirements.
