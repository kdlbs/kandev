---
created: 2026-10-26
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
legacy_specs: []
---

# Implementation plan: Focused SSH management

## Overview and intent

Make SSH management match Kandev's settings UI. Replace the crowded combined
editor with a useful collection, a readable saved overview, and focused edit
and access flows. First deliver browsing/editing, then access setup, then
contextual executor integration. Each slice remains usable independently.

The user supplied screenshots and requested a less crowded experience on
2026-10-26. Keep SSH under Executors, as previously requested. The source
baseline is `b3517cdd869f2d6db96ad76c666338473e816cdf`, PR #4778. This package
was approved for local implementation by the user's later “go for it” request.
It does not authorize publication or a running-instance refresh.

Executors owns the durable shared SSH contract. Its existing requirement file
adds AC-EXECUTORS-SSH-CONNECTIONS-004.8 through 004.12. The focused companion
design now owns presentation and the prior navigation/continuation text;
the service design retains security and runtime ownership. No new UI system
requirement, backend API, persistence boundary, or ADR is needed.

## Evidence and assumptions

Confirmed: the user rejects the visual density and inconsistency. At baseline
`b3517cdd869f2d6db96ad76c666338473e816cdf`, `settings.tsx` mounts the name-only list next to a monolithic editable detail;
`connection-details.tsx` mounts trust, preflight, enrollment and deletion;
`enrollment-panel.tsx` leaves bootstrap controls visible after success. The
Executors page renders its navigation button as a separate spaced section.

Approved design choices: focused detail navigation on desktop as well as
phone; Overview/Access/Diagnostics detail tabs; enrollment only after an
explicit Install key action. The implementation preserves that approved
hierarchy. Routine geometry uses current shared primitives. No material
permission or SSH-policy choice is unresolved.

Repeated names in the screenshot also occur in separately preserved lab rounds.
Do not infer duplicate rendering or deduplicate existing data. Show endpoint,
identity and, when still ambiguous, a short public record ID.

## Scope

### In scope

- Executors header action; full-width connection and identity rows with status.
- Saved resource overview, explicit metadata edit, shared save and guarded Back.
- Guided access setup, explicit enrollment, diagnostics and public-key details.
- Identity metadata/custody/dependencies with the same visual hierarchy.
- Contextual identity/bastion creation, executor return and legacy adoption parity.
- Desktop/phone/coarse-tablet behavior, localization and public setup guidance.

### Out of scope

Backend/refactor work, live access changes, new SSH capabilities, new settings
categories, search/pagination/bulk management, automatic record merging,
renaming persisted identities, changing feature defaults, CI/review remediation,
merge-conflict resolution, committing/pushing, and updating the running lab.

## Technical approach

Use `useSSHConnections` and current API methods. Build small domain components
under `components/settings/ssh-connections/`; use `SettingsPageHeader`,
`SettingsTabs`, expanded `SettingsGroup`/`SettingsRow`, `SettingsInfo`, and
`SettingsPageTemplate(contentFrame="none")`. Retain the shared save coordinator
as the sole metadata draft owner. Do not add page-local Save controls or global
style changes. Proposed local view-state helpers derive next-step presentation
without mutating route readiness.

Extend `lib/settings/ssh-connection-settings-routes.ts` with validated `section`
and `mode` values. Retain tab/id/new and all adoption/return fields. Invalid
values fall back to overview. Update consumers through this helper. The existing
query-only navigation and persistent continuation provider retain their scope.

Guard secret-bearing forms explicitly by active section. Shared settings tabs
force-mount visited panels, so CSS hiding is not secret cleanup. Keep retained
warnings visible outside the form and ignore late responses after departure.

### Compatibility matrix

| Shape | Intended behavior | Verification | Unsupported/failure path |
| --- | --- | --- | --- |
| Managed direct, generated/imported key | Current pinned SSH operations through focused views | Real SSH enrollment/runtime browser cases | Show sanitized failed step, no fallback |
| Managed one-bastion route | Separate trust/identity per hop; same focused flow | Existing direct/bastion adoption matrix | Hop-specific error; no direct fallback |
| External file/agent identity | Existing reference modes and preflight | Identity tests and diagnostics state tests | Missing/empty/locked source remains actionable |
| Legacy inline executor | Existing editor until explicit adoption | Existing adoption browser cases | Chains stay unsupported; profile unchanged |
| Disabled/member | No entry or management effects | Existing disabled/auth browser cases | Existing unavailable/denied state |
| Old bookmark/return context | Equivalent focused record; preserve flow context | URL/navigation and continuation cases | Missing entity/continuation uses explicit fallback |

## UI previews

These structured previews follow the task's rich-output/no-ASCII instruction.
Labels illustrate hierarchy, not final translated copy. Structural requirements
are the full-width collection, focused detail, distinct edit/access operations,
visible state, one primary next action, and one page scroll owner. Current
before-state is established by the user screenshots and source above.

### UI-01: Collection and Executors entry

Entry: Settings > Executors, then SSH connections. State: populated or empty.
ACs: 004.4, 004.6, 004.8, 004.12, 005.6.

| Order | Desktop | Phone |
| --- | --- | --- |
| Executors header | Title and description; secondary SSH connections action beside it | Same shared header; action beneath description |
| Manager header | SSH connections; standard Connections/Identities tabs; New action | Same hierarchy, tabs and 44px New action contained in width |
| Populated content | One grouped list; name, endpoint, identity, state, chevron | Tappable rows; name/status first, endpoint and identity below |
| Empty content | Short explanation plus one New action | Same outcome without empty detail pane |
| Loading/error | Content status or Retry, no false empty state | Same readable status and reachable Retry |

Rows scroll with the settings shell, not a nested list. The collection does not
mount an adjacent detail card. Same-name rows expose distinguishing metadata.

### UI-02: Saved connection overview and edit

Entry: open a collection row or selected-record bookmark. State: ready overview.
ACs: 001.6, 002.3, 002.6, 004.3, 004.8, 004.9, 004.12.

| Order | Desktop | Phone |
| --- | --- | --- |
| Header | Back; name and endpoint; status; Edit; visible overflow menu | Back and menu; name/endpoint; status; reachable action row |
| Header tabs | Overview / Access / Diagnostics | Same bounded standard tabs |
| Overview | Expanded Connection and Identity/Route summaries; Test SSH access | One stacked read-only summary, then primary Test SSH access |
| Edit state | Grouped endpoint and identity fields; optional bastion choice | Full-page single-column form; related-field pickers use drawers |
| Dirty state | Shared floating Save/Reset and guarded navigation | Same save surface with safe-area/keyboard clearance |
| Delete | Overflow action and existing confirmation | Responsive menu and existing confirmation; no inline red button |

Saved overview has no metadata inputs, password field, raw public-key block or
Delete button. Current verification is evidence for the saved stamp, not a live
reachability guarantee. Editing does not silently invoke a network check.

### UI-03: Access setup, enrollment and errors

Entry: Access tab, or save a new incomplete draft. ACs: 002.2, 002.3,
003.1-003.6, 004.2-004.5, 004.10-004.12.

| State | Desktop | Phone |
| --- | --- | --- |
| Host untrusted | Target summary; next step Inspect host key | Same focused step under compact summary |
| Observation ready | Full observed fingerprint, independent-verification help, explicit Trust confirmation | Wrapping fingerprints; responsive confirmation |
| Trusted, unchecked | Test SSH access; secondary Install key and Diagnostics | One primary test action, secondary labelled choices below |
| Enrollment opened | Existing-access explanation, method, chosen credential fields, explicit Install confirmation | Focused page step, single-column fields; no nested wizard dialog |
| Verified enrollment | Short result and Test SSH access; bootstrap form absent | Same result and reachable test action |
| Uncertain/failed | Persistent warning; fresh test/recovery path; no automatic retry | Same message visible in page flow, no hidden warning |
| Diagnostics | Explicit Check prerequisites; hop-specific results | Same command and readable result list |

Access setup uses inline content and the single settings scroller. Only temporary
choices and confirmations use responsive overlays. Full public-key text appears
only after Show public key; Copy always copies the complete value. Safety
consequences and uncertain outcomes remain visible.

### UI-04: Contextual return

Entry: new SSH executor > Manage connections, or connection > Create identity/
bastion. ACs: 004.1, 004.3, 004.4, 004.5, 004.7, 004.11, 005.2, 005.6.

| Order | Desktop | Phone |
| --- | --- | --- |
| Related creation | Quiet action at the owning field; preserve non-secret draft | Same action opens focused form, not stacked drawers |
| Back/Cancel | Return to the originating draft; restore field focus | Same direct navigation and focus return |
| Executor return | Compact return context; Return versus explicit Use connection | Wrapping touch actions without obscuring resource status |
| Completion | New executor check followed by shared Save | Same explicit sequence and safe-area save surface |

Return context never starts trust, enrollment, authentication, or profile save.

## Tests and acceptance mapping

| Criteria | Unit/component coverage | Browser coverage |
| --- | --- | --- |
| 004.8, 004.9, 004.12 | Proposed `connection-presentation.test.ts`, `settings.test.tsx`; route helper tests | Proposed `ssh-management-layout.spec.ts` and phone companion |
| 004.3, 002.3, 002.6 | Existing `connection-details.test.tsx`; save-owner cases | Existing draft/conflict/navigation helpers adapted to explicit Edit |
| 004.10, 004.11, 002.2 | Proposed `access-flow.test.tsx`; enrollment/confirmation tests | Proposed desktop/phone access scenarios; real SSH specs |
| 001.1-001.3, 001.6 | Existing identity form tests; proposed overview cases | Existing identity/dependency specs and phone companions |
| 003.1-003.6, 005.2 | Existing enrollment/bootstrap tests retained | `managed-enrollment`, `managed-connections`, `managed-adoption-browser` |
| 004.7, 005.6 | Existing setup-continuation and route allowlist tests | Existing contextual setup/profile-link/navigation specs |
| 001.5, 005.3 | Existing gated mounting tests | Existing auth and disabled specs |

The work orders create the listed new files through TDD. Cover ready, incomplete,
failed, changed-host, unavailable, busy, and stale states. Opening tabs, navigating
Back, and resizing must make zero remote mutation/probe requests. Secret cleanup
must include leaving and reopening a visited panel, method changes, cancellation,
route revision change, and pending response after navigation.

## E2E tests

Desktop and phone layout specs drive collection-to-overview, Edit/Reset/Save,
explicit access steps, identity details, and state transitions. Include 390px
phone, 767/768px boundary, 820px coarse tablet, and 1280px desktop. Verify 28px
ordinary desktop controls within 1px and 44px phone/coarse hit areas. Check
focus/Back, light/dark, pseudo locale, long values and zero horizontal document
scroll. Phone project uses its configured Pixel 5 device, not per-test overrides.

Real SSH cases must pass against disposable targets, including ready-route
retest, enrollment, and direct/bastion adoption. UI fixtures prove presentation
and request boundaries only; label them as such. Run managed projects
sequentially with the repository's worker budget. Do not use the main instance,
old recovery source, running user lab, or real host credentials.

## Work orders

- [x] [Task 01: Collections, overview and metadata edit](task-01-resource-views.md)
- [x] [Task 02: Focused access and enrollment](task-02-access-setup.md)
- [x] [Task 03: Contextual setup and executor entry](task-03-contextual-integration.md)

Dependency order: 01 -> 02 -> 03. Execute here, sequentially, after an explicit
implementation request. No delegation or persistent subtask is authorized.

## Verification results

### Design checkpoint

Design-only validation passed on 2026-10-26 before implementation:

- `python3 scripts/list-docs.py validate`: 393 decisions and 1674 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all files passed.
- Local `validateCoverage` from `.github/scripts/pr-docs.cjs`: all three work
  orders accepted with no errors. A prospective change to `settings.tsx`
  activated traceability validation; no GitHub requests or statuses were sent.
- Plan/work-order relative links and referenced requirements/designs validated;
  `git diff --check` passed, including explicit checks for new untracked files.

Receipts: `/tmp/ssh3735-ux-{catalog,spec-tests,spec-lint}.log` and
`/tmp/ssh3735-ux-delivery-preflight.json`. The current requirement and both
system designs remain within their size limits. At that checkpoint, production code and permanent tests had not changed, and
rendered geometry had not been verified. These are historical design results.
The work-order results below record the later implementation checks.

### Implementation checkpoint

The user authorized implementation after the design checkpoint. All three work
orders are done. The implementation matches AC-EXECUTORS-SSH-CONNECTIONS-004.8
through 004.12 and preserves the existing security/continuation contract.
The owning requirements are active; both paired system designs are current.

| Check | Final result |
| --- | --- |
| Focused unit suite | 130 tests in 17 files passed |
| Desktop browser | 21 passed |
| Phone browser | 13 passed |
| Real OpenSSH adoption | 8 passed, direct/bastion/unpinned/chains on desktop and phone |
| Administrator/member browser | 1 passed |
| Real OpenSSH enrollment/runtime (Task 02) | 4 distinct cases passed, including bastion retry after fixture networking repair |
| Web typecheck, formatting, changed-file lint, localization | Passed |
| Public docs | 63 validator tests and 54 pages passed |
| Specification checks | 36 tests, 393 decisions and 1674 specs passed; lint and delivery coverage passed |
| Disposable environment cleanup | Daemon, fixtures, data root, bridge and owned firewall rules removed |

The eight real adoption cases passed after fixing an ambiguous test locator.
The 43 final browser cases include mocked UI flows and real SSH flows; these
are distinct forms of evidence. Work-order Results preserve the RED/GREEN
failures, exact scope, commands, receipts and environment repairs. No broad
backend audit, PR review, CI remediation, or macOS acceptance is claimed.

Desktop and phone overview/access screenshots were visually checked and retained
under `/tmp/ssh-ux-evidence/context-{desktop,phone}/`. Main port 9998 was not
modified. The earlier manual lab was observed stopped; refreshing it remains
outside this package. Changes remain local and uncommitted. The pre-existing
PR conflict state was not addressed or rechecked.

## Companion records and public docs

The original SSH package and navigation package remain completed historical
implementation records. This package supersedes their presentation, not their
verified security work. Keep their results; add links to this revision.

The implementation updates the managed SSH how-to in `docs/public/executors.md`
and verifies its existing coverage mapping. `README.md` and `docs/screenshots.md` have no
managed-SSH flow instructions to rewrite. `/docs-maintainer` applies to the
implementation. No PR body/comment, screenshot upload, or lab refresh now.

## Risks

- Tab force-mounting can retain secrets unless active-state cleanup is explicit.
- Simplification can obscure changed-host or uncertain-installation warnings.
- Changed composition can lose adoption/identity/bastion/executor return state.
- Historical success or duplicate names can mislead without current-stamp and
  endpoint context. No auto-deduplication or optimistic ready state is allowed.
- PR #4778 has existing conflicts with main. Resolving those is separate from
  this design package and may require a later artifact/source reconciliation.

## Implementation file manifest

Paths changed by this package relative to its recorded baseline:

```text
apps/web/app/settings/executors/page.tsx
apps/web/components/settings/ssh-connections/access-flow.test.tsx
apps/web/components/settings/ssh-connections/connection-access.tsx
apps/web/components/settings/ssh-connections/connection-collection.tsx
apps/web/components/settings/ssh-connections/connection-details.test.tsx
apps/web/components/settings/ssh-connections/connection-details.tsx
apps/web/components/settings/ssh-connections/connection-fields.tsx
apps/web/components/settings/ssh-connections/connection-overview.tsx
apps/web/components/settings/ssh-connections/connection-presentation.test.ts
apps/web/components/settings/ssh-connections/connection-presentation.ts
apps/web/components/settings/ssh-connections/connection-trust.tsx
apps/web/components/settings/ssh-connections/enrollment-panel.tsx
apps/web/components/settings/ssh-connections/identity-details.tsx
apps/web/components/settings/ssh-connections/manager-detail.tsx
apps/web/components/settings/ssh-connections/manager-header.tsx
apps/web/components/settings/ssh-connections/public-key-details.tsx
apps/web/components/settings/ssh-connections/resource-actions.tsx
apps/web/components/settings/ssh-connections/settings.test.tsx
apps/web/components/settings/ssh-connections/settings.tsx
apps/web/components/settings/ssh-connections/use-manager-navigation.ts
apps/web/e2e/helpers/ssh-access-setup.ts
apps/web/e2e/helpers/ssh-context-setup.ts
apps/web/e2e/helpers/ssh-enrollment.ts
apps/web/e2e/helpers/ssh-identities.ts
apps/web/e2e/helpers/ssh-identity-dependencies.ts
apps/web/e2e/helpers/ssh-management-layout.ts
apps/web/e2e/helpers/ssh-navigation.ts
apps/web/e2e/tests/auth/ssh-connections-permissions.spec.ts
apps/web/e2e/tests/settings/mobile-ssh-access-setup.spec.ts
apps/web/e2e/tests/settings/mobile-ssh-connections.spec.ts
apps/web/e2e/tests/settings/mobile-ssh-management-layout.spec.ts
apps/web/e2e/tests/settings/ssh-access-setup.spec.ts
apps/web/e2e/tests/settings/ssh-connections.spec.ts
apps/web/e2e/tests/settings/ssh-management-layout.spec.ts
apps/web/e2e/tests/ssh/managed-adoption-browser.spec.ts
apps/web/e2e/tests/ssh/managed-connections.spec.ts
apps/web/hooks/domains/settings/use-ssh-authentication.ts
apps/web/hooks/domains/settings/use-ssh-connection-draft.ts
apps/web/hooks/domains/settings/use-ssh-connections.test.tsx
apps/web/hooks/domains/settings/use-ssh-connections.ts
apps/web/lib/settings/ssh-connection-settings-routes.test.ts
apps/web/lib/settings/ssh-connection-settings-routes.ts
apps/web/src/locales/en/settings.json
apps/web/src/locales/ja/settings.json
apps/web/src/locales/ko/settings.json
apps/web/src/locales/pseudo/settings.json
apps/web/src/locales/pt-pt/settings.json
apps/web/src/locales/zh-cn/settings.json
apps/web/src/locales/zh-hk/settings.json
apps/web/src/locales/zh-tw/settings.json
docs/plans/ssh-connection-manager/plan.md
docs/plans/ssh-connections-executor-navigation/plan.md
docs/plans/ssh-connections-ux/plan.md
docs/plans/ssh-connections-ux/task-01-resource-views.md
docs/plans/ssh-connections-ux/task-02-access-setup.md
docs/plans/ssh-connections-ux/task-03-contextual-integration.md
docs/public/executors.md
docs/specs/executors/requirements/ssh-connection-manager.md
docs/specs/executors/system-design/ssh-connection-management-ui.md
docs/specs/executors/system-design/ssh-connection-manager.md
```
