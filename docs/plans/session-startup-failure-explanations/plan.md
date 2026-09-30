---
created: 2026-09-30
status: complete
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
system_design:
  - ../../specs/agents/system-design/session-startup-failure-explanations.md
legacy_specs: []
---

# Implementation Plan: Session Startup Failure Explanations

## Overview

Preserve known model and permission-mode failures from the lifecycle decision
through durable errors and user presentation. Deliver specific recovery guidance,
independent workspace status, useful copied details, and truthful recovery history.
Application failures retain the actual model passed to the provider, catalog
labels and restored success notices pass safe-value checks, host attempt IDs use
their production format in technical details, and each owned successful resume
records bounded stamp-specific resolution independent of transcript delivery.
Historical failure rows use their own typed evidence; the active row points to
its one mounted explanation.

The [requirement amendment](../../specs/agents/requirements/session-recovery-failures.md#startup-cause-amendment-september-30)
adds criteria 006.21-.34 to the existing recovery contract. The
[design](../../specs/agents/system-design/session-startup-failure-explanations.md)
owns the additive evidence and presentation rules.

The user explicitly authorized implementation after reviewing this package.
All four work orders are complete with their scoped build, test, lint, i18n,
E2E, public-documentation, and specification checks recorded below. No commits,
pushes, or persistent platform tasks or sessions were created.

## Checkout and evidence

- Checkout: `/root/.kandev/tasks/implement-clearer-se_hhvg8vdz/kandev`.
- Dedicated branch: `feature/plan-clearer-session-73b`.
- Base: `ff917a370ac9127fee6f8d7cee80b6b7b9b6175b`.
- PR #4065 merged commit `fa729f2d7653f4480c107c6a6d50a5452eb3dd0e`
  is an ancestor, verified with `git merge-base --is-ancestor`.
- The package was moved into this existing task checkout at the user's request. Original developer files are not changed.
- Incident evidence is supplied by the user from another computer; reproduction
  must use isolated mock agents. No claim of a local incident reproduction.

The verified cause loss is formatted model/mode errors becoming `unknown` in
`bootstrapFailureFor`. Cause allowlists, blank fresh-start operation, and
frontend field parsing are additional loss boundaries. The UI concatenates
projections and suppresses the cause summary after workspace-only success.
The completed-turn event carried `metadata.lifecycle_only` for synthetic
lifecycle history and `metadata.error_terminated` for a correlated failure, but
the browser helper forwarded only `had_output` into its notice predicate. That
made a false empty-turn notice for failed startup history. The real producer
contract and both event orders are now covered, and the predicate preserves
normal empty-turn feedback after unrelated failure history.

## Scope

### In scope

Typed safe selection failures; initial start and resume operation evidence;
normalization/persistence/reload/transport coverage; cause-first recovery UI;
identity-based duplicate suppression; independent workspace outcomes;
empty-turn feedback; confirmed recovery model/history; locales; public recovery
instructions during implementation; affected desktop and phone E2E fixtures.

### Out of scope

Provider or model policy changes, automatic fallback, saved-setting mutation,
conversation replacement, workspace implementation changes, new recovery
operations, feature flags, model-selector redesign, live-instance mutation,
legacy cause reconstruction, and commit/publication work.

## Technical approach

### Task 01: Typed cause to durable evidence

Create typed evidence in `start_model.go` and `applyExplicitSessionMode`.
Preserve `errors.Is` cancellation/deadline semantics and existing policy behavior.
Extend `BootstrapFailure` safe fields, task `AgentErrorCause`, and code/operation
normalization. The five codes are `model_unavailable`, `model_selection_failed`,
`permission_mode_failed`, `permission_mode_unconfirmed`, and
`permission_mode_mismatch`. Each has a closed reason set in the design.

Add optional requested/effective model/mode and `prompt_not_sent`; include `start`
in allowed operations. Use existing outer attempt/execution/stamp/timestamp fields.
Normalize before persistence, including bounded safe identifiers, not arbitrary
provider strings. Extend both orchestration projection paths and their history,
DTO, and task-summary boundaries. Retain atomic failure settlement and guards.
No SQL migration: the new fields live in existing metadata.

### Task 02: Cause-first visible recovery

Extend frontend `AgentErrorCause`, last-error parsing, active-error adapters,
hydration, and history projection. Give `buildRecoveryCardModel` explicit primary
cause and separate workspace status. Prefer specific durable evidence only for
correlated projections of the same attempt; keep independent failures distinct.

Extend request-local recovery failure state with request/attempt association where
its current operation-only type loses ownership. Preserve admission in
`useSessionRecoveryActions` and the existing pending registry. Do not create a
new mutation owner. Update safe details formatting and Copy details in the existing
primitive; validated host correlation fields are separate from redacted legacy prose.
Add all source/localized catalogs and generated variants in the same work order.

### Task 03: Faithful empty-turn feedback

Pass existing turn metadata through `maybeEmitEmptyTurnNotice` and exclude
`lifecycle_only`. Trace failure turn settlement and retain bounded pre-dispatch
correlation in existing turn metadata if needed. Cover completion-before-failure
and the reverse; remove only the exact matching synthetic warning if required.
Do not suppress a healthy empty turn because its session once failed startup.
Use actual turn, execution, attempt, and error-stamp associations.

### Task 04: Confirmed recovery notice and history

Extend existing provider-restored success metadata only where necessary for a
stable model display label and prior resolved error stamp. Use the notice's own
confirmed selector snapshot. The existing success producer already stores
known flags and IDs. Render known model or honest unknown success and preserve
the settings/permission disclosure. Keep history dated and make only the matching
failure resolved. A manual dismissal is not success.

Extend existing fixtures to prove native identity, unchanged saved settings,
strict future resume, read-only separation, distinct attempts, unknown selectors,
and stale event rejection. Update public recovery guidance only with implemented
behavior and record exact work-order results.

### Provider compatibility matrix

| Provider/path | Transport and identity | Intended behavior | Evidence and unsupported fallback |
| --- | --- | --- | --- |
| Auggie ordinary start/resume | ACP, executor catalog, native conversation | Strict model/mode admission with specific failure | Lifecycle plus existing settings-recovery fixture; absent evidence uses generic cause |
| Auggie explicit failed recovery | ACP, same native identity, validated `provider_restored` | Omit overrides for this attempt, keep saved inputs | Existing desktop/mobile trace assertions; invalid identity remains error |
| Other ACP agents | Shared model/mode helpers, declared capabilities | Explain actual failure without changing policy | Table-driven shared tests; successful fallback warnings stay warnings |
| Passthrough/non-ACP/preparation errors | Existing runtime paths | Existing fallback/specialized presentation | Legacy tests; no fabricated model/mode cause or Auggie eligibility |
| Workspace, auth, branch, cancellation | Existing ownership and recovery checks | Preserve independent outcome and permissions | Existing fenced tests plus read-only fixture; no policy bypass |

## ASCII UI preview

Copy uses i18next; examples show safe mockable values. Structural requirements
are cause visibility, separate outcomes, action order, and disclosure behavior.
Spacing, widths, and icons are illustrative. The complete wording and unknown
fallbacks are in the design.

### UI-01: Current failure after read-only restoration

Entry: selected task Chat, blocked composer. Criteria .21, .27-.29, .34.
Current source behavior: read-only title suppresses the main summary; cause
labels and details can repeat generic bootstrap explanations.

Desktop proposal:

```text
+---------------------------------------------------------------------+
| Saved model unavailable                                             |
| Auggie did not list "claude-opus-4-8" among its available models.      |
| Your previous conversation is preserved. No prompt was sent.*        |
| Workspace available in read-only mode. The agent has not resumed.    |
|                                                                     |
| Resume this conversation using Auggie's restored settings.            |
| Saved selections remain unchanged.                                  |
| [Existing disclosure: restored permission mode may differ]           |
| [Resume] [Start fresh session] [eligible workspace/branch actions]     |
| A fresh session uses saved selections and may hit the same problem.   |
| > Technical details                                                 |
+---------------------------------------------------------------------+
```

Phone proposal uses the same inline recovery region and stacked actions:

```text
+-------------------------------------+
| Saved model unavailable             |
| Auggie did not list                  |
| "claude-opus-4-8" among its           |
| available models.                   |
| Conversation preserved.             |
| No prompt was sent.*                |
| Workspace available read-only.      |
| The agent has not resumed.          |
| Resume uses restored settings.      |
| Saved selections remain unchanged.  |
| [Existing permission disclosure]    |
| [             Resume              ] |
| [       Start fresh session       ] |
| [       other eligible actions    ] |
| Fresh start may hit the same issue.  |
| > Technical details                 |
+-------------------------------------+
```

`*` No-prompt line exists only with source evidence. Workspace restoration is
shown as an action only while eligible; the available workspace outcome does
not promise resumed agent access. All existing confirmations are retained.
Fine-pointer controls stay 28px; phone/coarse-pointer hit targets are at least
44px. Labels and unbroken IDs wrap without horizontal document overflow.
Expanded content uses the current bounded recovery-region scroller; the
transcript keeps its sibling scroller. No nested technical-text scrollbox.

### UI-02: Expanded details and pending/failed retry

Entry: Technical details on UI-01. Criteria .24-.26, .28-.29, .34.

```text
v Technical details                    [Copy details]
  Operation: resume
  Phase: bootstrap
  Cause: model_unavailable
  Reason: requested_not_advertised
  Requested model: claude-opus-4-8
  Effective model: mock-fast (only if reported)
  Prompt sent: no (only if known)
  Occurred: <original timestamp>
  Attempt: <validated host reference>
  Execution: <validated host reference>
```

Copy returns exactly the displayed safe fields. Pending Resume keeps the cause,
shows status/spinner, and disables equivalent operations. A later failed load
gets its own occurrence and explanation. An independent workspace-restore error
gets a separate operation section. Neither adds duplicate bootstrap text.

### UI-03: Resumed conversation and historical failure

Entry: matching recovery reaches authoritative readiness. Criteria .32-.34.

```text
Chat history (normal transcript scroller)
  <time A> Saved model unavailable [Resolved]  > Technical details
  <time B> Session resumed with Gemini 3.7 Flash.
           Your previous conversation was preserved.
           [Existing skipped-settings and permission disclosure]

Composer: [draft and attachments retained                       ] [Send]
```

If current model was not confirmed for that attempt:
"Session resumed. Your previous conversation was preserved."
A future failed attempt owns a new active card; this dated success is historical.
Phone shares this composition and its native chat layout without a new overlay.

## Tests

All test additions below are planned, not existing or executed evidence.
Use TDD: assert current failure first, then change production code and record
red/green results. Preserve existing assertions rather than replace scenarios.

| Acceptance criterion | Planned executable evidence | Work order |
| --- | --- | --- |
| 006.21 | `start_model_executor_authority_test.go`: `TestStrictSelectionBootstrapEvidence/missing_model`; executor `TestBootstrapSelectionEvidenceProjection`; card test `shows saved model cause before disclosure` | 01, 02 |
| 006.22 | Same lifecycle table: `catalog_empty`, `selection_unsupported`, `application_failed`; card cause variants | 01, 02 |
| 006.23 | `session_mode_source_test.go`: `TestPermissionModeBootstrapEvidence` with failed, unconfirmed, missing effective, mismatch, deadline/cancel cases; card cause variants | 01, 02 |
| 006.24 | `launch_errors_test.go`: `TestSelectionCauseRoundTrip`; real SQLite `session_bootstrap_failure_evidence_test.go`: `TestBootstrapSelectionEvidenceSurvivesReload`; DTO/status-summary serialization tests; `session-last-agent-error.test.ts` | 01, 02 |
| 006.25 | Backend normalization tables and frontend last-error/card legacy and malformed field tables | 01, 02 |
| 006.26 | Backend secret sentinels absent from persisted JSON, DTOs, and messages; frontend details/card/clipboard tests | 01, 02 |
| 006.27 | Card read-only regression plus existing settings recovery helper before/after restore/reload | 02 |
| 006.28 | `session-recovery-presentation.test.ts` and card tests for same stamp/attempt projected via three sources, including `session-recovery-card.test.tsx` | 02 |
| 006.29 | Same tests with same text/different stamps, new failed recovery request, and sibling sessions | 02, 04 |
| 006.30 | `empty-turn-notice.test.ts`, `turns.test.ts`, task `service_turns_test.go`, and backend bootstrap-history tests for both event orders | 03 |
| 006.31 | Existing slash-command/subagent tables plus a real completed-empty-turn control after recovery | 03 |
| 006.32 | `explicit_resume_notice_test.go` snapshot/unknown/label persistence; `status-message.test.tsx`; settings recovery E2E identity/settings checks | 04 |
| 006.33 | Existing executor `TestBootstrapFailureSuccessorFence` and `TestBootstrapFailureSuccessorFenceAfterFinalOwnershipRead`; notice `TestExplicitResumeNoticeRejectsSuccessorAndOrdinaryResume`; recovery UI order tests | 01, 04 |
| 006.34 | Component locale/clipboard/accessibility tests and desktop/mobile settings and recovery UI specs | 02, 04 |

Paths in this table are relative to the directories named in work-order file
lists. The new SQLite test file is proposed; other named files exist.

## E2E tests

Reuse the existing worker-isolated backend, native mock Auggie registration,
ACP trace, and `session-resume-settings-recovery.ts` helper. Add only scriptable
mock modes needed to force catalog/apply/confirmation variants. Never seed a
live developer database. The fixture may edit only its disposable database.

| Flow | Criteria | Specs/projects |
| --- | --- | --- |
| Strict saved-model failure; visible cause; copy fields; read-only restore and reload; later failed load kept distinct | .21, .24-.29, .34 | `session-resume-settings-recovery.spec.ts` / chromium; `mobile-session-resume-settings-recovery.spec.ts` / mobile-chrome |
| Unconfirmed/mismatched mode after successful model application; no prompt; catalog absent and unsupported controls | .22-.23, .30 | Extend mock branches in the same helper/spec pair; application distinctions also have unit coverage |
| Real completed empty prompt after healthy/recovered readiness | .31 | Existing recovery helper plus `session-error-recovery-ui` pair, using a real mock prompt with no output |
| Successful explicit Resume: same native identity, unchanged saved input, confirmed model/unknown notice, one historical success, stale failure rejected | .29, .32-.33 | Settings recovery pair, preserving #4065's request/trace/selector assertions |
| Long values, separate operation failures, safe copy success/failure, wrapped details, accessible actions, pending/focus and zoom/narrow phone checks | .26-.29, .34 | `session-error-recovery-ui.spec.ts` / chromium; `mobile-session-error-recovery-ui.spec.ts` / mobile-chrome |

Use causal WS/HTTP waits and backend-state polls. Run desktop and phone commands
sequentially through the guarded runner, which builds production artifacts and
limits workers/shards. Do not pass all-worker overrides or overlap full suites.
Native phone composition means the existing Pixel 5 mobile-chrome task layout;
this package does not change the Tauri shell or require unrelated Rust tests.

## Documentation and existing packages

Update the existing requirements and link the supplemental design from the
near-limit recovery design. Do not grow that design past 32 KiB. No system
README boundary change is required; the catalog discovers the new design.

The existing `agent-resume-mode-fallback`, `session-error-recovery-ui`,
`contribution-resume-recovery`, and `error-scope-and-history` packages record
shipped behavior. Their historical results remain valid history. During the
owning work order, add a concise forward link where necessary and reconcile any
changed fixture assertion or scenario wording; do not rewrite prior green counts
as evidence for this change. Implementation results belong in this package.

Public guidance now covers the specific cause, restored-settings eligibility,
fresh-start limitation, read-only separation, copied diagnostics, and resolved
history in `docs/public/sessions-and-review.md`. The profile page links to that
recovery guidance. README and screenshot references were checked; neither needs
an update for this behavior, and no new screenshot asset is required.

## Work orders

- [x] [Task 01: Persist typed startup selection evidence](task-01-typed-startup-evidence.md)
- [x] [Task 02: Present correlated causes and workspace outcomes](task-02-visible-recovery-causes.md)
- [x] [Task 03: Separate bootstrap failure from empty agent turns](task-03-empty-turn-feedback.md)
- [x] [Task 04: Report confirmed recovery and preserve history](task-04-recovery-success-history.md)

Dependency order: 01 -> 02 -> 03 -> 04. Tasks share contracts and fixtures;
execute sequentially. Each work order includes its exact scoped checks.
Frontend dependencies were installed in this checkout and reused. Desktop and
mobile E2E runs completed sequentially with the guarded runner.

## Verification results

Planning validation on September 30 passed:

- `git merge-base --is-ancestor fa729f2d7653f4480c107c6a6d50a5452eb3dd0e HEAD`: exit 0.
- `python3 scripts/list-docs.py validate`: 333 decisions and 1261 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/list-docs.py specs --system agents --text session-startup --format paths`: finds the requirement and both relevant designs.
- `git diff --check -- docs/specs docs/decisions docs/plans/session-startup-failure-explanations`: clean.
- Work-order command path audit: all listed unit/lint/E2E inputs and Go packages exist.
- Repository `validateCoverage` preflight below: covered, all four work orders accepted, zero errors. It deliberately simulates one future production path because a documentation-only diff is otherwise exempt.
- `git status --short -- docs/plans/session-startup-failure-explanations`: includes the new untracked package; nothing staged or committed.

Coverage preflight, from repository root:

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const dir = 'docs/plans/session-startup-failure-explanations';
const paths = fs.readdirSync(dir).filter(p => p.endsWith('.md')).map(p => `${dir}/${p}`);
paths.push('docs/specs/agents/requirements/session-recovery-failures.md',
  'docs/specs/agents/system-design/session-startup-failure-explanations.md');
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'added' }));
changedFiles.push({ filename: 'apps/backend/internal/agent/runtime/lifecycle/start_model.go',
  status: 'modified' });
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status,
  workOrders: result.workOrders, errors: result.errors }, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

Product tests, typecheck, i18n, source lint, E2E, and public documentation checks
are recorded in the work-order results below.

Task 01 implementation verification passed. The full six-package race command
passed; after its final normalization-helper refactor, the affected evidence,
SQLite reload, DTO/summary, safe projection, and successor-fence tests passed
again. Scoped Go lint reported zero issues. See
[Task 01 results](task-01-typed-startup-evidence.md#results) for exact commands,
durations, and the PostgreSQL fixture note. Task 02 passed 130 focused frontend
tests, typecheck, catalog validation and generation, i18n ratchet, scoped
zero-warning ESLint, plus guarded E2E (Chromium 9/9, mobile-chrome 9/9). See
[Task 02 results](task-02-visible-recovery-causes.md#results). Task 03 passed
focused red/green frontend tests (48 green), backend producer/race tests and
lint, frontend typecheck/ESLint/i18n ratchet, and desktop/mobile E2E (9/9 each).
See [Task 03 results](task-03-empty-turn-feedback.md#results). Task 04 passed
its original 190-test focused frontend set, which grew to 208 tests for the
review-finding regressions. The final follow-up passed web typecheck, full web
lint, i18n check/ratchet, six-package Go race tests, scoped Go lint, public-doc
and specification validators, and sequential Chromium and mobile-chrome E2E
(9/9 each). The E2E runner built the backend, fixture plugin, and pseudo-locale
Vite assets. See [Task 04 results](task-04-recovery-success-history.md#results)
for detailed commands and reruns.

Review-finding follow-up verification passed: the provider-restored success
notice stores only sanitized model evidence; mounted details retain `bootstrap`
and validate `resume-<uint64>` attempt references separately from UUID
execution IDs; every stamped successful resume writes an owned bounded
resolution independent of transcript delivery; history renders immutable typed
evidence with one compact active-row marker; and model-application failures
identify the actual safe value sent to `SetModel`. Regressions cover absent
transcript history, successor failures, manual dismissal, safe-copy/DOM
redaction, fallback and variation rejection, and unknown-model fallback.

## Risks

- Legacy generic records have already lost their model/mode cause. This work
  cannot recover it without original evidence and must not infer it from text.
- Async resume may return a request error before durable cause publication.
  Reconcile only proven ownership; retain independent uncorrelated errors.
- Backend sanitizers and UI sanitizers redact UUID-like data. Permit only
  validated host references as structured details; do not weaken prose redaction.
- Task 03 confirmed that turn metadata already carries lifecycle and failure
  ownership. The frontend now consumes those per-turn markers, so historical
  failures do not suppress unrelated real empty turns.
- Current provider-restored notice may receive no selector report before ready.
  Report honest unknown rather than substitute the saved model.
- Catalog/model IDs and long translations can be hostile or unusually large.
  Test omission, bounds, wrapped text, and copy content at every boundary.
