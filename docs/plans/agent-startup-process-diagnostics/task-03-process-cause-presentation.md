---
id: "03-process-cause-presentation"
title: "Explain process failures on desktop and phone"
status: completed
wave: 3
depends_on:
  - "02-process-cause-propagation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.2
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.13
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.24
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.25
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.28
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.29
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.34
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.36
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-007.5
system_design:
  - ../../specs/agents/system-design/startup-process-diagnostics.md
---

# Task 03: Explain process failures on desktop and phone

## Summary

Show the confirmed startup exit and recognized mise prerequisite through the
existing recovery card. Prove the complete subprocess-to-browser flow on desktop
and phone, including safe copied details and failure history after reload.

## In scope

- Add optional exit status to the frontend cause type and strict parser.
- Extend the shared bootstrap recovery model and mounted card adapters with localized process-exit copy.
- Use the existing detail/copy primitive and same-attempt precedence rules.
- Keep earlier attempts in history without attaching their causes to current failures.
- Add seven shipped locale translations and generated Traditional Chinese/pseudo values.
- Add real-launch desktop and phone E2E fixtures and update the public recovery guide.

## Out of scope

New recovery actions, a trust button, command execution, generic error-string
classification, layout redesign, new warning owners, and checkpoint recovery.

## Acceptance

1. Known mise refusal shows a localized cause and execution-host prerequisite.
   Unknown exits show the observed status without invented toolchain attribution.
2. Displayed and copied details use the same bounded safe fields. Reload preserves
   the cause, status, occurrence time, and distinct attempt history.
3. Desktop and phone use the existing action eligibility, pending state, drafts,
   confirmations, and scroll ownership. The real launch fixture proves propagation.

## ASCII UI preview

These excerpts match [UI-01 and UI-02 in the plan](plan.md#ascii-ui-preview).
They cover 006.2, .13, .24-.26, .28-.29, .34, .36 and 007.5.

UI-01, task Chat, known mise refusal:

```text
Desktop
Agent startup blocked by mise
mise did not trust this workspace's configuration.
Review configuration on the execution host first.
[Resume session] [Start fresh session] [Restore workspace]
> Technical details

Phone
Agent startup blocked by mise
mise did not trust this
workspace's configuration.
Review configuration on
the execution host first.
[ Resume session        ]
[ Start fresh session   ]
[ Restore workspace     ]
> Technical details
```

UI-02, expanded details, shared content order:

```text
Technical details               [Copy details]
Operation: Resume attempt
Phase: bootstrap
Cause: process_exited
Reason: mise_config_untrusted
Exit code: 1
```

The labels are illustrative and all copy uses translation keys.
The current `SessionRecoveryCard` and `RecoveryActions` are the mobile exemplars.
Only eligible actions appear. Phone controls retain at least 44-pixel hit areas.
The inline recovery region owns expanded detail scrolling, with existing safe
areas and viewport bounds. The transcript keeps its own sibling scroll owner.
No hover-only information or nested diagnostic scroller is necessary.

## TDD and rendered evidence

Before production edits, add failing parser/model/component assertions for the
new cause. Include exit zero, absent and malformed status, unknown reason,
hostile detail, same-attempt generic wrappers, and a later distinct failure.
Verify that both mounted and historical adapters use normalized cause evidence.

Add `e2e/helpers/agent-startup-process-diagnostics.ts` using the private CLI pattern
in `copilot-plugin-startup-failure.ts`. The helper starts a real saved session,
then makes its private subprocess emit the reported refusal during resume.
Use a file-controlled outcome or equivalent local fixture for deterministic failure.
The helper must also support initial-start failure and a distinct later error.
Restore backend environment and stop only owned fixture resources in cleanup.

Add the desktop and `mobile-` specs named in the plan. Assert one active card,
specific cause, prerequisite guidance, safe exit status, collapsed details,
matching clipboard text, reload, and original dated history after a later failure.
Use touch activation for phone interactions. Assert no horizontal document overflow
and action targets of at least 44 pixels. Compare rendered structure with UI-01/UI-02.

The fixture must drive the actual backend start/initialize pipeline.
Seeded error metadata alone cannot prove the required propagation.
No real mise installation, provider credentials, npm registry, or external network is required.

## Verification

Run from the repository root. Install dependencies once before the first pnpm
command if the worktree has no install:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/session-last-agent-error.test.ts lib/session-error-details.test.ts lib/session-recovery-presentation.test.ts components/task/chat/session-bootstrap-recovery-model.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/session-recovery-card.test.tsx hooks/domains/session/use-session-recovery-actions.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-startup-process-diagnostics.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-agent-startup-process-diagnostics.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Generate locale variants before the final unit/rendered runs if those runs
depend on updated catalogs. Managed E2E commands rebuild the product and clean
up their isolated instance. Run desktop and phone sequentially with their owning projects.
Record discovery counts and actual results. Add any new helper test file to
the explicit Vitest command rather than leaving it unexecuted.

## Files likely touched

- `apps/web/lib/types/task-status-summary.ts`.
- `apps/web/lib/session-last-agent-error.ts` and `session-last-agent-error.test.ts`.
- `apps/web/lib/session-error-details.ts` and `session-error-details.test.ts` only if the existing field primitive needs an additive status field.
- `apps/web/lib/session-recovery-presentation.ts` and its tests only for same-attempt cause preservation.
- `apps/web/components/task/chat/session-bootstrap-recovery-model.ts` and its tests.
- `apps/web/components/task/chat/session-recovery-model.ts`, card adapters, and current component tests.
- `apps/web/components/task/chat/messages/action-message-recovery-history.tsx` only where historical typed copies need the new evidence.
- `apps/web/hooks/domains/session/use-session-resumption-operations.ts` and matching recovery tests only where request-local typed causes need preservation.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/task.json`.
- New `apps/web/e2e/helpers/agent-startup-process-diagnostics.ts`.
- New desktop and phone specs named in the verification commands.
- `docs/public/sessions-and-review.md` and this package's recorded results.

## Dependencies

[Task 02](task-02-process-cause-propagation.md) provides normalized causes and owned recovery response details.

## Risks

A generic manual or automatic notice can hide a specific same-attempt cause.
An unconditional preference for an old specific cause can hide a distinct current
failure. Use existing identity/stamp proof and preserve chronology.
Never interpolate a raw diagnostic or its path into any visible or copied field.

## Parallelism

`sequential`

## Inputs

- [Design: recovery presentation](../../specs/agents/system-design/startup-process-diagnostics.md#recovery-presentation).
- [Recovery requirements](../../specs/agents/requirements/session-recovery-failures.md).
- Current `SessionRecoveryCard`, `RecoveryActions`, bootstrap model, and
  `copilot-plugin-startup-failure.ts` fixture.
- Read `/tdd`, `/mobile-parity`, `/e2e`, `/docs-maintainer`, scoped web guidance,
  and relevant E2E fixture/cleanup references before implementation.

## Results

Implemented strict cause parsing, localized mise and unknown-exit recovery copy, prerequisite guidance, exit-code details, same-attempt recovery response causes, and all seven locale catalogs plus pseudo. Added the real-launch desktop and phone fixtures and specs, then updated `docs/public/sessions-and-review.md`.

Verification passed:

- Focused Vitest: 147 tests across 10 files, including the new parser, model, presentation, and service regressions.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, `pnpm run i18n:ratchet`.
- `pnpm e2e:run --host --project chromium tests/session/agent-startup-process-diagnostics.spec.ts`.
- `pnpm e2e:run --host --project mobile-chrome tests/session/mobile-agent-startup-process-diagnostics.spec.ts`.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.test.py`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`.

The browser flow covers real start failure, reload, safe copied details, and a distinct later fresh-start failure. Resume readiness cause preservation is covered by the backend regression from work order 02.

Review remediation: managed-runtime retry failures with a final process-exit cause now render the process title, summary, prerequisite, and exit status, while explicit branch, guard, and restore refusals keep their own explanation and suppress the earlier process prerequisite. Added mounted-card regressions for both managed-runtime exit cases and the branch, guard, and manual later-refusal cases. Focused Vitest grew to 152 passing tests; both Playwright specs passed again.
