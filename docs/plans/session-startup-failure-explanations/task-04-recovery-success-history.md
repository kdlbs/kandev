---
id: "04-recovery-success-history"
title: "Report confirmed recovery and preserve history"
status: done
wave: 4
depends_on:
  - "03-empty-turn-feedback"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.29
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.32
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.33
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.34
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Task 04: Report confirmed recovery and preserve history

## Summary

Render success from the confirmed selector snapshot of the successful attempt
and resolve only its matching failure. Retain the original native conversation,
saved settings, and permission disclosure through later failure, reload, and replay.

## In scope

- Follow `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.
- Extend `persistProviderRestoredResumeNotice` for safe catalog-label persistence
  and resolved-error linkage. Apply the established provider sanitizer before
  persistence; reuse known-selector fields and current ownership fences. Never
  substitute the requested saved model for an unknown effective model.
- Render success from persisted notice metadata in `StatusMessage`. Keep the
  override-omission and potentially different permission-mode disclosure. A later
  selector change cannot rewrite an older notice's model.
- Persist a bounded, stamp-specific success record for every owned successful
  resume that captured an active failure stamp. The record must survive missing
  or paginated transcript notices, remain separate from manual dismissal, and
  leave successor failures unresolved. Preserve the global resolution timestamp
  only for legacy unstamped rows.
- Adapt `useAgentBootOutcomeAfterMessage` for correlated selection-error history,
  matching the exact validated stamp and host attempt. Preserve unrelated
  filtering and safe legacy rows.
- Preserve chronological error history and original details/time. Resolve only
  the linked prior error stamp; manual dismissal is not recovered success.
  A later failed attempt remains active despite an older dated success notice.
- Render each historical typed cause, phase, safe selectors, attempt/execution,
  and timestamp from that message's own metadata. Keep the row owned by the
  mounted card compact so one occurrence has one full explanation.
- Test stale boot-ready and bootstrap failure callbacks, same-execution newer
  attempts, failed notice-write retries, replay deduplication, and partial reports.
- Extend existing desktop/mobile fixtures without removing native identity,
  saved input, request policy, selector change, unknown value, or prompt trace
  assertions from #4065. Add future ordinary strict-resume regression after a
  successful recovered attempt and a subsequent failed attempt with dated history.
- Translate new success/resolved copy in all required locales; generate pseudo
  and Traditional Chinese catalogs. Check narrow/short panes, expanded diagnostics,
  touch targets, focus return, and draft/attachment survival.
- Update `docs/public/sessions-and-review.md` recovery how-to guidance to match
  delivered behavior. Check `agents-and-profiles.md`, README, and screenshots
  for affected instructions. Update prior companion packages with forward links
  and changed fixture expectations where needed, preserving historical results.
- Record each work order's exact commands, counts, skips/blockers, and outcomes;
  mark tasks done only after their own required checks pass.

## Out of scope

Additional providers, silent fallback, setting mutation, new persistence tables,
rewriting prior test counts, broad QA/review gates, commit, push, PR, or release.

## Acceptance

1. Success has a confirmed model label/ID or an honest unknown message from that
   attempt's durable snapshot; saved settings/native identity are unchanged.
2. Only matching active error controls retire. Dated history distinguishes
   resolution, dismissal, prior success, later failure, and separate attempts.
3. Existing strict and provider-restored flow assertions pass on desktop/phone;
   public guidance and all locale catalogs match the implemented contract.

## ASCII UI preview

UI-03 from the [full preview](plan.md#ascii-ui-preview), criteria .29, .32-.34:

```text
Desktop and native phone Chat:
  <time A> Saved model unavailable [Resolved] > Technical details
  <time B> Session resumed with Gemini 3.7 Flash.
           Your previous conversation was preserved.
           Saved selections kept; restored permission mode may differ.
  [Composer draft and attachments retained] [Send]

Unknown confirmed-model state:
  <time B> Session resumed. Your previous conversation was preserved.

Later failed attempt:
  Keep <time B> as history; new cause owns the current recovery card.
```

Gemini's label requires reliable provider-reported/catalog evidence. Do not
infer a label from its identifier. New status text has no mutation controls;
phone retains native Chat scrolling and the existing composer focus rules.

## Verification

Run from repository root after prior task dependencies. Catalog generation
requires translated en/pt-pt/zh-cn/ja sources; inspect generated output.

```bash
(cd apps/backend && go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle)
(cd apps/backend && golangci-lint run ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/...)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run components/task/chat/messages/status-message.test.tsx components/task/chat/messages/action-message.test.tsx hooks/processed-message-filtering.test.ts lib/session-recovery-presentation.test.ts lib/session-last-agent-error.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx lib/ws/handlers/empty-turn-notice.test.ts lib/ws/handlers/turns.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/messages/status-message.tsx components/task/chat/types.ts components/task/chat/messages/action-message-state.ts hooks/processed-message-filtering.ts lib/session-recovery-presentation.ts e2e/helpers/session-resume-settings-recovery.ts e2e/helpers/session-error-recovery-ui.ts e2e/tests/session/session-resume-settings-recovery.spec.ts e2e/tests/session/mobile-session-resume-settings-recovery.spec.ts e2e/tests/session/session-error-recovery-ui.spec.ts e2e/tests/session/mobile-session-error-recovery-ui.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-resume-settings-recovery.spec.ts tests/session/session-error-recovery-ui.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts tests/session/mobile-session-error-recovery-ui.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/session-startup-failure-explanations
```

Run only the scoped checks justified by final edits; earlier results remain
recorded, but a production change requires its affected regressions to rerun.
Extend lint/test command lists for additional changed consumers. The last task
owns success/history implementation, not a generic verification audit.
Run E2E projects sequentially through the memory-bounded runner with fresh builds.
Record spec discovery/counts and confirm tests are exercised, not skipped.
Use existing `TestExplicitResumeNoticeAttemptOwnership`,
`TestExplicitResumeNoticeKeepsSelectorsUnknownWhenSnapshotBelongsToAnotherAttempt`,
`TestExplicitResumeNoticeRejectsSuccessorAndOrdinaryResume`, and selector source
epoch tests. Add snapshot label and resolved-stamp tests beside them.

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_agent.go`
- `apps/backend/internal/orchestrator/explicit_resume_notice_test.go`
- Existing guarded recovery-settlement tests and selector snapshot tests
- `apps/web/components/task/chat/messages/status-message.tsx`, `.test.tsx`
- `apps/web/components/task/chat/types.ts`
- `apps/web/lib/session-recovery-presentation.ts`, `.test.ts`
- `apps/web/components/task/chat/messages/action-message-state.ts`
- `apps/web/components/task/chat/messages/action-message.tsx`, `.test.tsx`
- `apps/web/hooks/processed-message-filtering.ts`, `.test.ts`
- Existing compact historical error metadata/rendering consumers
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/task.json`
- `apps/web/e2e/helpers/session-resume-settings-recovery.ts`
- `apps/web/e2e/helpers/session-error-recovery-ui.ts`
- Their four desktop/mobile specs under `apps/web/e2e/tests/session/`
- `docs/public/sessions-and-review.md`
- `docs/public/agents-and-profiles.md` if selection guidance needs a cross-link
- Relevant companion plan links and this package's results/status records

## Dependencies

Task 03 completes the shared fixture and empty-turn distinction. Tasks 01 and 02
supply typed persisted evidence, safe presentation, and correlation ownership.

## Risks

Ready may precede an effective-model report; unknown copy is required. A success
snapshot cannot read a newer attempt's selectors. A single global resolution
timestamp or manual dismissal cannot prove every historical failure resolved.
Do not broaden provider-restored eligibility or weaken stale/cancellation fences.

## Parallelism

`sequential`

## Inputs

- Design: Recovery success and history; Desktop and phone composition.
- Explicit-resume-settings requirements/design and existing ADR.
- `explicit_resume_notice_test.go`, `status-message.test.tsx`, and settings-recovery
  trace/identity/unknown-selector fixture assertions.
- Existing recovery public docs and the companion implementation packages.

## Results

Completed. The success notice now renders only the successful attempt's
confirmed model name or ID, or an honest no-model claim. The attempt stores the
active failure stamp at admission and resolves only that captured failure;
frontend history correlates selection failures to the exact success stamp,
keeps dismissed history distinct, and leaves later failures active. Saved
selectors, native ACP identity, and the prior conversation remain unchanged.

Backend evidence:

- `go test ./internal/orchestrator -run 'TestExplicitResumeNotice|TestBeginResumeAttemptCapturesOnlyTheActiveRecoveryFailure|TestAgentBootReady|Test.*RecoveryResolved' -count=1` passed the focused attempt, model-snapshot, replay, and resolution tests.
- `go test -race ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle` passed all three packages: orchestrator 116.833s, executor 4.000s, lifecycle 71.943s. The race run includes the provider-restored admission boundary and boot-ready correlated-resolution tests.
- `golangci-lint run ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/...` reported `0 issues`.

Frontend and catalog evidence:

- `pnpm exec vitest run components/task/chat/messages/status-message.test.tsx components/task/chat/messages/action-message.test.tsx hooks/processed-message-filtering.test.ts lib/session-recovery-presentation.test.ts lib/session-last-agent-error.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx lib/ws/handlers/empty-turn-notice.test.ts lib/ws/handlers/turns.test.ts lib/active-session-recovery.test.ts` passed 9 files and 190 tests.
- `pnpm run typecheck` passed. Changed-file ESLint passed with `--max-warnings 0`; the E2E helper also passed its scoped zero-warning lint after the paragraph assertion was corrected.
- `pnpm run i18n:zh-hant` generated all Traditional Chinese catalogs; `pnpm run i18n:pseudo` generated the pseudo catalog. `pnpm run i18n:check` passed for all six shipped locales with no missing keys or em dashes. `pnpm run i18n:ratchet` reported zero added and 19 modified files with copy violations; the guard allowlist remained intact.

Build and browser evidence:

- The guarded E2E runner built the Go backend, fixture plugin, and pseudo-locale Vite assets successfully.
- Final desktop command, `pnpm e2e:run --project chromium tests/session/session-resume-settings-recovery.spec.ts tests/session/session-error-recovery-ui.spec.ts`, passed 9/9 tests. Its first run passed 8/9 and exposed a test assertion that concatenated separate notice paragraphs; the helper now compares each paragraph, and the clean rerun passed.
- Final mobile command, `pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts tests/session/mobile-session-error-recovery-ui.spec.ts`, passed 9/9 tests. Both projects ran sequentially with one worker and no skips.

Documentation and specification evidence:

- `node --test scripts/validate-public-docs.test.mjs` passed 62 tests; `node scripts/validate-public-docs.mjs` validated 47 published pages.
- `python3 scripts/list-docs.py validate` validated 333 decisions and 1261 specifications. `python3 scripts/lint-spec-files.test.py` passed 36 tests; `python3 scripts/lint-spec-files.py --all` passed.
- `docs/public/sessions-and-review.md` now explains confirmed or unknown success, preserved conversation, unchanged settings, differing permission mode, and Resolved versus Dismissed history. `agents-and-profiles.md` links to that guide. README and existing screenshot references were checked; no further changes were needed.
- `git diff --check` passed. All fixture tests ran; none were skipped.

Review-finding follow-up: every eligible successful resume now persists a
bounded resolution record containing its captured failure stamp, validated
host `resume-<uint64>` attempt ID, and timestamp. The row-locked update is
attempt-owned and idempotent, preserves a successor failure, and is independent
of the best-effort Auggie transcript notice. Frontend history uses that record
after reload, keeps explicit dismissal separate, and requires the exact record
for every stamped failure; a global timestamp or unrelated boot notice cannot
retire it. Tests cover absent transcript rows, manual dismissal, and successor
isolation. Historical failure rows present their own immutable typed evidence,
safe diagnostics, and copy details, while the currently mounted row becomes a
compact marker. Provider-restored model IDs and friendly labels are sanitized
before persistence and again before legacy notice rendering.

Review-fix verification:

- The focused frontend recovery set passed 12 files and 208 tests after adding
  mounted-card, historical-row, legacy-notice, and stamp-specific resolution
  regressions.
- The six-package Go race suite passed for lifecycle, orchestrator, executor,
  task models, SQLite repository, and task DTO. Scoped `golangci-lint` reported
  zero issues across these packages and task status summary.
- `pnpm run typecheck` and full `pnpm run lint` passed. `pnpm run i18n:check`
  validated all six locales, non-JSX copy, translation indices, plural forms,
  and punctuation; `pnpm run i18n:ratchet` reported zero added or modified
  copy violations with the guard allowlist intact.
- Final guarded Chromium E2E passed 9/9 and final mobile-chrome E2E passed 9/9,
  sequentially with one worker. An intermediate strict-resume run timed out;
  the final rerun exercised and passed the same-conversation resume. A stale
  resolved-history E2E fixture was updated to use an authoritative
  `resume-1`-owned stamp record.
- `node --test scripts/validate-public-docs.test.mjs` passed 62 tests and the
  public-doc validator checked 47 pages. The spec catalog validated 333
  decisions and 1261 specifications; all specification lint and its 36 tests
  passed. `git diff --check` was clean.
