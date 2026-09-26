---
created: 2026-09-19
status: completed
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
legacy_specs: []
---

# Implementation Plan: Session error and recovery UI

## Overview

Deliver one coherent recovery hierarchy while preserving existing operation,
scope, history and authorization contracts. Implement safe diagnostics first,
then integrate ownership/action selection and responsive composition. The initial pass is implemented. The September 20 review of FAILED and
WAITING_FOR_INPUT-with-error fixtures exposed duplicate controls and inconsistent
specialized cards. The user approved uniform presentation; work order 03 is the
implemented design amendment and supersedes the placement previews below.

## Confirmed intent and ownership

This is the existing standalone task `e8757499-cfd8-4715-8b04-81fb7c1f3749`.
The user requested a top-level sibling, not a child of the utility-agent task.
Do not create another task, assign a parent, or move the runtime investigation.
Runtime secret repair is excluded and remains in task
`275733ee-671d-40a9-ba11-b63817a87690`.

Agents own the existing session recovery presentation contract; amend its
[requirements](../../specs/agents/requirements/session-recovery-failures.md)
and [design](../../specs/agents/system-design/session-recovery-failures.md), rather than create a second UI specification.
The September 19 additions .11-.16 have initial implementation evidence, but
the expanded fixture review exposed gaps. Criteria .17-.20 define the refinement. Existing criteria .1-.10 remain
compatibility constraints, especially chronological history and bounded details.

## Scope

In scope: bootstrap and post-start session errors, send-failure feedback,
manual/automatic recovery, no-session ensure fallback, dependent workspace panes,
legacy run errors, shared task-error consumers, desktop/phone/tablet composition,
copy/redaction, localization and focus behavior.

Out of scope: secret provisioning, runtime diagnosis, provider reset policy,
new backend recovery operations, new error scope or persistence model, automatic
fresh starts, history deletion and global Alert redesign unrelated to the defect.

## Technical approach

Use the paired design's action-selection table as the implementation authority.
Extend `lib/session-recovery-presentation.ts`, existing launch-error context and
`useSessionRecoveryActions`; keep current operation/revision guards. Introduce
only a domain diagnostics helper/disclosure and a pure action-selection helper
if needed for testability. Do not add a generic application error framework.

`WorkspaceUnavailable`, `EnsureSessionErrorBanner`, `SessionBootstrapRecoveryCard`,
`SessionStoppedBanner`, `ActionMessageDetails`, `SessionRecoveryActionButtons`,
and `RunErrorEntry` must converge on that model. Audit all callers, including
Quick Chat and preview; suppress equivalent send feedback only with explicit
request/owner correlation. Preserve independent errors and specialized authentication, quota and
managed-runtime semantics through one shared card. Archive/Delete move to the
existing task menu. No model-switch operation is added.

Reproduce the Alert grid hypothesis in a focused test. Repair recovery markup
locally when possible; if the primitive itself is wrong, cover icon/titleless and
icon/title-bearing consumers before changing its column contract.

### Related packages

[Contribution recovery](../contribution-resume-recovery/plan.md),
[error scope/history](../error-scope-and-history/plan.md),
[resume cancellation](../resume-cancellation/plan.md), and
[accepted-turn cancellation](../resumed-turn-cancellation/plan.md) are completed
predecessors, not pending dependencies. Their recorded verification is historical
and is not evidence for this package. Preserve their operation/history/cancellation
contracts; use their tests as regression coverage. The older recovery reveal-at-top
layout remains superseded. The [active recovery ownership decision](../../decisions/2026-09-20-active-session-recovery-owner.md)
qualifies the [scope/history decision](../../decisions/2026-09-14-error-scope-and-history.md)
for active controls only; history and task scope remain unchanged.

## ASCII UI preview

These views supersede the initial transcript-owned active-card previews. Copy is
illustrative; layout, owner, action order and phone composition are required.

### UI-04: Desktop, blocked session

Before: transcript Resume + stopped-composer Resume + separate workspace details.

```text
CHAT                                      FILES / TERMINAL
Earlier messages                          Workspace unavailable
12:04 Session couldn't start               [View recovery]
      > Historical details
Later history (normal scroll)

+-------------------------------------+
| ! Session couldn't start            |  <- replaces blocked composer
| Runtime credential is unavailable.  |
| Restore the credential before retry.|
| [Retry] [Restore workspace]         |
| [Start fresh session]               |
| > Technical details                 |
+-------------------------------------+
```

Shared neutral card and warning icon across generic, runtime and quota failures.
One emphasized eligible action at most. Runtime uses Retry setup; quota with no
eligible recovery uses reset/prerequisite guidance without a pretend primary.
Every eligible recovery action is a visible individual button. Secondary buttons
use lower emphasis, not an overflow menu. Archive/Delete remain in the task menu.
History keeps original cause/time/details, without mutation controls. Shared task
errors retain their independent shell owner.

### UI-05: Phone, active recovery and visible actions

```text
+--------------------------------+
| Task                     [...] | <- Archive/Delete stay here
| Chat / Files / Changes         |
| Earlier messages               |
| 12:04 Session interrupted      | <- compact historical entry
| > Historical details          |
| ...normal transcript scroll... |
+--------------------------------+
| ! Session interrupted          | <- composer region, not overlay
| Agent connection was lost.     |
| [       Resume session       ] |
| [ Restore read-only workspace ] |
| [     Start fresh session    ] |
| > Technical details            |
+--------------------------------+
| Existing safe-area/navigation  |
+--------------------------------+

```

Touch targets >=44px; desktop ordinary controls 28px. Expanded diagnostics wrap
inside the recovery region, capped at 50dvh with one region scroller and no nested
pre scroller. Transcript remains an independent sibling scroll region. No document
horizontal overflow, including short phone viewports and 200% zoom. All eligible
actions are directly reachable; confirmation dismissal returns focus to the
initiating button. Explicit View recovery activates Chat and focuses the card.

### UI-06: Shared pending, failure and success

```text
Pending:  [Resuming... disabled] [Restore disabled] [Start fresh disabled]
Failure:  same card, updated short cause + collapsed safe details
Restored workspace: same card, "Workspace available; agent is stopped"
Success:  original composer and saved draft/attachments restored
History:  original failure stays at original position, without mutation buttons
```

Usable WAITING_FOR_INPUT with historical errors keeps the composer. FAILED and
WAITING_FOR_INPUT plus error_message use the shared card. Healthy STARTING still
allows the existing draft/queue behavior. Only tracked recovery pending owns the
pending card; a stale callback never retires a successor failure.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| .13, .15 | `lib/session-error-details.test.ts`: redacts before truncation, safe idempotent copy payload, nested/malformed assignments, URL/path/identifier cases and empty result; `components/task/session-error-details.test.tsx`: disclosure, safe copy success/failure, locale switch |
| .11, .16 | `lib/session-recovery-presentation.test.ts`: correlated owner, distinct equal-text errors, reversed delivery, no identity fallback; existing processed-message and action tests: history and old/new stamp isolation |
| .12, .16 | `lib/session-recovery-actions.test.ts` (new pure presentation tests): every action-table row; hook tests: one request, failed retry, stale result, workspace-only success |
| .14, .15 | Component tests for semantic roles and focus; browser assertions below for actual geometry, native touch surfaces and readable wrapping |

## E2E tests

Add `tests/session/session-error-recovery-ui.spec.ts` (chromium) and
`tests/session/mobile-session-error-recovery-ui.spec.ts` (mobile-chrome).
Use existing launch-failure/recovery fixtures and an isolated mocked backend;
synthetic secrets only. A WS proxy can inject distinct and duplicated responses
without changing the production error protocol. Mock tests establish UI behavior,
not a repair of real Kubernetes/agentctl authentication.

| Scenario/test title | Required assertions | Criteria |
| --- | --- | --- |
| correlated failure has one recovery owner | Send, bootstrap, stopped and workspace reflections yield one actionable owner and announcement; independent workspace error survives; explicit owner navigation works | .11, .15 |
| details wrap and copy safely | Both reported desktop examples with 20 KiB nested data, long token/URL, CJK and multiline content; no synthetic secret in rendered text, attributes or clipboard; same displayed/copied redaction; empty and denied clipboard cases | .13, .14 |
| recovery selects the existing operation | Parameterize action table, inspect exact request method/action, guards and confirmations; no duplicate request on repeated taps; independent failures remain | .12, .16 |
| recovery retains history across races | Failure, retry, workspace-only success, full resume, newer failure, stale callback, reload and reconnect; old entries retain cause/time and no actions; no forced scrolling while reading history | .11, .16 |
| visible actions and focus remain usable | Every eligible button visible without overflow navigation; desktop wrapping, phone stacking, confirmation/focus return, locale switch, long translated labels, safe area and >=44px hit areas; no page overflow | .14, .15, .20 |

Desktop geometry: 1440px and the reported wide layout near 2048px, a narrowed
Dockview chat pane, and phone-boundary probes at 767/768/769px. Check 28px +/-1
fine-pointer controls, content-column bounding boxes and multiword text occupying
more than the icon column; zero overflow alone would miss one-character wrapping.
Use Pixel 5 from the mobile project without per-test device overrides; use a
separate coarse-pointer tablet context to check 44px fallback. Capture collapsed,
expanded and pending screenshots on desktop/phone; inspect hierarchy visually.
Keep causal waits and one-worker-per-shard policy. Run desktop and mobile suites
sequentially via the managed runner, which rebuilds artifacts.

## Work orders

- [x] [Task 01: Safe readable diagnostics](task-01-safe-diagnostics.md)
- [x] [Task 02: Initial recovery ownership and actions](task-02-recovery-hierarchy.md), depends on 01.
- [x] [Task 03: Uniform composer recovery](task-03-uniform-recovery.md), depends on 02. Completed September 20.

## Documentation impact

Work order 03 replaces the More options instructions with directly visible
recovery buttons and documents the composer owner, retained history, quota task-menu
actions and View recovery navigation. The following records the initial implementation:

Updated `docs/public/sessions-and-review.md` for More options, safe copying,
workspace-only recovery, and correlated View recovery navigation. Existing
recovery operation semantics remain documented. No README or public screenshot
replacement was needed.

## Verification results

Work order 03: pending implementation. The four seeded failure fixtures are
reproduction evidence, not passing coverage for the new layout. No production
code or permanent tests were changed during this amendment.

Initial implementation evidence (work orders 01/02; does not prove 03):

- 295 focused tests across 22 files passed: diagnostics/copy and locale switching,
  action selection, repeat-tap admission, typed guards, stale operations, retained
  history, correlated workspace ownership, announcements and message-send callers.
- TypeScript check passed. Full web lint passed; focused lint on subsequent
  changed files passed. All-locale i18n checks and the new-code ratchet passed.
- Traditional Chinese generation used `i18n:zh-hant --namespace task`; a whole-tree
  generation attempt encountered pre-existing residual characters in
  `workflows:openAgentSettings`. The changed task catalogs and full key checks pass.
- Desktop: all six planned browser regressions passed. After localizing legacy
  recovery-action labels, a rebuilt desktop diagnostic smoke test passed again.
- Phone: the final strict run passed all eight scenarios without retries. The
  earlier strict run reported seven passed and one flaky delayed-resume timeout;
  the complete final rerun passed cleanly. The failure and its resolution remain
  recorded rather than weakening the runner's flaky-test gate.
- Catalog validation (291 decisions, 1038 specifications), specification lint,
  and `git diff --check` passed. Design-phase specification-linter tests passed (36).

The UI browser scenario checks 20 KiB input, CJK text, redacted copied/displayed
text equality, a real content width above 150px, transcript-owned scrolling,
28px desktop controls, >=44px phone/coarse-pointer controls, menu/drawer focus,
and no horizontal overflow at 2048, 1440, 767, 768 and 769px. The Pixel 5 context
also checks the drawer at a 1024px touch-tablet viewport. This supplements phone
coverage without overriding the configured device or claiming physical-device QA.

Screenshots inspected: collapsed and expanded desktop/phone, plus the touch-tablet
options drawer. Copies are in `/tmp/session-recovery-ui/` (`desktop-collapsed.png`,
`desktop-details.png`, `mobile-collapsed.png`, `mobile-details.png`,
`tablet-options.png`). Browser originals live under `apps/web/e2e/test-results/`.
Pending behavior is covered by request/guard tests rather than a static pending
screenshot. Real Kubernetes authentication repair is not claimed.

Browser tests were updated to open secondary choices through More options and to
assert that native disclosure content is hidden, rather than absent from the DOM.
The initial runs caught a lost branch's generic summary, which was corrected to
retain its localized specific cause. No assertions about operation effects or
history retention were removed.

## Risks

Missing correlation must leave separate errors visible; do not invent identity
to improve screenshot count. A plain-text-only credential failure may use the
localized unknown-cause fallback until upstream exposes a typed cause. Frontend
redaction must fail closed on unsafe legacy text. Shared Alert changes can affect
unrelated consumers and require focused primitive proof. In-flight predecessor
changes must be reconciled against source before implementation.

### Implementation result (2026-09-19)

Shared bounded redaction/disclosure, recovery action selection, transient shared
request admission, correlated workspace navigation, legacy-summary fallback,
localized copy, and local Alert content-column placement are implemented.
The public session guide describes the new controls. Runtime provisioning is unchanged.

New-announcement and refusal-prerequisite tests failed before their fixes.
The final legacy refusal summary adjustment passed its focused component tests
and lint; recovery operation semantics are unchanged.

Code inspection established that legacy prompt-forwarding errors contain no
shared failure stamp. They remain separate; send feedback is sanitized but not
suppressed by text equality. Task-owned branch recovery keeps its existing
specialized controls. Workspace navigation requires an exact restore-attempt ID
and a visible owner. There is no broader causal inference.

## September 20 execution handoff

Confirmed: composer owns blocked-session recovery; one shared layout; compact
history; task-menu destructive actions; capability-preserving recovery. Verified:
`ActionMessage` specialized renderers and `ChatInputContainer` independently own
UI; `needsRecovery` includes WAITING_FOR_INPUT with error_message. Current quota
metadata only provides archive/delete, so the conservative supported design shows
reset/prerequisite guidance rather than adding model switching. No material
question blocks the work order. Agents remain the durable owner.

Implement work order 03 sequentially with TDD, then rebuild and refresh only the
owned :48540 test instance for review. Preserve its existing tasks and the original
healthy comparison fixture. Never alter :9998. Keep pending/done status and new
verification evidence separate from the historical test counts above.

## September 20 action visibility correction

The user requested all recovery actions as individual buttons. This supersedes
the initial More options menu/sheet design, including the historical test setup
recorded above. One primary button; lower-emphasis secondary buttons; desktop
wrapping and full-width phone stacking. Existing confirmations and capability
checks remain. Work order 03 must remove obsolete overflow interactions from
recovery browser tests and public instructions.


## Work order 03 implementation and final verification (September 20)

Implemented the approved composer recovery card and individual visible action
buttons. Current failures have one owner; transcript entries retain safe details.
Runtime retry, quota reset guidance, bootstrap restoration, profile selection,
branch guards and task-scoped recovery keep their existing semantics. Unresolved
waiting errors can be identified from correlated live projection/history even
before session metadata catches up; successful boot/resolution retires controls.

Final results:

- Focused unit/component suite: **16 files, 171 tests passed**.
- Desktop: **13 passed**, using the work order's five desktop suites and the stable
  rebuilt frontend (`--no-build`).
- Mobile: **13 passed** using the five mobile suites with
  `--grep-invert 'delayed resume cancellation'`; the two delayed-cancellation tests
  had already passed separately in the initial mobile run (pause preserves runtime,
  cancel fences delayed startup). All 15 selected mobile cases therefore passed.
  The later run includes the repaired real failed-resume regression, 200% text,
  draft/attachment/pending/focus preservation, branch recovery and task-menu access.
- Typecheck, zero-warning full lint, localization check/ratchet, spec catalog/lint
  and whitespace checks passed. Existing locale keys were reused.
- The isolated test UI at http://100.105.155.17:48540 serves the rebuilt assets.
  Original comparison and four scenario tasks remain available. Direct Tailscale
  desktop/phone checks found no horizontal overflow or synthetic-credential leaks;
  mobile View recovery successfully activated Chat and focused the card.

Final logs: `/tmp/uniform-unit-final.log`,
`/tmp/uniform-e2e-desktop-complete.log`, `/tmp/uniform-e2e-mobile-final.log`,
`/tmp/uniform-types-final.log`, `/tmp/uniform-lint-final.log`,
`/tmp/uniform-i18n-final.log`, `/tmp/uniform-ratchet-final.log`.
Shutdown only the owned mock instance with
`python3 /tmp/kandev-recovery-test-d1os6mt5/stop.py`. The main instance at :9998
was not changed. Runtime secret provisioning remains outside this UI package.


## Approved color refinement

The user approved adding amber icon/border/tint to the shared card, retaining the
purple primary action and neutral secondary buttons/details. Pending recovery uses
blue; a failed recovery attempt uses red summary text. This is styling within the
existing reviewed layout. The shipped mobile composer recovery card is the exemplar;
its stacked touch controls, scroll ownership and navigation remain unchanged.
Verification: focused component checks and rendered desktop/phone checks in light
and dark themes, including pending state and neutral diagnostic surfaces.


Color refinement completed: seven focused component tests and targeted ESLint
passed; frontend build, spec validation and whitespace checks passed. Rendered
Playwright checks covered all four cases on desktop and phone in light/dark themes,
plus blue pending-state feedback with disabled actions. No horizontal overflow was
found. Captures: `/tmp/kandev-recovery-test-d1os6mt5/color-*.png`.
The existing :48540 mock now serves the updated colors; :9998 was untouched.


## September 21: task-entry color regression

Correction within the approved color contract (006.17): the shared presentation
used `isSessionRecoveryBusy`, which includes initial `checking`, as both the action
lock and the blue/resuming indicator. Separate busy locking from an actual recovery
action. Keep `checking` amber with ordinary labels and disabled actions; use blue
for automatic `resuming` and explicit pending recovery operations.

The new `session-recovery-color.test.tsx` reproduced checking as recovering (red).
It covers checking, settled error, actual resume, manual restore while checking,
and unrelated-session isolation. Planned verification: focused component tests,
targeted lint, build, and initial-entry desktop/phone color observation in the
existing owned mock. No runtime or recovery admission changes.


Task-entry correction verified: 12 focused component tests passed after the
initial failing regression, targeted ESLint and TypeScript checks passed.
Playwright observed only amber throughout the initial status request on desktop
and Pixel 7, then blue after a browser-local actual-recovery state transition.
Specification validation and whitespace checks passed. The four owned mock
fixtures were reset for repeatable inspection; the main :9998 instance was untouched.


## September 21: neutral buttons and immediate action visibility

User requested neutral buttons and removal of the delayed action reveal. All
recovery buttons now use outlines, retaining recommended-action ordering. Choices
from current failure data render with the card while history loads, disabled until
loading and status checking finish. Specialized retry/quota rules remain intact.
Mobile retains the existing stacked full-width 44px controls; desktop retains
its compact wrapping row. Verify initial visibility, disabled dispatch, hydration
transition, specialized causes, and desktop/phone rendered entry in the owned mock.

Verification complete: the two new loading regressions failed before the fix;
all 14 focused component tests now pass. Targeted lint, typecheck, frontend build,
spec validation, and whitespace checks passed. Desktop and Pixel 7 browser
observers confirmed outlined buttons on the first card render, amber throughout
status checking, and blue during recovery. Phone controls meet 44px sizing and
neither viewport overflows horizontally. The owned :48540 mock serves this build.


## September 22: approved warning/action separation

Implement the user-approved concept drawing: amber warning header, neutral body,
solid neutral recommended button and clearly bordered opaque secondary buttons.
Decorative operation icons aid scanning; stable labels and a reserved localized
status row provide checking/recovery feedback without hiding controls. Existing
eligibility and dispatch semantics remain unchanged. Desktop uses the existing
compact wrapping row; phone uses stacked full-width 44px controls and the same
scroll owner. This supersedes the all-outline styling refinement.

Desktop: [amber icon | title + cause]
         [solid Resume] [outlined Fresh] [outlined Restore]
         [reserved progress status]
         [Technical details disclosure]
Phone:   [amber icon | title + cause]
         [solid Resume, full width]
         [outlined Fresh, full width]
         [outlined Restore, full width]
         [reserved progress status]
         [Technical details disclosure]

Verify existing focused recovery tests, lint/typecheck/localization, and all four
seeded cases in desktop/phone light/dark views, including initial and pending states.


Approved redesign completed. Fourteen focused tests pass, including stable
accessible button labels and localized checking/recovery status. Targeted lint,
typecheck, build, localization checks and spec validation pass. Playwright covered
all four seeded failure cases on desktop and Pixel 7 in light/dark themes, plus
pending recovery: no horizontal overflow and phone action targets at least 44px.
Rendered screenshots were inspected against the approved concept. The owned
:48540 mock serves this build; the main :9998 instance was untouched.


## September 24: idle spacing and quota recovery controls

Remove the reserved idle progress height/margin while retaining a mounted live
region. Progress adds space only when needed; button positions remain stable.
Remove the blanket quota-action suppression: honor backend recovery choices and
explicit empty lists; absent metadata falls back to manual Resume session only.
Keep capacity guidance and existing recovery guards. No automatic retry, quota
bypass, or fabricated reset deadline. Verify two quota regressions, supplied
actions/explicit-empty policy, desktop/phone compact idle spacing and visible
quota actions in the isolated mock.


September 24 verification complete: two quota regressions failed before the fix;
all 16 focused tests now pass, including explicit supplied/empty actions and
loading locks. Targeted lint, typecheck, build, specification validation and
whitespace checks pass. Browser checks cover all four cases in desktop/Pixel 7
light/dark modes, zero-height idle progress, visible enabled quota Resume,
nonzero pending status, disabled pending actions, touch targets and no overflow.
Inspected startup and quota captures. Updated owned :48540 mock; :9998 untouched.


## September 24: stable warning color during recovery

User reported the temporary blue warning after clicking recovery. Keep amber
header, border and icon throughout checking and recovery; use the existing
spinner/status and disabled controls for progress. This supersedes blue pending
styling. Verify computed colors before/during recovery on desktop and phone in
both themes, and rebuild the owned mock. Recovery behavior is unchanged.

Stable-amber verification: targeted lint, build, spec validation and whitespace
checks passed. Browser checks compared header background, border and icon colors
before/during recovery on desktop and Pixel 7 in both themes: unchanged. Existing
progress text, disabled actions, compact idle layout and quota Resume remained
intact across all four examples. The owned mock now serves this revision.


## September 24: quieter recommended action

User approved a uniform neutral button fill instead of the contrasting solid
recommended action. Keep recommendation first with a slightly stronger border;
retain icons, dimensions, states and semantics. Verify desktop/phone light/dark
computed fills and border distinction, then update the owned mock.

Verified matching fills/text and a distinct recommended-action border on desktop
and Pixel 7 in both themes. Mobile controls remain at least 44px. Inspected the
rendered dark card. Targeted lint, build, spec and whitespace checks pass; owned
:48540 mock updated. No recovery behavior changes.


## September 24: reuse app button styling

User reports recovery buttons still look unlike the app. Match the existing
workspace-unavailable Retry control: shared Button outline variant and icon gap,
no custom palette, border emphasis or disabled opacity. Retain recommendation
ordering and responsive wrapping/touch sizes. Verify theme styles against an
existing shared outline control and refresh the owned mock.

Shared-style verification complete: browser comparisons against the existing
outline Copy details control matched fill, border, radius, font size/weight and
border width in desktop/Pixel 7 light/dark views. Phone hit targets remain 44px.
Compared settled styles with CSS transitions disabled in the measurement harness.
Targeted lint, build, spec validation and whitespace checks pass; owned mock updated.


## PR preparation

Full focused unit run passed 228 tests. Added a failing insecure-context copy
regression and adopted the repository's shared clipboard helper; all four detail
copy tests then passed. Updated the shared quota E2E assertion to require manual
Resume when action metadata is absent. Full lint, typecheck and localization
checks passed; final desktop/mobile managed E2E verification follows.

PR verification complete: desktop 13/13 and mobile 15/15 managed E2E checks
passed, including delayed cancellation and runtime preservation. Native build,
frontend build, focused unit tests (228 plus the added clipboard regression),
full lint, typecheck and localization checks passed. Final desktop/mobile
screenshots were inspected and compressed for publication outside the PR branch.


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
