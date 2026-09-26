---
id: "03-uniform-recovery"
title: "Uniform composer recovery"
status: done
wave: 3
depends_on: ["02-recovery-hierarchy"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.11
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.12
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.13
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.14
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.15
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.16
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.17
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.18
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.19
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.20
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
---

# Task 03: Uniform composer recovery

## Summary

Move current blocked-session recovery into one composer-region card and use a
single visual structure for generic, bootstrap, managed-runtime and quota causes.
Keep compact chronological history, safe diagnostics and existing operation guards.

## Evidence and root cause

Four isolated fixtures at :48540, recorded in
`/tmp/kandev-recovery-test-d1os6mt5/more-cases.json`, reproduce FAILED startup,
WAITING_FOR_INPUT with error_message, FAILED runtime installation and FAILED quota.
Desktop/phone screenshots `case-<0..3>-<desktop|mobile>.png` show their divergent
presentation. The startup case has transcript Resume and composer Resume.

`ActionMessage` dispatches to generic, managed-runtime and quota renderers with
separate markup. `ChatInputContainer` independently returns `SessionStoppedBanner`.
`useSessionState` derives `needsRecovery` from WAITING_FOR_INPUT plus error_message,
but the input remains a disabled composer. The original waiting-without-error
fixture missed these paths. This is a composition/ownership gap, not proof of a
runtime secret provisioning defect.

## Scope and files

Own the existing package refinement, sequentially in the primary session.

- `lib/session-recovery-presentation.ts` and tests: active owner/model derivation,
  current identity, history-only matching, cause adapter inputs and no-action states.
- Proposed `components/task/chat/session-recovery-card.tsx` and tests: common amber
  layout using `components/task/recovery-actions.tsx` and `session-error-details.tsx`.
- `components/task/chat/chat-input-area.tsx`, `chat-input-container.tsx`,
  `session-stopped-banner.tsx`, `session-bootstrap-recovery-card.tsx` and tests:
  owner integration, draft preservation and disappearance/focus behavior.
- `components/task/chat/messages/action-message.tsx`, `action-message-recovery.tsx`,
  `action-message-details.tsx` and tests: history-only specialization with no duplicate
  mutation controls; preserve standalone owner fallback.
- `components/task/task-launch-error-context.tsx`, `workspace-unavailable.tsx`,
  `ensure-session-error.tsx`, `passthrough-chat-composer.tsx`, and
  `simple/components/run-error-entry.tsx` with relevant tests: owner/navigation adapters.
- Existing `use-session-recovery-actions.ts` and `session-recovery-pending.ts` only
  as needed to share the same attempt, pending state and existing confirmations.
- Existing task menu/mobile navigation adapters as needed to retain quota task
  Archive/Delete; locate their current owners before changes, reuse their handlers.
- `src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/{task,chat}.json`, existing
  recovery E2E helper/suites, `docs/public/sessions-and-review.md`, and package status.

No backend recovery API, schema, real provider access, runtime-secret repair,
quota-policy change or new Change model command. Preserve task-scoped shell cards.
No delegation, commit of unrelated files, or mutation of the main :9998 instance.

## Acceptance and regression matrix

Write the failing composer/transcript integration regression before implementation.
The old build must fail on two active Resume controls for the seeded FAILED case.
Do not mock away either real owner or declare success from card-only unit tests.

| Scenario | Required outcome | Evidence |
| --- | --- | --- |
| FAILED startup + matching transcript stamp | One composer owner, zero transcript mutation controls; safe cause/details | Component integration and desktop/mobile |
| WAITING_FOR_INPUT + error_message | Shared card replaces blocked editor | Component and desktop/mobile |
| Healthy waiting with old error | Usable composer; historical row cannot re-block input | Model and component |
| Runtime installation | Same structure; exact runtime_retry payload, pending, failure update | Component and desktop/mobile WS assertions |
| Provider quota | Same structure; provider/model/reset guidance; backend recovery choices or manual Resume fallback when metadata is absent; no unsupported Change model; task menu still archives/deletes with confirmations | Component and desktop/mobile |
| Missing profile / branch / recovery guard | Original prerequisites, eligible operations and confirmations preserved | Existing hook/guard suites plus component |
| Retry pending / failed / succeeded | One admitted request, same card updated, matching resolution restores draft and attachments | Hook/component and desktop/mobile |
| Read-only workspace success | Workspace available; composer still blocked; no implied resumed agent | Component and browser |
| Correlated workspace pane | View recovery activates Chat and focuses owner, no repeated details/actions | Component and desktop/mobile |
| Unrelated workspace/task/session error | Correct independent scope survives; no text-based suppression | Model and existing scope suites |
| Pagination, reload, reordered events, newer failure | Current owner independent of loaded history; old rows persist without actions; stale callbacks harmless | Model and browser |
| Layout and accessibility | 28px desktop / >=44px touch, no overflow, details/copy safe, single announcement, conditional focus return | Component and browser |

## ASCII UI preview

Detailed UI-04/05/06 are in [plan.md](plan.md#ascii-ui-preview).

```text
Desktop                          Phone
History: cause/time/details      History: cause/time/details
(no mutation controls)           (normal transcript scroll)

+-----------------------------+  +----------------------------+
| ! Session interrupted       |  | ! Session interrupted      |
| Agent connection was lost.  |  | Agent connection was lost. |
| [Resume] [Restore workspace]|  | [     Resume session     ] |
| [Start fresh session]       |  | [Restore read-only space] |
| > Technical details         |  | [   Start fresh session  ] |
+-----------------------------+  | > Technical details        |
(replaces blocked composer)      +----------------------------+
                                 (safe-area/nav clearance)
```

Desktop outcome: one predictable recovery location. Phone entry: task Chat via
existing navigation; View recovery from Files selects it. Use the current stopped
composer location and RecoveryActions touch-target sizing as exemplars. Inline
card suits a blocking local decision. All eligible actions are individual buttons;
there is no overflow menu or secondary-action sheet. Shared view model and mutation
guards; desktop wraps the button row and phone stacks full-width buttons. Existing
confirmation surfaces remain for operations that require confirmation.

Compact card is intrinsic height. Expanded recovery region is capped at 50dvh and
has one scroll owner; no nested pre scroll. Transcript remains a sibling scroller.
Test short 320x568 phone geometry, configured Pixel 5, 767/768/769px boundary,
coarse-pointer tablet, narrow desktop pane, 1440px and 2048px, plus 200% text/zoom.
Actions precede details and stay reachable. Retain safe-area and keyboard behavior.

## Implementation sequence

1. Mark this work order in_progress. Add the failing real-owner integration test
   and pure selection cases. Record the expected red result.
2. Add the shared presentation/model; integrate composer and history consumers.
   Keep the old task-scoped owner independent. Reuse handlers and draft stores.
3. Adapt runtime/quota/bootstrap/stopped cases, dependent workspace navigation,
   explicit action focus, and task-menu destructive affordances. Localize copy.
4. Run targeted checks below; inspect screenshots against the previews. Update
   public docs only for implemented behavior. Rebuild the current task UI.
5. Refresh only the owned mock test instance with its existing data; preserve the
   original comparison and four failure fixtures. Provide updated links. Record
   verification results and mark this work order done only when acceptance passes.

## Verification commands

From repository root, run focused unit/component tests first:

```bash
(cd apps/web && pnpm exec vitest run lib/active-session-recovery.test.ts lib/session-recovery-presentation.test.ts lib/session-recovery-actions.test.ts components/task/chat/session-recovery-card.test.tsx components/task/chat/chat-input-container.test.tsx components/task/chat/chat-input-area.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/session-bootstrap-recovery-card.test.tsx components/task/chat/messages/action-message.test.tsx components/task/chat/messages/action-message-recovery.test.tsx components/task/workspace-unavailable.test.tsx components/task/task-launch-error-context.test.tsx hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts components/task/simple/components/run-error-entry.test.tsx components/task/ensure-session-error.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant --namespace task)
(cd apps/web && pnpm run i18n:zh-hant --namespace chat)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
```

Extend `e2e/helpers/session-error-recovery-ui.ts` and both existing wrapper suites
with the matrix above. Update specialized runtime/quota tests to use the shared
owner and task-menu affordances rather than keeping their old layout assertions.
Replace existing More options interactions in recovery tests with direct action
buttons. Assert every eligible action is visible without opening a menu on both
viewports, including long translated labels and the all-actions branch-loss case.
Run desktop and mobile sequentially; the managed runner builds current artifacts:

```bash
(cd apps/web && pnpm e2e:run --host --project chromium tests/session/session-error-recovery-ui.spec.ts tests/session/session-resume-recovery.spec.ts tests/session/managed-runtime-npm-recovery.spec.ts tests/session/provider-quota-recovery.spec.ts tests/task/launch-failure-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/session/mobile-session-error-recovery-ui.spec.ts tests/session/mobile-session-resume-recovery.spec.ts tests/session/mobile-managed-runtime-npm-recovery.spec.ts tests/session/mobile-provider-quota-recovery.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Add directly changed caller suites to these exact commands before completion.
Assert actual command method/action and recovery outcome, not only visible labels.
Check diagnostics and copied text contain no synthetic credentials. Test copy denial
on plain HTTP without exposing raw fallback. Do not use the running manual instance
as a substitute for disposable E2E fixtures. Record screenshots for all four cases,
collapsed/expanded/pending states and desktop/phone, plus focus/draft results.

## Risks

Legacy errors may lack correlation: preserve history and use a conservative owner,
never silently drop a distinct cause. Moving controls can unmount hooks mid-request:
shared pending state and attempt guards must survive. Runtime retry cannot become
resume. Quota must not gain unsupported mutations. Editor unmount can lose local
attachments unless existing draft ownership is preserved. Expanded footer details
must not hide navigation or consume the entire phone viewport.

## Verification results

September 20 implementation verification:

- Focused Vitest command above: 16 files, 171 tests passed. New red cases proved
  unresolved WAITING_FOR_INPUT errors without error_message and live error
  projection arriving before session metadata; both now pass. Resolved history
  remains usable. Component tests cover one owner, runtime/quota eligibility,
  bootstrap causes, loading guards, pending retention and workspace focus routing.
- Desktop and mobile E2E results are recorded in the parent plan. Coverage includes
  direct actions, task-menu Archive/Delete access, recovery method semantics,
  branch-loss guards, failed saved-session resume, history/reload, redacted copying,
  long diagnostics, breakpoint/tablet layouts, 200% text, and draft plus attachment
  preservation through pending recovery with editor focus restoration.
- Typecheck, repo-wide lint with zero warnings, i18n check and ratchet passed. No
  new locale keys were needed: existing translated operation/cause labels are reused.
  Spec catalog validation, specification lint and git diff whitespace checks passed.
- Rebuilt frontend is served by the existing isolated mock instance at
  http://100.105.155.17:48540. Refreshed all four fixtures and preserved the original
  comparison task. Desktop/phone checks found no document overflow, exposed
  synthetic credentials or enabled composer in blocked cases. A direct Tailscale
  phone check confirmed Files → View recovery activates Chat and focuses the card.
  Main :9998 was untouched. Shutdown: `python3 /tmp/kandev-recovery-test-d1os6mt5/stop.py`.

The seed API persists snapshots; live draft transition tests explicitly drive the
existing E2E store bridge. Real recovery operations are verified by the branch,
failed-resume and cancellation suites. Early runs exposed a file-browser prop
error and obsolete menu/fixture assumptions; those were corrected before the final
runs. A known-failing automatic retry was stopped before rebuilding. No runtime
secret provisioning changes are included.


The subsequent approved color refinement adds amber attention styling, blue pending
feedback and red failed-attempt text to this shared layout. Details remain neutral;
primary and secondary button treatments retain their existing hierarchy.


### September 22 approved visual refinement

Follow the approved header/body split in plan.md: amber warning header, neutral
action body, solid neutral recommended action, and opaque bordered secondary
buttons with icons. Keep action labels stable and use a localized live status
row for checking/progress that collapses when idle. Preserve immediate visibility, existing eligibility,
and compact desktop versus full-width 44px phone controls.


September 24 refinement: idle progress has no reserved height/margin. Quota
cards honor explicit recovery choices, including empty lists; absent action
metadata offers manual Resume only, retaining provider/reset guidance.

The warning stays amber during recovery as well as checking/idle. Progress is
communicated through the existing spinner/status and disabled controls only.


## PR integration and review remediation

Merged current main while preserving its launch warnings, late clarification
answers and chat scroll handling. Moved the presentation amendments beneath
requirement 006 so delivery-package coverage resolves every acceptance criterion;
added the recovery strings to the newly introduced Japanese catalog.

Current recovery now survives a fresh STARTING mount, including history loading,
until a matching durable resolution. A context without an active card no longer
suppresses transcript remediation. Explicit recovery navigation selects the failed
session and opens its session-specific panel. Backend action ordering and sanitized
warning text are retained; warnings are readable without hover on phones.
The existing active-card provider remediation link is covered by regression tests.

Verification: 142 focused tests across eight chat/recovery suites passed. The
additional pre-history STARTING regression failed first, then passed with its
28-test selector/card rerun. Type checking, full lint (with focused rerun after
complexity extraction), localization, documentation coverage, specification and
harness validation passed. The 13 desktop managed E2E cases passed, including
provider-link reachability at 320px. All 15 mobile scenarios passed (14 in the suite and the corrected quota navigation
case in its focused rerun). CodeRabbit summary-review corrections add sanitized
guard/restore summaries, stable pending labels, hidden-owner fallback recovery,
a deduplicated branch action and corrected Traditional Chinese copy. Their six
focused suites passed 60 tests, including initially failing regressions. Final
rendered reruns passed six desktop and seven mobile cases on the completed
remediation build. Exact-head GitHub CI/reviews remain externally pending until
the fixup completes.

### CI fixture alignment

Updated legacy browser assertions for the shared recovery card, native details
disclosure, visible missing-profile explanation and standard 28px desktop control.
Archive/Delete remain in task menus as specified; provider-link URL, redaction,
keyboard and 44px mobile coverage are retained. Added the store dependency to
Quick Chat and Office test harnesses without changing production behavior.

All reported deterministic failures reproduced before correction. The affected
unit suites passed 14 tests. Desktop verification passed eight cases plus both
deleted-profile cases in a focused rerun; the mobile provider-link case passed.
The three CI retry-only failures (preview capture, environment reuse and file
drag/drop) passed locally without retries. Exact-head CI remains externally
pending after delivery of these test-only corrections.

### Current-base integration

Integrated the newly shipped npm release-age policy cause into the shared card,
preserving its localized explanation and runtime-only retry semantics during
bootstrap and history loading. The three new policy regressions failed before
the integration; all 55 focused component tests passed afterward. Desktop and
mobile npm recovery each passed two managed E2E cases. Type checking, full lint
(with the duplicate-literal correction rechecked), localization, specification
and catalog checks passed. Desktop/mobile light/dark controls match the shared
outline style.

The four Kubernetes CI failures used the retired transcript recovery selector.
Updated their shared assertion to require one composer recovery card with
controls and causal details. Local Kind verification was attempted twice but
blocked before assertions by image-load timeouts; the second cleanup also hit
a Docker container exit-event error. Remote container CI must verify this test
correction. No runtime or Kubernetes fixture behavior was changed.

### September 25 translation conflict resolution

The preceding PR head completed all 53 GitHub checks with no unresolved review
threads. Integrated current main and preserved both the Japanese recovery keys
and its new workflow-change keys. All task catalogs parse without duplicate
keys; localization, type checking and 55 focused recovery tests pass. Fresh
exact-head CI and review verification remain pending after this merge push.

### Review fallback refinement

A branch-recovery error containing only ANSI/control characters now retains the
localized failure explanation after sanitization. The regression failed before
the fix and its eight-test component suite passed afterward. The optional
context memoization suggestion is deferred: no measured performance regression
was identified, and it is not required for the recovery correctness contract.
Exact-head CI/review validation remains pending after this correction is pushed.

### September 26 review confidence audit

The original low-confidence Greptile summary refers to the initial PR head.
Rechecked its four findings against the existing owner, remediation, navigation,
and action-order fixes. Also audited CodeRabbit aggregate architecture concerns.
Resolved cause-specific data could still own FAILED sessions or waiting sessions
with retained error strings. The selector now honors durable success and matching
boot evidence in those states, retaining ordinary stopped-session recovery and
newer failures. Failure occurrence time takes precedence over transcript insertion.

Three regressions failed before the fix. The six affected suites passed 97 tests;
expanded selector coverage passed all 20 cases, including pre-history resolution,
a newer failure and a foreign-session boot. Managed desktop and mobile recovery
suites each passed seven cases without retries, including the new stale-runtime
case. Targeted lint, type checking, specification/catalog checks and frontend build
passed. Backend authorization, task/session correlation, lifecycle locking and
production diagnostic sanitization were inspected; their contracts are unchanged.
The optional memoization suggestion remains a documented performance deferral.
Fresh external reviewer assessment and exact-head CI are pending after delivery.

### Full-review guard fallback

The subsequent full CodeRabbit review reports low merge/security risk and no
architecture-level concerns. Its remaining stopped-banner empty-sanitization
finding is fixed with the existing localized failure message. A regression using
ANSI-only guard diagnostics failed before the change. After extracting the copy
calculation to stay within the complexity limit, both affected suites passed all
18 tests and targeted zero-warning lint passed. This is a display fallback only;
recovery actions, localization keys and desktop/mobile composition are unchanged.
The preceding browser runs remain applicable to those unchanged contracts.
Fresh exact-head CI/review verification remains externally pending after delivery.

### Guard policy and remaining review concerns

CodeRabbit review 5325683699 prompted operation-specific restore failure copy,
consistent primary-message sanitization, and restoration of the startup guard's
no-Restore policy across bootstrap, composer, stopped and legacy run surfaces.
Related regression coverage also found that a legacy run's remediation could be
hidden by a composer context with no active model; only a real model now owns it.
The system design records the existing startup-guard action restriction.

Eight new regressions reproduced these gaps before correction. The four affected
component suites passed 52 tests. Desktop and native mobile managed recovery
suites each passed eight cases without retries, including an intercepted startup
guard refusal. Targeted lint, typecheck and specification validation passed.
Greptile's latest assessment is 5/5 on a3cfb3e4a; it predates these final fixes.

CI on that head failed the Kubernetes recovery-after-backend-restart case
(run 36236750854, job 108390907369): resume returned an error and the composer
remained absent. Local reproduction and final-head CI/review remain pending;
these local UI results do not waive that failed required check.

### Match the active legacy run failure

CodeRabbit thread 4111254939 found that a composer card for a newer failure could
compact an unrelated retained run-error row. Compaction now requires an active
row with a nonempty stamp matching the composer model. Historical, differently
stamped and unstamped rows retain their category presentation and remediation.
This enforces the existing matching-row requirement; no design change is needed.

Three new cases failed before the correction; the matching-owner case confirms
that duplicate controls stay suppressed. The run-entry and chat-entry suites
passed all 31 tests, with zero-warning targeted lint and type checking. This
predicate-only correction does not change layout or native mobile interactions;
the preceding eight desktop/eight mobile cases remain applicable. Final-head CI
and renewed review verification remain pending after delivery.

### Component-size review cleanup

Greptile's renewed review requested compliance with the component-size guidance.
Bootstrap recovery now separates pure presentation-model construction from its
rendered content and recovery controls. The legacy run presentation is a separate
domain component. The affected component files are under 200 lines; actions,
copy, markup and recovery semantics are unchanged. No requirement or system-design
change is needed for this structural extraction.

All seven focused recovery suites pass (97 tests). Typecheck, targeted ESLint,
architecture, localization and specification checks pass. Prior Kubernetes CI evidence was superseded by a successful run of
the same restart-recovery case with zero retries (run 36239673423, container
shard 2, job 108403952098). Current-head CI and renewed reviews remain pending.
