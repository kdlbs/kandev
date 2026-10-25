---
id: "10-execution-surfaces"
title: "Deliver Execution settings and session visibility"
status: in_progress
wave: 8
depends_on:
  - "04-policy-authority"
  - "05-nono-provider-lifecycle"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-006
  - REQ-AGENTS-SANDBOX-007
  - REQ-AGENTS-SANDBOX-008
  - REQ-PLUGINS-SANDBOX-001
acceptance_criteria:
  - AC-AGENTS-SANDBOX-006.1
  - AC-AGENTS-SANDBOX-006.2
  - AC-AGENTS-SANDBOX-007.1
  - AC-AGENTS-SANDBOX-007.2
  - AC-AGENTS-SANDBOX-007.3
  - AC-AGENTS-SANDBOX-007.4
  - AC-AGENTS-SANDBOX-007.5
  - AC-AGENTS-SANDBOX-007.6
  - AC-AGENTS-SANDBOX-008.1
  - AC-AGENTS-SANDBOX-008.2
  - AC-AGENTS-SANDBOX-008.3
  - AC-AGENTS-SANDBOX-008.4
  - AC-AGENTS-SANDBOX-008.5
  - AC-AGENTS-SANDBOX-008.6
  - AC-AGENTS-SANDBOX-008.7
  - AC-AGENTS-SANDBOX-008.8
  - AC-PLUGINS-SANDBOX-001.6
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
---

# Task 10: Deliver Execution settings and session visibility

## Summary

Expose generic wrapper configuration and optional sandbox policy in one Execution section.
Capture launch snapshots and project current evidence into shared session details and truthful desktop/mobile indicators.

## In scope

- Move `CommandPrefixField` into Execution without changing saved prefixes or core-only behavior.
- Reuse policy authority for executor context, agent defaults, and workflow/task selection.
- Persist safe immutable snapshots for new launches, including attempts without a sandbox.
- Project attempt-bound status, capability health, redacted details, and separate next-launch settings through existing authenticated session transports.
- Bind shield visibility to validated runtime evidence. Handle stale events, legacy data, reconnect, provider loss, stop, and sibling sessions.
- Render one shared detail model from Dockview, session actions, and phone status controls. Link available approvals and reports.
- Add all localized copy and focused desktop/mobile E2E coverage.

## Out of scope

Provider-specific host pages, CLI/profile parsing in core, synthetic enforcement claims, a task-wide protection badge, and a security dashboard.
Task 08 owns denial-report normalization, learning review, and policy publication. This task exposes their supported/unsupported states and entry points.
No user-terminal confinement, automatic installation, policy weakening, or changes to task completion semantics.

## Acceptance

- Wrapper-only configuration works with plugins disabled. Two fixture provider identities use the same settings and permissions summary without provider-name branches.
- Captured/current/next-launch data remains separate. Only fresh bound confinement evidence creates an active shield, and secret or foreign-attempt data never appears.
- Desktop and phone users reach equivalent details and configuration. Keyboard/touch activation preserves tab behavior, draft state, focus, and scroll containment.

## UI preview

See [UI-01, UI-05, and UI-06](plan.md#ui-preview) for the combined structural preview.

| View and criteria | Desktop order | Phone order |
| --- | --- | --- |
| UI-01, SANDBOX-007.1-6 | Execution heading, Command wrapper editor/validation/preview, Sandbox provider/default/permissions/source, Save | Same single-column order, bottom executor/policy picker, return to preserved draft |
| UI-05, SANDBOX-008.1-2, .4-6, .8 | Dialog title/close, status/time, This attempt, Protection scope, Next launch, Approvals, Denials | Fixed Back/title, same sections in one full-height scrolling body, safe-area actions |
| UI-06, SANDBOX-008.3, .7 | One session-tab icon button opens UI-05 without dragging or closing the tab | Labelled Execution status control with at least 44px target opens UI-05 |

The no-plugin state retains Command wrapper. An unavailable provider retains its selection and displays a blocker.
Unknown evidence removes the live shield and keeps historical configuration. Stopped attempts display history with no active protection claim.
Use one status icon when a wrapper and sandbox coexist. Details explains both layers and the exact protection scope.
Existing approval/report components remain the source for their records. Do not duplicate pending decisions in this view.
The group order, navigation, state labels, and attempt identity rules are requirements. Copy examples and icon spacing are illustrative.

## Verification

All commands run from the repository root. New test paths are implementation deliverables, not existing passing evidence.
Install workspace dependencies from `apps/` before the first package command in a fresh worktree.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/sandbox/... -run 'ExecutionSnapshot|ExecutionEvidence|ExecutionProjection|SandboxPolicy' -count=1)
(cd apps/backend && go test -trimpath ./internal/plugins/... -run 'SandboxProvider' -count=1)
pnpm --dir apps/web exec vitest run lib/execution-status.test.ts components/settings/profile-execution-section.test.tsx components/task/session-execution-indicator.test.tsx components/task/session-execution-details.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/execution-settings.spec.ts tests/task/execution-details.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-execution-settings.spec.ts tests/task/mobile-execution-details.spec.ts -- --retries=0
```

Use TDD for changed logic. Run suites sequentially with the repository's worker budget.
Do not count an empty test filter as a pass.

| Evidence | Required cases |
| --- | --- |
| `TestExecutionSnapshot` | Wrapper-only and sandbox attempts, immutable revision/source, next-launch difference, redacted values, legacy unknown, database round trip |
| `TestExecutionEvidence` | Availability alone, launch receipt alone, bound evidence, wrong policy/process, deadline, freshness expiry, generation change, plugin loss, stopped supervisor |
| `TestExecutionProjection` | Session authorization, shared/read-only redaction, no mutation from viewing, sibling isolation, late active event after stop |
| `execution-status.test.ts` | Every status, no shield for custom prefix text, old/foreign events, reconnect, historical evidence, service health distinct from confinement |
| Component/browser checks | Prefix value preserved after moving from Advanced, no-plugin editor, executor-context resolution, two fixture providers, wrapper-plus-sandbox, invalid/read-only states |
| Desktop/phone rendered checks | Two sessions, current/next distinction, keyboard tab action, focus return, 44px targets, long names, keyboard-safe actions, no page overflow |

Record desktop and phone rendered checks for UI-01, UI-05, and UI-06, including failure and unknown states.
Actual Nono status evidence remains subject to Task 01 and Task 09 acceptance. Mock metadata proves UI behavior only.

## Files likely touched

- `apps/backend/internal/agent/sandbox/execution_snapshot.go (new)`
- `apps/backend/internal/agent/sandbox/execution_snapshot_test.go (new)`
- `apps/backend/internal/agent/runtime/lifecycle/`
- `apps/backend/internal/agentctl/server/process/`
- `apps/backend/internal/task/models/`
- `apps/backend/internal/task/handlers/`
- `apps/backend/internal/task/repository/`
- `apps/web/lib/types/http.ts`
- `apps/web/lib/state/slices/`
- `apps/web/lib/execution-status.ts (new)`
- `apps/web/lib/execution-status.test.ts (new)`
- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/components/settings/profile-advanced-options.tsx`
- `apps/web/components/settings/command-prefix-field.tsx`
- `apps/web/components/settings/profile-execution-section.tsx (new)`
- `apps/web/components/settings/profile-execution-section.test.tsx (new)`
- `apps/web/components/settings/executor-profile-dialog.tsx`
- `apps/web/components/settings/workflow-step-agent-profile-selector.tsx`
- `apps/web/components/task/session-tab.tsx`
- `apps/web/components/task/session-tab-menu.tsx`
- `apps/web/components/task/session-execution-indicator.tsx (new)`
- `apps/web/components/task/session-execution-indicator.test.tsx (new)`
- `apps/web/components/task/session-execution-details.tsx (new)`
- `apps/web/components/task/session-execution-details.test.tsx (new)`
- `apps/web/components/task/mobile/session-mobile-top-bar.tsx`
- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/src/locales/`
- `apps/web/e2e/tests/settings/execution-settings.spec.ts (new)`
- `apps/web/e2e/tests/settings/mobile-execution-settings.spec.ts (new)`
- `apps/web/e2e/tests/task/execution-details.spec.ts (new)`
- `apps/web/e2e/tests/task/mobile-execution-details.spec.ts (new)`

## Dependencies

04-policy-authority and 05-nono-provider-lifecycle provide policy resolution, snapshots, and runtime ownership primitives.
Task 03 supplies generic metadata/evidence contracts. Task 01 supplies the Nono evidence feasibility gate through Task 05.
The plan places this task after 06 and 07 so available approval and MCP details can use their real contracts.
Task 08 adds learning after this shared surface exists.

## Risks

An active shield can overstate protection if it follows mutable settings or stale evidence.
If Nono cannot supply reliable per-attempt evidence, show unknown and keep that support claim blocked.
Raw prefix values can contain secrets. Diagnostic projection must use a safe representation, not a permissive redaction heuristic.
Do not infer recovery success from a replacement session or from a sandbox-status update.

## Parallelism

`sequential`

## Inputs

- [Agent requirements](../../specs/agents/requirements/sandbox-policies.md).
- [Agent design](../../specs/agents/system-design/sandbox-policies.md#operator-surfaces).
- [Plugin contract](../../specs/plugins/system-design/sandbox-provider-capability.md).
- Existing profile form, Dockview session tab, mobile picker, and task/session projection patterns.

## Results

Implementation is authorized by the user's later request. The core command-wrapper field now appears in an expanded Execution section for both ACP and native-terminal profiles. Existing form tests verify saved-value preservation, native editing, section order, and dirty-state behavior. All 32 focused form/field tests pass. Updated copy removes the obsolete ACP-only restriction and distinguishes wrapping from verified sandbox protection. Seven locale catalogs are updated. Policy selectors, validation/preview, session details, and desktop/mobile rendered checks remain pending.

The policy catalog and prospective-selection API now enforce human-operator authority, tenant/workspace scope, bounded pagination, and saved revision precedence. SQLite and PostgreSQL checks pass. Generic installed-provider metadata is wired without provider-name branches. The native-terminal command preview now includes the configured wrapper. A safe preview mode omits opaque arguments, environment values, and executable directory paths. API privacy, validation, and both-transport preview tests pass. The frontend preview is integrated into the Execution disclosure.

The agent policy selector requires explicit executor context and preserves incompatible stored selections. It supports explicit replacement and inheritance, displays canonical grants and provider metadata, and refreshes the saved selection source after saving. Desktop and phone checks pass with two generic provider identities. The phone check covers the bottom-sheet picker, draft preservation, 44px touch targets, and horizontal overflow. Frontend typecheck passes. These fixtures prove UI behavior only.

The explicit availability API now validates operator authority and current selection, verifies the installed provider runtime artifact, and rechecks policy and installation identity before success. It performs no agent launch or policy mutation. The packaged-provider integration and bounded-deadline, stale-selection, and private-error tests pass. Desktop and phone browser checks for the availability control and invalid-selection alert pass. The phone screenshot was inspected. Typecheck, localization, public-document validation, and backend lint pass.

Executor configuration now uses the existing profile page and shared Save coordinator. The form states that its policy applies to all profiles of that executor. Mode, provider, ceiling, eligible revisions, and default remain distinct explicit choices. Desktop and phone checks pass for exact saved bindings. Missing or incompatible selections remain available for explicit repair.

Initial policy creation now uses generic canonical grants. Defaults grant no access. Publication does not change the executor binding. Desktop and phone create-and-select flows pass, including explicit work/state references, exact network destinations, environment names, and MCP intent. Unit tests verify duplicate-submit prevention and ignored callbacks after editor disposal. The slice passes typecheck, localization checks, lint, and 55 focused tests.

Phone executor settings use the existing vertical profile page, bottom-sheet policy pickers, one page scroll owner, and fixed Save action. Policy creation uses a full-height form with a fixed title and Cancel/Create actions, one contained scrolling body, safe-area spacing, and native access/protocol selectors. Desktop uses a bounded dialog. The existing plugin executor profile dialog is the surface exemplar. Immutable public execution snapshots, session details, indicators, and learning remain open.


Workflow and task editors now reuse the generic selection model. Workflow preview choices preserve inherited agent assignment. Started tasks require an explicit preview-agent context. Task previews include the current workflow step. Task policy drafts save through Update before any launch, and Cancel discards them. Invalid drafts and failed saves block launch. Late responses from an obsolete operator or editor cannot continue the old save.

All four desktop and phone flows pass together: agent settings, executor policy creation/configuration, workflow selection, and task editing. The phone task form uses native selectors inside the existing full-height dialog, with one form scroll owner and its fixed Cancel/Update footer. It preserves the existing task-edit geometry. Tests cover 44px targets, overflow, Cancel/Update, workflow context, and saved payloads. The phone screenshot was inspected. The latest focused unit run passes 64 tests. These fixtures prove interface behavior, not actual Nono confinement.


Immutable public launch records now pass SQLite and PostgreSQL race checks. Capture rejects stale execution ownership, conflicting replays, private command values, and caller-supplied sequence values. Session reads preserve frozen history, isolate siblings, return unknown for malformed records, and cascade records on deletion. The public session DTO carries this history. Frontend reconciliation rejects stale or foreign snapshots and clears history when a session identifier is recreated. All 428 session-store tests pass, and the final 11 focused snapshot tests, typecheck, and scoped frontend lint pass. Production launch capture, evidence projection, and session details remain in progress.


Production capture now runs before chat and native-terminal start. Persistence failure prevents dispatch. The packaged provider check binds public records to the exact private attempt, policy revision, and runtime digest. Kubernetes refresh, workspace rebind, and rollback paths also capture history. Sandboxed workspace rebind is refused before stopping the existing process, preserving its captured scope. Focused lifecycle and integration race tests pass. State-change and reconnect transports carry the snapshot; 72 frontend handler tests and focused backend transport checks pass. Live evidence, the shared details surface, and immediate capture publication remain open.


A dedicated execution notification now publishes captured history immediately without a task-state transition. Gateway checks prove workspace scoping and rejection of unattributed events. The browser rejects old session incarnations, foreign tasks, and older attempt sequences while preserving lifecycle state. The 73 transport tests, focused backend race tests, typecheck, and scoped frontend lint pass. Runtime observation now keeps provider health separate from confinement. The real packaged Nono check returns available/unknown from registry identity, unavailable/unknown after plugin removal, and stopped after verified cleanup. Bound-evidence tests reject wrong policy, attempt, generation, process birth, future time, and expired evidence. Persistent observations and their UI projection remain in progress.


Observation storage now fences writes by backend owner, current snapshot, and private attempt version. SQLite/PostgreSQL read/list checks preserve historical evidence without restoring active status after restart. Browser reconciliation and status derivation pass 543 tests. A lost connection removes live status; reconnect requires a newer observation. The status helper rejects foreign, expired, future, or incomplete evidence and never infers protection from a wrapper name. The core observer, backend observation publication, and session details remain in progress. The orchestrator lint command timed out without findings and remains unverified.


The core observer is now wired during backend startup. Its owner claim invalidates old live evidence even when the provider is disabled. Public capture and committed private lifecycle transitions wake the observer. The periodic loop uses bounded pages, four concurrent reads, and a five-second provider deadline. Provider errors, nil results, and timeouts remove live claims without exposing private errors. Shutdown joins the loop. Focused backend race checks pass, including replacement attempts and workspace-scoped publication. Storage/runtime lint, SQL guard, and the complete sandbox policy package pass. Schema conformance and event-package lint remain under verification.

The shared browser expiry clock starts only for visible live-status consumers and stops after the last consumer leaves. The session hook uses the real store and keeps sibling sessions separate. Hook/clock/status tests pass, including expiry without incoming traffic and fresh observations after reconnect. No rendered session details or indicator is delivered yet.

The final snapshot identity regression now passes on SQLite and PostgreSQL: public and sandbox attempt IDs must match. Scoped Go lint passes. The schema upgrade checks pass after adding the new store and tables to the previous-stable fixture manifest; historical SQL fixtures remain unchanged. Event-package lint and the exact-deadline hook typecheck pass.

Execution details now opens from the desktop tab button and context menu, the labelled phone header control, and the phone session picker. The phone view reuses the full-height dialog composition with a fixed Back/title row, safe-area spacing, and one contained scrolling body. Desktop uses a bounded dialog. Both use the same session state, immutable launch record, evidence status, and authorized policy reads. Captured policy permissions remain separate from current policy defaults. Missing reports remain unavailable rather than implying zero denials. Approval actions and report navigation are not implemented yet.

The 40 focused frontend tests pass for status expiry, reconnect, sibling isolation, control activation, focus return, policy revision binding, read-only authority, and sign-out during a request. Typecheck, scoped frontend lint, and localization checks pass. Desktop and phone browser scenarios are added; rendered verification is running. These fixtures do not prove Nono confinement or complete application acceptance. Current command defaults, approval actions, denial learning, and explicit restart policy review remain open.


Safe saved-profile command previews now resolve launch settings on the server and expose only command composition. ACP/native handler tests cover changed wrappers, missing profiles, configuration authority, and private-error redaction. Browser reads refresh after profile updates and discard responses after sign-out or profile changes. Captured commands remain immutable. Details now show captured selection source and ceiling digest, provider health at the recorded observation, and separate current command/policy defaults.

The new phone status row initially overlapped the session picker. A shared header-height value now aligns the panel, page feedback, and error surfaces. Seventy-four focused layout/component tests passed. The final preview/details slice passes forty tests, typecheck, lint, and localization checks. Desktop and phone rendered flows pass with keyboard/touch entry, focus return, sibling isolation, long text, one scrolling body, and phone widths 393, 700, and 767. Both screenshots were inspected. Approval actions, explicit restart policy review, and full actual-agent acceptance remain open.


The shared approval review is now connected to committed, workspace-scoped live events. It shows the session, captured policy revision and digest, exact operation and target, deadline, and delivery state. Allow once and Deny require an explicit operator action. Decisions preserve saved policies. Reads and mutations are scoped to the authenticated user, session incarnation, attempt, and connection epoch. Duplicate clicks share one request. Late responses after sign-out or operator replacement cannot enable an old action.

A deferred-read regression found that ordinary query invalidation could retain an older empty response. Live events now cancel that read and obtain the current list. Desktop browser testing found that the final request removed its own focus target; closing now returns focus to Execution. Twenty-four focused frontend tests, typecheck, localization, scoped lint, and public-doc validation pass. Desktop and phone browser checks pass for live arrival, explicit allow/deny, focus return, read-only access, expiry, disconnection, long content, 44px touch targets, and phone widths 393, 700, and 767. Both final screenshots were inspected. Browser fixtures prove interface behavior only. Full actual-agent acceptance, denial learning, explicit restart review, and child-state deletion remain open.
