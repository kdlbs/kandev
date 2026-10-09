---
id: "01-ownership"
title: "Unify automatic recovery ownership"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.11
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.19
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 01: Unify automatic recovery ownership

## Summary

Remove the legacy automatic-recovery page duplicate when the mounted composer
owns the current failure. Transfer safe automatic diagnostics and pending state
to the common card, including legacy sessions without bootstrap metadata.

## In scope

Page/composer ownership selection, explicit request identity, fallback surfaces,
preview parity, phone clearance and focused regression coverage from the plan.

## Out of scope

Backend/runtime repair, new recovery permissions, data migration and redesign.

## Acceptance

1. The plan's metadata-free and non-bootstrap fixtures render one active card,
   retain both safe operation-labelled causes and equivalent pending guards.
2. Independent/unmatched failures remain visible; stale callbacks cannot replace
   current state. No-session, passthrough and unknown-status paths retain a
   usable authorized owner; historical errors do not disable healthy messaging.
3. Desktop and phone preserve workspace-to-Chat reveal, details and retry;
   no duplicate banner/announcement, horizontal overflow or phantom header gap.

## ASCII UI preview

See [full UI-01 and mobile contract](plan.md#ascii-ui-preview).

### UI-01: Task Chat after automatic recovery fails

Before (screenshot and source): task header, red recovery banner, tabs,
conversation, amber composer recovery card.

After, desktop:

```text
Task header
Session / Plan / Pull request tabs
Conversation history                 Files: Workspace unavailable
                                     [View recovery]
[Session recovery failed]
[Resume session] [Start fresh session]
[Technical details: resume / restore]
```

After, phone:

```text
Task header / session picker
Conversation history
[Session recovery failed]
[Resume session               ]
[Start fresh session          ]
[Technical details            ]
Chat / Files / More
```

One active owner and operation-labelled diagnostics are structural requirements;
copy and spacing are illustrative. Keep existing capability-driven actions. The
card replaces the blocked composer, with its existing bounded recovery-region
scroll owner; history remains independently scrollable. Details have no nested
scroller. Phone actions stack with 44px targets and existing safe-area clearance.
Desktop actions wrap with 28px targets. Pending keeps this owner and disables
equivalent actions; successful agent recovery restores the composer; workspace
restoration alone retains stopped-agent feedback. Independent task errors retain
their shared strip. No-session and passthrough cases retain an explicit fallback.

## Verification

Use TDD: first add the composed regression named in the plan and demonstrate its
expected duplicate-owner failure. Add cases for every acceptance condition before
implementation. Install dependencies with `(cd apps && pnpm install --frozen-lockfile)`
if absent. Run browser suites sequentially through the managed runner.

```bash
(cd apps/web && pnpm exec vitest run lib/active-session-recovery.test.ts lib/session-recovery-presentation.test.ts components/task/task-page-mobile-feedback.test.ts components/task/task-page-recovery-feedback.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/preview-session-tabs.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/session-error-recovery-ui.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-error-recovery-ui.spec.ts)
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-page-inner.tsx`
- `apps/web/components/task/task-page-content-helpers.ts`
- `apps/web/components/task/task-launch-error-context.tsx`
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/session-recovery-context.tsx`
- `apps/web/components/task/preview-session-tabs.tsx`
- `apps/web/lib/session-recovery-presentation.ts`
- `apps/web/lib/active-session-recovery.ts`
- Tests named in Verification, with `task-page-recovery-feedback.test.tsx` new.
- `apps/web/e2e/helpers/session-error-recovery-ui.ts` and its desktop/mobile entry points.

## Dependencies

None. Execute in the primary session after an explicit implementation request.

## Risks

See plan: missing diagnostics, unsafe correlation, missing fallback and mobile
clearance. Do not remove all banners or change shared task-error scope.

## Parallelism

`sequential`

## Inputs

- [Requirement 006](../../specs/agents/requirements/session-recovery-failures.md).
- [System design](../../specs/agents/system-design/session-recovery-failures.md),
  Recovery presentation ownership and Uniform active recovery presentation.
- [Active ownership ADR](../../decisions/2026-09-20-active-session-recovery-owner.md).
- Source trace, fixture and test matrix in `plan.md`.

## Results

Implementation and targeted verification complete. The resumed session found
production changes and tests already present; prior RED output was not available.
The empty presentation test placeholder now exercises real page feedback and the
composer card together, including both sanitized causes, unmatched identity,
pending actions and workspace-only success. No prior RED result is claimed.

- Targeted Vitest: 11 files, 158 tests passed, including all named suites plus
  automatic presentation, resumption, ownership, color and workspace regressions.
- Typecheck and i18n: passed.
- Public docs: updated `docs/public/sessions-and-review.md` (how-to guidance);
  62 validator tests and validation of all 47 published pages passed.
- Full `pnpm run lint`: passed with zero warnings. The final test-only selector
  and formatting adjustments also passed focused ESLint.
- Desktop: `PATH=/usr/local/go/bin:$PATH pnpm e2e:run --host --no-build
  --project chromium tests/session/session-error-recovery-ui.spec.ts`: 9 passed.
- Phone: the same runner with `--project mobile-chrome
  tests/session/mobile-session-error-recovery-ui.spec.ts`: 9 passed.
  Both final runs reused the fresh managed backend/Vite/plugin build from this
  session; no application behavior changed after that build. Each run used one
  worker and no retries. The phone screenshot was inspected against UI-01.
- `git diff --check`, documentation index validation and spec lint: passed.
- Browser fixture correction: bare seeded sessions lack canonical environment
  mapping; use a launched, settled session before injecting automatic failures.
  The focused automatic-recovery scenario passed after that correction, then the
  full desktop and phone suites passed. The test also asserts history remains,
  View recovery focuses Chat, both actions disable during a deferred manual
  retry, and failure stays in the same card.

PR integration validation found that the automatic-notice helper narrowed away
the summary-visibility field added on the current base. Its parameter now derives
from the presentation-copy return type, preserving that contract without changing
runtime behavior. The synthetic merge's typecheck failed before this correction
and passed afterward with a 4 GiB Node heap; its six focused suites passed all
81 tests. The three affected recovery-card suites also passed on the PR branch.
Remote CI and review validation remain pending.

Review remediation keeps preview recovery feedback outside Chat while Plan is
selected and publishes committed request identity through React state, including
idle session/task/archive changes that have no other render trigger. Both issues
were reproduced with failing regression tests before the fixes. The affected
preview and resumption suites pass 76 tests; typecheck and the focused preview
Plan Playwright regression pass. Preview is desktop/tablet-only: phone task cards
navigate directly to the full task page. The existing phone automatic-recovery
scenario covers its native recovery path. No new copy or recovery policy is added.

## User review correction

The earlier Plan fallback was reachable but still used the legacy banner above
the agent tabs. That remediation is superseded: use the shared recovery card at
the lower composer position, with Plan content above it. See the updated preview
in `plan.md`. The corrected browser regression failed on the prior implementation
because Plan had zero shared recovery cards. It now checks placement, separate
causes, manual action guards and return-to-Chat ownership. Targeted validation passed: 57 tests in the two affected Vitest suites, frontend
typecheck, focused ESLint, specification lint and the Plan Playwright regression.
The regression failed before the correction and passed after it. Remote CI and
review checks remain pending for this correction; previous PR checks are historical.
