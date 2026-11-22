---
created: 2026-11-15
status: implemented
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-002
  - REQ-EXECUTORS-SSH-CONNECTIONS-003
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
system_design:
  - ../../specs/executors/system-design/ssh-connection-management-ui.md
  - ../../specs/executors/system-design/ssh-connection-manager.md
legacy_specs: []
---

# Implementation Plan: SSH access experience

## Overview

Give enrollment and diagnostics the same deliberate hierarchy as Kandev's
settings. Replace repeated form sections and small confirmation popovers with
one focused task. Turn raw diagnostic bullets into useful recovery guidance.
Implement Access/trust first, installation second, diagnostics third, sidebar access fourth. Each
work order delivers a usable vertical slice. Execute sequentially in the
primary session; this package does not authorize delegation.

The user rejected the quality of all enrollment and diagnostic steps on
2026-11-15. This is a new interaction revision of the completed
[UX package](../ssh-connections-ux/plan.md), not a restart of SSH implementation.
Preserve the completed [discovery/status package](../ssh-discovery-status/plan.md)
and its uncommitted source changes. Current branch is
`feature/recover-and-resume-s-1mt`, local merge `1967906b5b0b73a71e5a1bd88ab314d48b1d56a0`.

## Evidence and assumption check

Confirmed: improve enrollment and diagnostics; match Kandev; preserve the
Executors location and green verified status. Verified through source and live
inspection: duplicate enrollment headings, competing access actions, dense
confirmation popovers, and a button/bullet-only diagnostic page.

Browser inspection used the isolated lab on port 43735, desktop 1440x1000 and
Pixel 5. Captured current Access, host observation, enrollment, review, and
missing-agent diagnostics. Only observe/preflight were submitted. No trust or
enrollment was confirmed. The fake password was never submitted. The user's
existing practice connection was already trusted when inspected; do not reset
its records or claim it is still an untouched untrusted fixture.
Screenshots and read-only capture script are under
`/tmp/ssh3735-experience-20261115/`, with a durable copy under the lab's
`artifacts/experience-design-20261115/`. They show current UI, not mockups.
No runtime restart, source deployment, remote authorization, or main-instance
change occurred. Main :9998 is excluded from this work.

Source anchors: `connection-access.tsx`, `connection-trust.tsx`,
`enrollment-panel.tsx`, `prerequisite-panel.tsx`, `confirmation.tsx`,
`settings.tsx`, `use-ssh-connections.ts`, `use-ssh-authentication.ts`,
`internal/sshconnections/preflight.go`, and `probe_result.go`.
The local preflight reports failures and saved readiness; it does not report
individual passing checks or stream network progress.

Agent design choices within the requested scope: a bounded reading column,
inline final reviews, visible method choices, and issue-specific remedies.
The user approved implementation in a later “go for it” turn. No unresolved
ownership or security choice blocks the package. Existing accepted custody/trust/authorization
contracts remain authoritative. No ADR or backend change is required.

## Scope

### In scope

- Focused access state, host inspection/review, clear next action and local feedback.
- Password, existing-identity and manual key installation, complete inline review,
  honest pending/result states, secret cleanup and uncertainty recovery.
- Diagnostic scope, request errors, actual issue groups, actionable remedies,
  separate SSH login testing, and existing bastion navigation.
- Phone, keyboard, touch, light/dark/pseudo, localization and public setup guidance.
- A direct SSH connections child at the start of the Executors sidebar branch.

### Out of scope

New APIs, streamed check progress, storage/migrations, automatic credential
repair, live SSH changes, replacing identity custody, executor-readiness checks,
redesigning the rest of Settings, new top-level navigation sections, CI/review remediation,
commits, pushes, or deployment to the user's lab without a later request.

## Technical approach

Keep `useSSHConnections` as the command owner and preserve its synchronous
admission guard. Add narrowly scoped operation metadata and a safe error
projection only as needed for local feedback. Retain global handling for initial
load and unrelated operations. One request must not produce both a local error
and the detached global Working/Reload block.

Compose local `access-progress`, `host-trust-review`, `enrollment-methods`,
`enrollment-review`, `enrollment-result`, and `diagnostic-issue` components as
needed, reusing shared Settings components. These names are proposed files,
not claims that they exist. Keep existing domain methods, route helpers and
security guards. No new global wizard or generalized diagnostic framework.
Use pure projections for active stage, outcome and remedy selection. Do not
persist a step or put credentials into navigation state.

An inline review replaces the anchored confirmation for trust and enrollment.
Only its final action submits. Keep deletion's existing confirmation unchanged.
Copy the *safety contracts*, not the old small-popover geometry: target/stamp
invalidation, explicit submit, one admission, secret clearing, focus return,
and responsive-boundary invalidation. Preserve context on remedy links.

| Path | Presentation | Required evidence |
| --- | --- | --- |
| Existing working identity | Test directly; installation optional | No enrollment request from opening/testing |
| Password installation | Select method, credential, review, result | Exactly one explicit request; secrets cleared |
| Existing identity | Same review with identity label | Current allowed identity retained; no new eligibility rule |
| Manual installation | Public key and named destination guidance | Exact copy; no automatic install or success |
| Bastion | Affected hop and target remain explicit | Separate trust, bootstrap only at destination |
| Missing file/agent/managed key | Specific recovery guidance | No replacement or alternate credential fallback |

## UI previews

Structured previews honor the task's rich-output/no-ASCII instruction. These
are interaction proposals, not rendered screenshots. The rich-output preview
in chat supplies the review surface. Tables specify order and hierarchy; exact
copy is illustrative and must be localized. AC-004.15 through 004.19 apply.

### UI-01: Access and host trust

Entry: saved connection, Access tab. The existing detail header stays in place.

| Order | Desktop | Phone |
| --- | --- | --- |
| 1 | Compact Details / Server trust / SSH access milestones | Current milestone title, completed-prerequisite summary |
| 2 | One bounded task surface: title, short explanation, affected endpoint | Same focused content across available width |
| 3, before inspection | Inspect host key as main action | Full-width Inspect host key |
| 3, observed | Labelled account and server fingerprint; visible comparison guidance | Vertically stacked full values; exact Copy |
| 3, changed key | Warning, saved fingerprint, observed fingerprint together | Same visible warning and evidence, no tooltip |
| 4 | Back, Trust this server; no second confirmation overlay | Primary full-width trust action, quiet Back |
| Ready variant | Green SSH verified, Test SSH access; secondary Host details/Install | One result, primary full-width test, secondary links |

Opening the page does not inspect, authenticate, trust or install. Running state
belongs to the current task. Failure retains context and gives an appropriate
retry/recovery action. The settings page owns scrolling in both layouts.

### UI-02: Optional installation

Entry: explicit Install public key from Access. It replaces Access's ordinary
controls. Header/tabs remain. The step indicator describes Choose access,
Review, Result; it does not animate remote progress.

| View | Content hierarchy | Main action | Phone difference |
| --- | --- | --- | --- |
| Choose access | Title, destination account, three radio rows: Password / Existing identity / Install manually; only selected fields | Review installation, disabled until complete | Full-width choice rows and action, no horizontal stepper |
| Manual | Named account, required guidance, public key disclosure and exact Copy | Test SSH access | Wrapped public key and full-width action |
| Review | Account; optional bastion; server fingerprint; identity and key fingerprint; method; exact effect | Install public key | Labels above full values; review is page content |
| Running | One indicator and Installing key and checking access; no credential controls | No duplicate submit | Same content, one live announcement |
| Verified | Check icon, Access verified, short outcome | Test SSH access | Full-width action; no persistent credential form |
| Failed | Failure title, specific safe cause, affected hop | Correct method/credentials and review again | Focused recovery with no stacked alert boxes |
| Installed, unverified | Warning: installed, access not verified | Test SSH access | Warning stays visible above action |
| Unknown | Warning: key may be installed | Test SSH access | No success styling or default reinstall |

Review has Back; changing/leaving review clears the password. The result stays
in the main content. Background completion never moves another view's focus.

### UI-03: Diagnostic report

Entry: saved connection, Diagnostics tab. No command runs on entry.

| State | Main surface | Action |
| --- | --- | --- |
| Initial | Check SSH setup; one-sentence scope; Not checked in this view | Check setup |
| Running | One local indicator; Checking setup | Duplicate command disabled |
| Request failed | Could not complete setup checks; safe cause | Retry |
| Issues | Summary count, Destination/Bastion groups, icon + issue title + explanation + remedy per row | Remedy beside the issue; Check again |
| No issues | No setup issues found; explicit statement that this was not a live SSH test | Test SSH access |
| Live test | Separate network-test pending/result, current stamp and safe hop-specific failure | Test again or open Access |

Example issue: **SSH agent unavailable**. Explanation: Kandev cannot reach the
agent socket on the backend host. Remedy: **Open identity**, with **Edit
connection** to choose a different identity. Do not suggest a nonexistent agent
start/unlock button. Technical details are optional, errors and remedies are not.

Desktop uses one grouped report, with actions after each explanation. Phone
uses the same order, full-width actions and stacked hop/endpoint context. No
individual success rows appear unless the API supplies that evidence. A blank
error list does not create green SSH verified status.

### UI-04: Direct sidebar entry

User addition: SSH management must be directly available within the Executors
branch shown in the supplied screenshot. This extends AC-004.6 and replaces
the previous design's exclusion of a secondary sidebar entry.

| Order | Navigation row | Behavior |
| --- | --- | --- |
| Parent | Executors with existing profile count | Existing expand/collapse and parent navigation |
| First child | Key icon, SSH connections | Direct manager link, no expansion chevron |
| Remaining children | Local, Worktree, Local Docker, other executor entries | Existing order, links and profile children |

The SSH leaf remains available with no profiles. It is highlighted throughout
connection and identity views. Existing active-path rules reveal its parent
on direct navigation without replacing saved navigation preferences. It does
not add to the profile count. Feature-disabled and non-admin views omit it.

Desktop accordion/persistent modes and the phone Settings index reuse the same
SettingsTree node, existing keyboard behavior and touch targets. Flat mode
remains flat: the full-width entry on Executors remains its direct discovery
path. No new top-level item, duplicate collection, SSH polling, or SSH count.
Existing Executors content card remains available in every mode.

Implementation: `buildExecutorsBranch` gains an explicit optional eligibility
input for one static node. `useSettingsMenuBranches` derives it from the same
feature/admin hooks as the manager. Reuse `SSH_CONNECTIONS_SETTINGS_HREF`,
`settings:sshManager.title`, `SettingsMenuNodeRow` and active-path expansion.
Inspect shared hook dependencies to ensure role/flag changes remove the node.

### Shared visual and mobile contract

Use the existing Settings title/body/meta styles, `max-w-3xl` reading width,
16px surface padding, 24px semantic spacing, one surface border and row dividers.
No new global tokens or exaggerated desktop control heights. Desktop controls
are 28px; phone/coarse targets are at least 44px. Native settings shell remains
the sole page scroller. Temporary identity selection uses its existing drawer.
Dense review uses direct page content. No nested vertical scrollers or fixed
footer covering the phone keyboard. Safe-area padding clears final actions.

Nearest exemplars: `editors-settings.tsx` and shared `SettingsGroup`; diagnostic
icon/title vocabulary from `kubernetes-diagnostics-card.tsx`; direct phone
navigation from `kanban-with-preview.tsx`; temporary selection geometry from
`mobile-menu-sheet.tsx`. Share all operation logic, vary progress composition.
Use reduced-motion-safe status feedback, stable accessible command names,
localized live regions and explicit focus restoration.

## Tests

TDD covers new state and operation logic, not class-string assertions. Preserve
existing unit tests for stamp changes, failure precedence, one-submit guards,
request shape, secret clearing and focus. Add state/feedback/diagnostic cases
alongside the existing SSH component tests. Proposed exact test files and
required behavior are listed in each work order. UI-only markup is proved by
rendered interactions, geometry and visual inspection.

## E2E tests

| Scenario | ACs | File/project |
| --- | --- | --- |
| Trust, changed key, expired receipt, no implicit request | 002.2, 002.3, 004.15, 004.16 | settings/ssh-access-setup and mobile counterpart; chromium/mobile-chrome |
| Three methods, review, one-submit, result states, secret lifecycle | 003.1-003.6, 004.16, 004.17 | same access specs plus real ssh/managed-enrollment and managed-connections; containers |
| Scope, request error, zero issues, multi-hop/unknown issues, remedies | 004.18 | new settings/ssh-diagnostics and mobile counterpart |
| Focus, geometry, long/pseudo, light/dark, reduced motion, 767/768 and coarse tablet | 004.12, 004.19 | affected desktop/phone specs plus management-layout |
| Gate regression and continuation | 001.5, 004.7, 005.3 | existing permissions, navigation and profile-link coverage when shared helpers change |

Capture each materially different stage, including pending/failed/unknown, in
light and dark. Inspect screenshots at desktop and phone; do not accept passing
visibility checks as proof of visual quality. Assert real 28px/44px geometry and
parent containment, no horizontal overflow, keyboard focus, and full-value copy.
Use fresh production assets and disposable fixtures. No tests mutate the manual
lab or main instance. The real-container runner uses a separate isolated daemon.

## Work orders

- [x] [Task 01: Focused access and host review](task-01-access-and-trust.md)
- [x] [Task 02: Guided optional installation](task-02-installation.md)
- [x] [Task 03: Actionable diagnostic report](task-03-diagnostics.md)
- [x] [Task 04: Direct sidebar entry](task-04-sidebar-entry.md)

Task 02 depends on 01. Task 03 depends on 01 and 02 because it shares feedback,
Access destinations, and final wording. Task 04 has no hard dependency, but follows these tasks in
the default sequential queue. Keep previous completed package records
historical; update their successor links without changing old test receipts.

## Verification results

Implementation and scoped verification completed on 2026-11-15. See
[the verification record](verification.md) for current receipts, fixture repairs,
visual artifacts and limits. The design-phase results below are historical.

Design validation on 2026-11-15:
catalog validated 449 decisions and 1982 specifications; 36 specification-linter
tests passed; full specification lint passed. Relative links and whitespace
passed, including untracked work orders. Local `validateCoverage` accepted all
three work orders and both linked designs using the prospective Access source
path as the coverage trigger. It performed no GitHub writes. Its initial check
caught a draft subsection heading that detached AC-004.15-19 from REQ-004;
the heading was corrected and the final preflight passed.

The proposed test paths and current source symbols were checked against the
workspace. UI captures cover desktop and Pixel 5, but are baseline evidence.
No production, permanent-test, locale, or public-doc edits were made in this
design turn. Existing uncommitted UI and documentation changes are preserved.
Current-browser captures establish baseline presentation only. They do not
validate the proposed UI or replace future disposable enrollment tests.

## Risks

- Inline review must preserve confirmation invalidation and secret cleanup.
- Global feedback changes must not hide initial-load or unrelated errors.
- Preflight and live tests offer different evidence; attractive checklists can lie.
- Agent/file problems originate on the backend host, not necessarily the user's phone.
- Unknown enrollment is not permission to reinstall, rollback or show success.


Sidebar addition (2026-11-15): the user explicitly requested the nested manager
entry. The package now contains four work orders. Earlier three-work-order
validation above describes the package before this addition.

Sidebar addition validation passed: catalog, full specification lint, whitespace
and local traceability preflight for all four work orders. No production or
permanent-test files changed in this addition.
