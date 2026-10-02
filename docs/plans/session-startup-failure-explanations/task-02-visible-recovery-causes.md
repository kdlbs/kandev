---
id: "02-visible-recovery-causes"
title: "Present correlated causes and workspace outcomes"
status: done
wave: 2
depends_on:
  - "01-typed-startup-evidence"
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.21
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.22
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.23
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.24
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.25
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.26
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.27
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.28
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.29
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.34
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
---

# Task 02: Present correlated causes and workspace outcomes

## Summary

Adopt the persisted evidence in frontend parsing and show a specific cause
before technical disclosure. Preserve one recovery owner, correlate repeated
projections, and display workspace outcomes independently from agent readiness.

## In scope

- Read frontend guidance, `/tdd`, `/mobile-parity`, `/e2e`, and `docs/i18n.md`.
  Install fresh-worktree dependencies before any package command.
- Extend `AgentErrorCause` and error parsing, active-error/history projections,
  hydration/store merges. Validate optional fields and retain safe legacy fallback.
- Preserve request/attempt association in manual/automatic recovery state. The
  current manual failure type stores only operation; do not deduplicate by text
  or treat a later generic load failure as the earlier model failure.
- Change `buildRecoveryCardModel`, `useRecoveryPresentation`, and mounted
  `SessionRecoveryCard` plus legacy content adapters to primary cause plus
  separate read-only status. Preserve distinct restore failures and current
  eligibility/confirmation checks. Give fresh-start guidance about saved selections.
- Format safe technical fields once, with labelled validated host references
  (`resume-<uint64>` attempt IDs, UUID execution IDs) and original timestamps.
  Keep prose/reference redaction intact. Render typed history from each row's
  own metadata and make the active occurrence a compact marker beside its card.
  Use the same sanitized content for display and copy, including clipboard
  failure/selectable text and safe accessibility labels.
- Add titles/bodies and missing-value variants in task/chat catalogs. Translate
  en, pt-pt, zh-cn, ja; generate zh-hk, zh-tw, and pseudo. Resolve translations at
  render time, including locale changes. No Unicode em dash.
- Extend existing component and desktop/mobile recovery fixtures for visible
  selection causes, read-only outcome, reload, same/different identities, safe
  copy, long values, keyboard/touch controls, and viewport containment.

## Out of scope

Model selectors, provider routing, saved settings, recovery policy, empty-turn
feedback, success notice wording, public docs publishing, new recovery surfaces.

## Acceptance

1. Known model/mode failures show one readable localized title/body without
   opening details. Unknown and legacy records remain safe and useful.
2. Read-only restoration preserves the cause and controls; duplicated correlated
   projections collapse while different attempts/operations retain their evidence.
3. Desktop and phone fixtures prove safe copied details, accessible actions,
   wrapped content, pending states, and unchanged recovery eligibility.

## ASCII UI preview

UI-01 and UI-02 from the [full preview](plan.md#ascii-ui-preview), criteria
.21, .24-.29, .34:

```text
Desktop: cause title -> specific body -> preserved conversation evidence
         -> workspace outcome -> restored-settings disclosure
         -> wrapping action row -> Technical details / Copy details

Phone (same inline region):
+-------------------------------------+
| Saved model unavailable             |
| Auggie did not list the saved model. |
| Conversation preserved.*            |
| Workspace read-only; agent stopped. |
| Restored settings disclosure        |
| [             Resume              ] |
| [       Start fresh session       ] |
| [       other eligible actions    ] |
| > Technical details                 |
+-------------------------------------+
```

Body above is abbreviated; use the design's exact copy. `*` Availability and
no-prompt claims require their corresponding evidence. Reuse
`SessionRecoveryCard`, `RecoveryActions`, and `mobile/session-mobile-layout.tsx`.
Phone controls measure >=44px, desktop fine-pointer controls remain 28px.
Use the existing bounded recovery-region scroller without nested detail scrolling.

## Verification

From repository root, bootstrap once before the first frontend command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Then run these scoped checks. Regenerate catalogs only after English and real
translations are added; inspect generated diffs for unrelated edits.

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
(cd apps/web && pnpm exec vitest run lib/session-last-agent-error.test.ts lib/session-recovery-presentation.test.ts lib/session-error-details.test.ts lib/types/task-status-summary.test.ts lib/task-status-summary.test.ts components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/session-recovery-card.test.tsx components/task/session-error-details.test.tsx)
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/session-bootstrap-recovery-card.tsx components/task/chat/session-bootstrap-recovery-content.tsx components/task/chat/session-bootstrap-recovery-model.ts components/task/chat/session-recovery-model.ts components/task/chat/session-recovery-card.tsx components/task/session-error-details.tsx lib/session-last-agent-error.ts lib/session-recovery-presentation.ts lib/session-error-details.ts lib/types/task-status-summary.ts hooks/domains/session/use-session-recovery-actions.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/session-resume-settings-recovery.spec.ts tests/session/session-error-recovery-ui.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-session-resume-settings-recovery.spec.ts tests/session/mobile-session-error-recovery-ui.spec.ts)
git diff --check
```

Extend the lint list for any additional production adapters actually changed.
The guarded E2E runner builds before testing and owns isolation/cleanup.
Run desktop and mobile sequentially, without worker overrides or full-suite overlap.
Keep existing request and native-identity assertions in the settings helper.
New unit tests must fail before implementation; record actual results.

## Files likely touched

- `apps/web/lib/types/task-status-summary.ts`, `task-status-summary.test.ts`
- `apps/web/lib/session-last-agent-error.ts`, `.test.ts`
- `apps/web/lib/session-recovery-presentation.ts`, `.test.ts`
- `apps/web/lib/active-session-recovery.ts`
- `apps/web/components/task/chat/session-bootstrap-recovery-card.tsx`, `.test.tsx`
- `apps/web/components/task/chat/session-recovery-card.tsx`, `.test.tsx`
- `apps/web/components/task/chat/session-recovery-model.ts`
- `apps/web/components/task/chat/session-bootstrap-recovery-model.ts`
- `apps/web/components/task/chat/session-bootstrap-recovery-content.tsx`
- `apps/web/components/task/session-error-details.tsx`, `.test.tsx`
- `apps/web/lib/session-error-details.ts`, `.test.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`, `.test.ts`
- Existing recovery history metadata consumers under `components/task/chat/messages/`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,pseudo}/task.json`
- `apps/web/e2e/helpers/session-resume-settings-recovery.ts`
- `apps/web/e2e/helpers/session-error-recovery-ui.ts`
- Their four desktop/mobile specs under `apps/web/e2e/tests/session/`

## Dependencies

Task 01 supplies the stable persisted optional evidence fields.

## Risks

Operation-only local state cannot prove two errors are the same attempt.
Legacy detail sanitization removes arbitrary UUIDs; allow only validated host
references in separate structured fields. A read-only notice must not become
a session-success signal. Long interpolations must wrap and remain safe.

## Parallelism

`sequential`

## Inputs

- Design: Correlation and visible explanations; Technical details;
  Desktop and phone composition; UI-01/UI-02 preview.
- Existing session-error-recovery-ui and explicit-resume-settings packages.
- Existing card/details tests and worker-isolated shared E2E helpers.

## Results

Completed. Catalogs regenerated with `pnpm run i18n:zh-hant` (web zh-tw/zh-hk,
37 namespaces each; 23,000 web messages; zero residual simplified-Chinese
warnings) and `pnpm run i18n:pseudo` (37 namespaces, 11,500 messages). Focused
Vitest coverage passed: 11 files, 130 tests, including parsing, attempt
correlation, recovery cards, safe diagnostics/copy, and recovery-action guards.
`pnpm run typecheck`, `pnpm run i18n:check` (all six catalogs complete, no
missing keys or em dashes), `pnpm run i18n:ratchet`, and scoped ESLint with
`--max-warnings 0` passed. Guarded E2E passed sequentially: Chromium 9/9 and
mobile-chrome 9/9, including the real Auggie saved-model failure, read-only
workspace status, later recovery failure, long diagnostic wrapping, clipboard
parity, desktop viewport containment, and phone touch targets. The first
Chromium run caught an assertion that incorrectly treated a later independent
resume failure as the original selection failure; after correcting the
assertion to reflect distinct ownership, the final desktop and mobile runs
passed. `git diff --check` is included in the final package check.

Task 04 extends the settings-recovery fixture with confirmed success and dated
failure history. Its results are in
[Task 04](task-04-recovery-success-history.md#results).

Review-finding follow-up: the mounted recovery model now carries `bootstrap`
phase through to Technical details. Host resume references validate the real
`resume-<uint64>` format independently from UUID execution IDs. Historical rows
render their own typed cause, timestamp, and safe copy fields; the row owned by
the live card points to the card rather than repeating a generic explanation.
The provider-restored notice accepts only a sanitized catalog label or a safe
confirmed model ID, with unknown-model copy as the last fallback; the legacy
frontend notice parser independently enforces the same safe display contract.
The focused recovery set passed 12 files and 208 tests, including mounted-card
phase/reference copy, malicious-reference redaction, typed historical rows,
active-row deduplication, legacy provider-notice sanitization, and exact-stamp
resolution after reload. Full web typecheck and lint passed; i18n check and
ratchet passed; Chromium and mobile-chrome E2E each passed 9/9 sequentially.
