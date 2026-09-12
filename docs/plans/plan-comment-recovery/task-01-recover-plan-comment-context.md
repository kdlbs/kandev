---
id: "01-recover-plan-comment-context"
title: "Recover plan comment context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
  - REQ-TASKS-PLAN-COMMENTS-004
acceptance_criteria:
  - AC-TASKS-PLAN-COMMENTS-001.3
  - AC-TASKS-PLAN-COMMENTS-001.7
  - AC-TASKS-PLAN-COMMENTS-002.2
  - AC-TASKS-PLAN-COMMENTS-002.6
  - AC-TASKS-PLAN-COMMENTS-003.1
  - AC-TASKS-PLAN-COMMENTS-003.6
  - AC-TASKS-PLAN-COMMENTS-004.1
  - AC-TASKS-PLAN-COMMENTS-004.2
  - AC-TASKS-PLAN-COMMENTS-004.3
  - AC-TASKS-PLAN-COMMENTS-004.4
  - AC-TASKS-PLAN-COMMENTS-004.5
  - AC-TASKS-PLAN-COMMENTS-004.6
  - AC-TASKS-PLAN-COMMENTS-004.7
  - AC-TASKS-PLAN-COMMENTS-004.8
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
---

# Task 01: Recover plan comment context

## Summary

Implement the existing task plan-comment design's recovery refinements as one
working frontend change. Make empty-task Send independent of background
comment reads while automatically recovering and protecting real legacy drafts.

## In scope

- Task-scoped recovery state/coordinator, bounded retries, foreground and
  connection readiness, exact acknowledgement, and identity/lifetime guards.
- Quiet background reads, scoped structured/passthrough Send, selected-comment
  Run, and inline actionable recovery state.
- RED-first unit/component tests, focused desktop/mobile E2E, translated pending
  feedback, and the existing public task guide's recovery paragraph.

## Out of scope

Backend persistence/admission changes, global transport behavior, new settings,
layout redesign, other comment sources' ownership, and publication.

## Acceptance

1. Plain Send succeeds under the no-draft failure scenarios in UI-01; recovery
   errors cannot erase displayed refs, latch an empty-task block, or turn an
   unrelated task's records into local restoration feedback.
2. Actual legacy recovery follows the design's bounded schedule and preserves
   exact records until acknowledgement. Mixed recovered/unrecovered rows still
   guard composer Send; Run of a persisted selected comment remains scoped to
   that comment. Automatic recovery never submits a message.
3. UI-02 appears only for real actionable pending feedback. Structured,
   passthrough, and mobile paths preserve drafts/focus and pass the named unit,
   browser, i18n, specification, and public-doc checks.

## ASCII UI preview

Excerpts from the [full previews](plan.md#ascii-ui-preview):

```text
UI-01 (desktop + phone hierarchy):
[Existing context, when present]
[Your message] [Existing Send control]
No restoration row for empty/discovering/transient-read states.

UI-02 (real unresolved feedback requiring attention):
Desktop: [Saved plan feedback needs attention.] [Retry]
         [Your retained message               ] [Send]
Phone:   [Saved plan feedback needs attention.]
         [Retry: touch target >=44px           ]
         [Your retained message               ]
         [Existing composer actions + Send    ]
```

UI-01 covers `AC-TASKS-PLAN-COMMENTS-004.5`, `.6`, `.8`; UI-02 covers `.1`,
`.3`, `.4`, `.7`, `.8`. Reuse the existing task mobile chat and Plan Drawer
entry points. Inline wrapping, one existing scroll owner, safe-area clearance,
28 px desktop Retry, and a minimum 44 px touch target are required; spacing and
example copy are illustrative. Compare the rendered phone surface to both
states during the focused E2E run.

## TDD sequence

First add these named behavioral regressions against the unfixed source:

- `empty legacy storage does not request migration refresh or block Send`.
- `failed plan discovery does not block plain Send before or after a null plan`.
- `successful empty snapshot recovery clears obsolete migration restrictions`.
- `a transient legacy upload recovers without manual Retry`.
- `Run of a persisted comment ignores unrelated legacy recovery`.

Use the actual React hook/store in `use-plan-comment-migration.test.tsx` for
the first four, real submit hooks for message acceptance, and
`use-run-comment.test.ts` for Run. Assert the expected failure before changing
production code. Replace the existing tests that intentionally equate generic
read failure with migration failure; retain real partial failure, UUID conflict,
missing-plan, exact cleanup, and primary-routing coverage.

Add fake-timer cases for bounded attempts, coalesced resume events, connected
readiness, multiple consumers, and unmount. Keep responses deferred when proving
deduplication and late task/plan results. Include a mixed set containing an
acknowledged row, a failed row, and a non-plan row; a storage read failure after
discovery must not falsely empty the known pending set.

For E2E, install targeted transport interception before navigation and prove the
failure stimulus was reached. Preserve existing scenarios in both plan-comment
specs. If hook RED owns a timing path that cannot be recreated faithfully in the
browser fixture, record why and add its end-to-end outcome after the fix using
a fresh managed build; do not count a missing selector as behavioral RED.

## Files likely touched

Existing production boundaries:

- `apps/web/hooks/domains/comments/use-plan-comment-migration.ts`
- `apps/web/hooks/domains/comments/use-plan-comments.ts`
- `apps/web/hooks/domains/comments/use-run-comment.ts`
- `apps/web/lib/state/slices/comments/persistence.ts`
- `apps/web/lib/state/slices/session/types.ts`
- `apps/web/lib/state/slices/session/session-slice.ts`
- `apps/web/lib/state/app-state-types.ts`
- `apps/web/components/task/plan-comment-migration-notice.tsx`
- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/passthrough-chat-composer.tsx`

Planned focused additions:

- `apps/web/hooks/domains/comments/plan-comment-migration.ts`: coordinator
  extracted from the hook, with `plan-comment-migration.test.ts` for retry and
  exact acknowledgement behavior.
- `apps/web/hooks/domains/comments/plan-comment-loading.ts`: shared ordinary
  reads and bounded retry lifecycle, exercised by `plan-comment-loading.test.ts`
  and `use-plan-comments.test.tsx`.
- `apps/web/lib/plan-comment-recovery.ts`: shared recovery projection and
  Send restriction policy, with `plan-comment-recovery.test.ts`.
- `apps/web/components/task/plan-comment-migration-notice.test.tsx`.
- `apps/web/e2e/tests/session/plan-comment-recovery-helpers.ts`: correlated
  failure controls shared by the desktop and phone specs.

Update the adjacent tests named in Verification, the existing two E2E specs,
and `src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json` when changing
pending-action copy. Generate zh-hk/zh-tw through `pnpm run i18n:zh-hant` and
follow the existing pseudo-catalog workflow. Do not remove historical i18n
guard entries.

Add concise recovery guidance to `docs/public/tasks-and-workflows.md`, a how-to
guide: explain automatic retry, preserved feedback, and the actual Retry action
without exposing migration internals. Keep docs/spec status and package results
accurate. Root/scoped AGENTS guidance needs no change unless implementation
introduces an additional persistent architectural convention.

## Verification

Run from the repository root. The install is required because the previous
diagnostic turn's offline install was interrupted. The new test paths below are
part of this work order; confirm they exist and collect tests before marking
the command complete.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- lib/plan-comment-recovery.test.ts lib/state/slices/comments/persistence.test.ts lib/state/slices/session/task-plan-comment-actions.test.ts hooks/domains/comments/plan-comment-migration.test.ts hooks/domains/comments/plan-comment-loading.test.ts hooks/domains/comments/use-plan-comment-migration.test.tsx hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/use-run-comment.test.ts hooks/domains/comments/use-run-comment-primary-recovery.test.ts components/task/chat/chat-input-area.test.tsx components/task/passthrough-chat-composer.test.ts components/task/plan-comment-migration-notice.test.tsx components/task/task-plan-panel.session-switch.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint lib/plan-comment-recovery.ts lib/state/slices/comments/persistence.ts lib/state/slices/session/types.ts lib/state/slices/session/session-slice.ts lib/state/app-state-types.ts hooks/domains/comments/plan-comment-migration.ts hooks/domains/comments/plan-comment-loading.ts hooks/domains/comments/use-plan-comment-migration.ts hooks/domains/comments/use-plan-comments.ts hooks/domains/comments/use-run-comment.ts components/task/plan-comment-migration-notice.tsx components/task/task-plan-panel.tsx components/task/chat/chat-input-area.tsx components/task/passthrough-chat-composer.tsx)
(cd apps/web && pnpm exec eslint lib/plan-comment-recovery.test.ts lib/state/slices/comments/persistence.test.ts lib/state/slices/session/task-plan-comment-actions.test.ts hooks/domains/comments/plan-comment-migration.test.ts hooks/domains/comments/plan-comment-loading.test.ts hooks/domains/comments/use-plan-comment-migration.test.tsx hooks/domains/comments/use-plan-comments.test.tsx hooks/domains/comments/use-run-comment.test.ts hooks/domains/comments/use-run-comment-primary-recovery.test.ts components/task/chat/chat-input-area.test.tsx components/task/passthrough-chat-composer.test.ts components/task/plan-comment-migration-notice.test.tsx components/task/task-plan-panel.session-switch.test.tsx e2e/tests/session/plan-comment-recovery-helpers.ts e2e/tests/session/task-plan-comments.spec.ts e2e/tests/session/mobile-task-plan-comments.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/task-plan-comments.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts -- --retries=0)
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

The two managed browser commands are sequential and rebuild current assets;
do not overlap them or expand to the full suites. Record every command and
actual result in Results. If implementation changes the file decomposition,
update these commands and the owned-file list together before verification.

## Dependencies

None. The original task-owned persistence/admission package is already merged.

## Risks

Do not mistake unavailable storage for an authoritative empty scan after drafts
are known. Guard plan/task generations and keep one active retry owner across
multiple consumers. Sending before legacy discovery finishes uses only visible
persisted context; retain later-discovered drafts for the next explicit action.
Never apply automatic read/migration retries to message or queue delivery.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/plan-comments.md), especially
  requirement 004 and its recovery compatibility note.
- [System design](../../specs/tasks/system-design/plan-comments.md), especially
  Legacy migration, Failure and recovery, and Responsive behavior.
- [Ownership/admission ADR](../../decisions/2026-09-02-task-owned-plan-comments.md).
- Existing `useForegroundRefresh`, `use-session-turns-hydration.ts` bounded
  recovery, `ws-response-hold.ts` and `ws-drop.ts` correlated frame interception.
- `apps/web/AGENTS.md` plus `/tdd`, `/mobile-parity`, `/e2e`, and
  `/docs-maintainer` during implementation.

## Results

Implemented the identified-draft recovery boundary. Ordinary plan/comment reads
have their own store-scoped loader; migration retains and retries only identified
legacy records, consumes authoritative create snapshots, and removes only exact
acknowledged storage rows. Unavailable storage and task/plan generation changes
cannot silently clear pending feedback. Selected persisted-comment Run remains
independent of unrelated recovery. The public guide and six locale catalogs
describe automatic retry and retained messages.

TDD evidence:

- The initial RED run produced six failures for empty/discovery gating,
  automatic migration retry, and persisted-comment Run.
- Additional RED runs covered disconnected foreground reads, automatic read
  recovery, refused cleanup, quiet notices, stale plan-generation errors, cached
  surface remount, and replacement-plan reads behind an older in-flight request.
- The final command listed above passes all 130 tests across 13 files. Run's
  unrelated-recovery regression now lives beside the primary-recovery tests.
- Browser injection targets task/action and, for partial migration, comment UUID.
  One row is acknowledged while another fails; a colocated diff row survives.
  Both composers' unit paths preserve visible references and reject only actual
  pending feedback. Session-discovery failure is injected at the hook boundary.

Validation:

- Offline install passed with
  `pnpm install --frozen-lockfile --offline --package-import-method=hardlink`.
- Changed-file ESLint: passed with no warnings after helper extraction.
- `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet`: passed. Existing catalog orphan reporting is
  non-fatal; all required locale keys and placeholders remain valid.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed.
- `node scripts/validate-public-docs.mjs`: 46 published pages validated.
- `git diff --check`: passed.
- Final managed Chromium command: four scenarios passed, zero retries (1.8m).
  Inspected the empty and actionable-recovery screenshots.
- Initial managed Pixel 5 command: four scenarios passed, zero retries (1.0m).
  Inspected both screenshots; Retry met 44 px and no overflow was detected.
  The final mobile rebuild after the last read-generation correction was
  terminated with exit 143 before tests began. The final phone rerun uses
  `pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts -- --retries=0`
  against the fresh final desktop build: four scenarios passed with zero
  retries (1.6m). No application source changed after that build, and the
  managed runner enforced artifact freshness.

The first sandboxed browser build could not write the Go cache. The same
isolated managed runner succeeded with escalated build-cache/local-server access.
No user's running instance was mutated. No delegation was used. At the local
implementation handoff, the changes were not yet committed or published.

### PR review follow-up (2026-09-12)

Review regressions now cover unknown-plan lookup and exhausted lookup retries,
already-hydrated task sessions when list discovery fails, hidden/visible wake
coalescing in both coordinators, same-plan hydration invalidating a read, and
locale changes with a retained loader. Identified drafts publish their pending
count even while discovery is in flight. Concurrent body edits retain the
acknowledged row/version and use conditional updates; changed anchors, remote
conflicts, and lost responses cannot silently discard local feedback.

Added the missing waiting-for-plan notice/Retry and attention-projection cases,
strengthened disconnected-read coverage, removed the unused restoration key
from all six catalogs, and generated the Taiwan connection-term correction
through a reviewed key override. Explicit browser setup/recovery timeouts retain
the exact two-successful-uploads assertion, including after recovery settles:
this fixture acknowledges two drafts and must detect redundant acknowledged
uploads. Kept the two small private registries separate: the loader refreshes
localized errors on reuse, while migration retains acknowledgement state. No
shared eviction policy is introduced.

The focused 13-file command in Verification passes 145 tests. Changed-file
ESLint (zero warnings), web typecheck, i18n checks/ratchet, full specification
lint, and the public-doc validator pass. The locale conversion command was
`pnpm run i18n:zh-hant --namespace task`; unrelated generated terminology was
not retained. Its 24 tests pass with
`pnpm test -- scripts/convert-zh-cn-to-zh-hant.test.mjs`. The first attempt used
the wrong test runner; the Vitest subprocess test then hit sandbox `EPERM` and
passed with subprocess access. Temporary diagnostics were removed.

The existing public recovery guidance already covers these corrections. Only
the internal design needs the acknowledged-version reconciliation clarification;
no ownership, backend payload, or admission boundary changed. Exact-head remote
review/check evidence remains a separate delivery gate.

Final local browser verification passed all four Chromium scenarios (1.0m) and
all four Pixel 5 scenarios (48.8s), with zero retries. The fresh desktop command
used `pnpm e2e:run --project chromium tests/session/task-plan-comments.spec.ts -- --retries=0`;
the phone used `pnpm e2e:run --no-build --project mobile-chrome tests/session/mobile-task-plan-comments.spec.ts -- --retries=0`
against those unchanged assets. Both ran with
`GOCACHE=/tmp/pr-3616-go-cache.J336QV` after the shared cache lost a compilation
artifact before tests started. The isolated rebuild succeeded; no cache or
application change was needed outside the task-owned test environment.
