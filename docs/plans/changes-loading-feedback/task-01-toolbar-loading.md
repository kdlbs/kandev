---
id: "01-toolbar-loading"
title: "Consolidate Changes loading feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHANGES-LOADING-001
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
acceptance_criteria:
  - AC-UI-CHANGES-LOADING-001.1
  - AC-UI-CHANGES-LOADING-001.2
  - AC-UI-CHANGES-LOADING-001.3
  - AC-UI-CHANGES-LOADING-001.4
  - AC-UI-CHANGES-LOADING-001.5
  - AC-UI-CHANGES-LOADING-001.6
  - AC-UI-CHANGES-LOADING-001.7
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.25
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.26
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.27
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.28
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.29
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.31
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.35
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.37
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.38
  - AC-PLATFORM-WORKSPACE-GIT-STATUS-001.39
system_design:
  - ../../specs/ui/system-design/changes-loading-feedback.md
  - ../../specs/platform/system-design/changes-refresh-recovery.md
---

# Task 01: Consolidate Changes Loading Feedback

## Summary

Deliver toolbar loading/warning feedback and delayed automatic recovery for failed Git reads.
Remove loading and Git refresh-failure banners while preserving data and context ownership.

## In scope

- Extract and lift the existing detail hook into the shared panel data owner.
- Add a read-only pending snapshot projection and pass the same controller to the timeline.
- Add toolbar status in full, narrow, initial, and phone states.
- Add the localized "Review" tooltip to the eye button in both header compositions.
- Add one shared retry timer with capped backoff, eligibility checks, and lifecycle cleanup.
- Replace summary-panel Retry with automatic recovery and translate warning tooltips in every supported locale.
- Remove successful loading notices, pending file labels, and loading commit descriptors.
- Extend the unit/component and E2E suites named in the plan.

## Out of scope

Mutation feedback, diff viewers, backend changes, and general toolbar redesign.

## Acceptance

1. One status covers passive requests and retains the order and accessibility shown in UI-01 through UI-03.
2. Content has no loading or refresh-failure banners. Automatic recovery preserves files and shared ownership, while inline commit Retry remains available.
3. Targeted tests pass, with desktop/phone screenshots and geometry evidence recorded in Results.

## ASCII UI preview

Excerpt from the [combined preview](plan.md#ascii-ui-preview).

```text
UI-01 Desktop pending
| Diff | Review (eye) | Walkthrough | (spinner)     Branch | Pull | ... |
| Files and commit headers; no passive loading rows                   |

UI-02 Phone/narrow pending
| [route] [eye] (spinner)                  ... |
|              content scroller              |
| existing bottom navigation (phone)          |

UI-02 Initial pending
| (spinner)                               ... |
| no completed empty state yet                |

UI-03 Failed refresh / scheduled retry
| Diff | Review (eye) | Walkthrough | (warning)     Branch | Pull | ... |
| Last available files; no banner or Retry     |

UI-03 Active automatic retry
| Diff | Review (eye) | Walkthrough | (spinner)     Branch | Pull | ... |
| Last available files                        |

UI-03 Phone backoff
| [route] [eye] (warning)                  ... |
```

Keep the toolbar fixed and the spinner passive.
Place the status after the entire left action group, before the toolbar spacer.
Hover and keyboard focus reveal the localized "Loading changes..." tooltip.
Hover and keyboard focus on the eye button reveal "Review".
Phone status does not require hover. Existing phone actions keep their touch targets.
This excerpt maps to AC-UI-CHANGES-LOADING-001.1 through 001.7 and Platform `.28` and `.29`.

## Verification

If workspace dependencies are absent, first run `(cd apps && pnpm install --frozen-lockfile)`.
Use the following block from the repository root.
The managed E2E runner builds and cleans up its isolated runtime.
Run the desktop and phone commands sequentially.

```bash
(cd apps/web && pnpm exec vitest run components/task/changes-inline-commit-state.test.ts components/task/changes-inline-commit-state-owner.test.tsx components/task/changes-panel-header.test.tsx components/task/changes-panel-body-context.test.tsx components/task/changes-panel-file-row.test.tsx components/task/changes-timeline-model.test.ts components/task/changes-panel-timeline-history.test.tsx components/task/changes-timeline-history-row.test.tsx components/task/mobile/mobile-changes-panel.test.tsx components/task/changes-panel-git-status.test.ts components/task/changes-panel-refresh-status.test.tsx hooks/domains/session/git-status-refresh-coordinator.test.ts hooks/domains/session/use-session-git-refresh.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/changes-panel.tsx components/task/changes-panel-data.tsx components/task/changes-panel-header.tsx components/task/changes-panel-header-actions.tsx components/task/changes-panel-body.tsx components/task/changes-panel-file-row.tsx components/task/changes-panel-timeline-content.tsx components/task/changes-panel-timeline-types.ts components/task/changes-inline-commit-state.ts components/task/use-changes-inline-commit-details.ts components/task/changes-timeline-history-model.ts components/task/changes-timeline-history-row.tsx components/task/mobile/mobile-changes-panel.tsx components/task/changes-panel-git-status.ts components/task/changes-panel-refresh-status.tsx hooks/domains/session/git-status-refresh-coordinator.ts hooks/domains/session/use-session-git-refresh.ts e2e/tests/git/changes-panel-refresh-recovery.spec.ts e2e/tests/git/mobile-changes-panel-refresh-recovery.spec.ts e2e/tests/git/git-status-refresh-helpers.ts)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/git/changes-panel-refresh-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-changes-panel-refresh-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Add any extracted status component or hook test to the relevant lint/test command.
After these checks pass, promote both draft designs and the UI requirement.
Mark the combined plan implemented only after Task 02 also passes.
The Platform requirement retains its active status.

## Files likely touched

- `apps/web/components/task/changes-panel.tsx`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-header.tsx`
- `apps/web/components/task/changes-panel-header-actions.tsx`
- `apps/web/components/task/changes-panel-body.tsx`
- `apps/web/components/task/changes-panel-file-row.tsx`
- `apps/web/components/task/changes-panel-timeline-content.tsx`
- `apps/web/components/task/changes-panel-timeline-types.ts`
- `apps/web/components/task/changes-inline-commit-state.ts`
- `apps/web/components/task/use-changes-inline-commit-details.ts` (new hook)
- `apps/web/components/task/changes-timeline-history-model.ts`
- `apps/web/components/task/changes-timeline-history-row.tsx`
- `apps/web/components/task/mobile/mobile-changes-panel.tsx`
- `apps/web/components/task/changes-panel-git-status.ts`
- `apps/web/hooks/domains/session/git-status-refresh-coordinator.ts`
- `apps/web/hooks/domains/session/use-session-git-refresh.ts`
- `apps/web/src/locales/<locale>/task.json` and generated pseudo copy
- `docs/public/git-operations.md` (delivered recovery explanation)
- Unit/component suites and E2E files named in the plan.

## Dependencies

None. This work order precedes Task 02, which also changes history-row logic.

## Risks

Preserve detail-owner lifecycle while moving it.
Remove loading descriptors from virtualization rather than hiding their content.
Keep the status independent of review action availability.
Preserve one retry schedule across sibling consumers and cancel work on loss of eligibility.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/changes-loading-feedback.md)
- [System design](../../specs/ui/system-design/changes-loading-feedback.md)
- [Platform requirements](../../specs/platform/requirements/workspace-git-status.md)
- [Automatic recovery design](../../specs/platform/system-design/changes-refresh-recovery.md)
- [Plan and evidence matrix](plan.md)
- Existing refresh-recovery enrichment gates and mobile recovery assertions.

## Results

Implemented the shared toolbar status, shared inline-commit detail owner,
automatic foreground retry schedule, and removal of passive body/loading rows.
The shared retry schedule uses 5, 10, 20, and capped 30-second delays, and it
clears on recovery or loss of foreground/surface ownership. English and all
supported translations include the new warning copy.

Validation passed: 13 targeted Vitest files (88 tests), web typecheck, scoped
ESLint, Prettier, i18n generation/check/ratchet, spec and public-doc validation,
and `git diff --check`. The desktop refresh-recovery E2E passed 3/3 tests for
initial pending-to-settled refresh with commit history visible, hover and
keyboard tooltips, Review action, held enrichment, and concurrent inline
commit reads. The phone E2E passed 2/2 tests,
including prior-file preservation across a failed refresh, automatic retry, a
held selected diff, concurrent commit reads, full-height diff-sheet geometry,
and zero document overflow. `prCapture` recorded pending and ready states.

The combined plan remains in progress. Task 02 was completed after PR #4145
landed at merge commit `0ec0538aa038f2e8b8617fb4f5be6128f0cedbac`; its
post-merge geometry measurements and validation are recorded in
[Task 02](task-02-section-spacing.md).

Review follow-up: recovery monitoring now runs only when the watched session's
environment binding or scoped Git status/refresh state changes. A shared detail
failure projection drives both automatic recovery and toolbar warnings, while
complete membership and prior rows remain available. Tooltip Escape dismissal
and state reset between loading episodes are covered.

Validation passed for the three affected Vitest suites (30 tests), web
typecheck, zero-warning scoped ESLint, `pnpm run build:vite`, and
`git diff --check`. The full `pnpm test` command ran for more than 35 minutes
without producing its final Vitest summary; its session then closed with exit
code 0, so no full-suite test count is recorded here. Task 02 remains gated on
PR #4145, which was open at head `c5038084dc3d2a3f05294c0797e48905d5fb8953`
at the latest check.
