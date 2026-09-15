---
created: 2026-09-15
status: implemented
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-006
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
legacy_specs: []
---

# Implementation Plan: Executor-aware workspace sources

## Overview

Separate host-folder eligibility from in-place repository execution, then make
local-origin repositories usable by remote executors with explicit clone semantics.
Two sequential work orders deliver the executor-aware source policy and the
server-authoritative remote-origin selection flow. Both work orders are
implemented and verified in the current branch.

[Requirements](../../specs/tasks/requirements/mixed-repository-selection.md),
requirement 006, and the [design](../../specs/tasks/system-design/mixed-repository-selection.md)
are authoritative. Tasks owns this extension to its existing creation contract.

## Confirmed intent and decisions

- Worktree allows folder browsing; committing a folder-only selection changes to
  Local with a visible notice. Mixed repo/folder input lets the user select Local
  or Worktree. Worktree isolates repository changes; folder changes remain live.
- Remote executors disable folders. Local repositories remain selectable when
  their origin can be cloned, with `Clone from remote` visible. Local uncommitted
  changes and unpushed commits are excluded; unusable origins are disabled.
- Manual executor changes preserve rows and show incompatibility, blocking Start.
- Draft-only switch provenance prevents add-order differences without overriding
  later explicit choices. This is an implementation decision to satisfy the approved
  order-independent flow; it does not override fresh-dialog executor-default policy.
- Existing repository-count, provider identity, credentials, and runtime limits stay.

## Scope

### In scope

- Accurate folder capability state, Local/Worktree transitions and helper text.
- Server-owned local origin inspection, remote branch selection and clone intent.
- HTTP/WS preflight, serializer and last-used adapters, sets, editable subtasks.
- Desktop/phone parity, error recovery, localized copy and public instructions.

### Out of scope

- Folder upload/sync to remote environments, pushing local work, arbitrary remote
  selection beyond the repository's origin, new executors or expanded repo limits.
- Redesigning running-task attachments, provider readiness or portable defaults.
- Automatically changing an executor merely because a user browses a picker.

## Technical approach

Task 01 adds a shared source-policy derivation and coordinated draft transition.
Do not broaden `useIsLocalExecutor`, which drives checkout semantics. Wire the policy
to both source-menu consumers, submission validation, hints, and existing executor
selection policy. Preserve branch intent and the same-sheet phone presentation.

Task 02 adds read-only server inspection for local repository origins and remote refs.
The current discovery/status shapes do not supply verified origin information.
Implement the proposed inspection HTTP/WS operation and optional `checkout_source`
intent documented in the design. Validate against the actual origin again on create;
never turn a client-supplied URL or host path into remote authority. Reuse provider
resolution and runtime clone credentials. Keep original checkout provenance for mode
reversal, while remote execution receives the validated remote descriptor.

Inspection has cancellation/generation guards and bounded concurrency. Test remote
branch loss, origin replacement, unsupported schemes and missing access. Browser
readiness is not proof that a remote runtime can reach the origin; actual clone
failure keeps the existing recoverable launch behavior. No new database table is
planned; extend existing JSON preference/source adapters compatibly.

## ASCII UI preview

### UI-01: Worktree plus folder (desktop)

Entry: Worktree selected, open Add. Local Folder is enabled.

```text
[Repo: api | from: main v | x]   [+ Add v]
                                +----------------------+
                                | Repository         > |
                                | Local Folder       > |
                                | Repository Set     > |
                                +----------------------+

After choosing a folder:
[Repo: api | from: main v | x]
[Folder: ~/design-assets | x]    [+ Add]
[Prompt...]
[Agent v] [Workflow v] [Executor: Worktree v]
A worktree will be created from main.
The selected folder will be used directly.       [Start task]
```

Maps to 006.1-006.3, 006.9. Worktree stays selected because a repository exists.
Choosing Local through the existing executor selector changes the bottom helper
to direct checkout/folder use. No second executor selector is added.

### UI-02: Folder-only adjustment and explicit override

Entry: empty draft with Worktree selected; browse a folder and commit it.

```text
[Folder: ~/research | x] [+ Add]
Switched to Local for folder-only work.
[Prompt...]
[Agent v] [Workflow v] [Executor: Local v]
The selected folder will be used directly.       [Start task]
```

Maps to 006.1, 006.3, 006.7. Browse/cancel does not switch. Adding a repository
restores the previous Worktree choice only if the user has not since made an
explicit executor choice. Removing all items restores the longer Add label and
bottom-only scratch explanation from UI-03 of the predecessor plan. An unavailable
Local profile retains input and shows recovery; it does not claim a successful switch.

### UI-03: Remote executor and local-origin choices

Entry: SSH selected, Add menu, then Repository > Local.

```text
[Executor: SSH v]
[+ Add Repository/Folder v]
+------------------------------------------------------+
| Repository                                         > |
| Local Folder                              [disabled] |
| Local folders require a Local or Worktree executor.   |
| Repository Set                                     > |
+------------------------------------------------------+

+------------------------------------------------------+
| < Back / Repository                                  |
| Local | GitHub | Bitbucket | Azure                    |
| ------                                               |
| [Search repositories...                            ] |
|------------------------------------------------------|
| api                      Clone from remote           |
| github.com/acme/api                                  |
|                                                      |
| offline-project                           [disabled] |
| No usable remote origin.                             |
|                                                      |
| another-project                           [checking] |
| Checking remote origin...                            |
+------------------------------------------------------+

After selecting api:
[Repo: api | Clone from remote | main v | x] [+ Add]
[Prompt...]
[Agent v] [Workflow v] [Executor: SSH v]
Repositories will be cloned from their remote origins.
Uncommitted changes and unpushed commits are not included.
                                         [Start task]
```

Maps to 006.4-006.6, 006.8-006.9. Provider tabs still require configured/enabled/tested
integrations. The Local tab names where the candidate was found; the explicit
label identifies how it will run. Unsupported sources are disabled, not hidden.
Search/header remain fixed; results alone scroll. No local-only branch auto-fallback.

### UI-04: Retained incompatible selections

Entry: switch a mixed host workspace to SSH, or retain an unpushed branch.

```text
[Repo: api | Clone from remote | feature/local-only v | x]
This branch is not available remotely. [Choose remote branch]
[Folder: ~/design-assets | x]  [+ Add]
Local folders require a Local or Worktree executor.
[Executor: SSH v]                         [Start: disabled]
```

Maps to 006.6-006.8. Errors are per row. Switching back to a supported host executor
restores valid checkout behavior. Removing incompatible rows or selecting a remote
branch enables Start when all other validation passes. Origin changes show a
refresh-required row error; access failures show Retry/settings without losing the
selection. These are resolved error states, never indefinite executor waiting text.

### UI-05: Phone source navigation and recovery

Entry: phone New Task, then Add under the chosen executor.

```text
+--------------------------------------+
| [Repo: api                        x] |
| [Clone from remote | main v]         |
| [+ Add]                             |
| [Prompt...                        ] |
| [Agent v] [Workflow v] [SSH v]       |
| Clone remote contents only.         |
| Local uncommitted/unpushed work     |
| is not included.                    |
| [Start task]                        |
+--------------------------------------+

+--------------------------------------+
| Add to workspace                 x  |
| Repository                       >  |
| Local Folder             [disabled] |
| Requires Local or Worktree.         |
| Repository Set                   >  |
+--------------------------------------+

+--------------------------------------+
| < Back / Repository                 |
| Local | GitHub | ...                |
| [Search...                        ] |
|--------------------------------------|
| api                                |
| Clone from remote                  |
| github.com/acme/api                 |
|                                    |
| offline-project         [disabled] |
| No usable remote origin.           |
+--------------------------------------+
```

Maps to 006.9 plus 006.1-006.8. With Worktree, the same folder option is enabled
and navigates within this sheet to the folder picker. Reuse MobilePickerSheet:
inset bottom drawer, fixed navigation/search, one vertical results scroller,
dynamic viewport sizing and safe-area clearance. Back replaces the sheet body;
selection returns focus to Add. No stacked popovers. Recovery is visible beside
rows, not tooltip-only. At least 44px touch targets; no page horizontal overflow.
Desktop menus use the surrounding 12px text and normal compact controls. Copy in
these sketches is illustrative except approved labels; all copy uses five locales.

Menu order, exact contextual Add labels, clone label, bottom hint location, and
phone navigation/scroll ownership are structural. Names and spacing are illustrative.
The existing empty-state and compact-typography fixes must remain intact.

## Tests

All new filenames/test names below are planned outputs. AC suffixes refer to
`AC-TASKS-MIXED-REPOSITORIES-006`.

| AC | Evidence |
| --- | --- |
| .1-.3, .7, .10 | New web `components/task-create-dialog-executor-source-policy.test.ts`: folder-first/repo-first; cancel; remove-final-repo; explicit override; no Local profile; locked subtask; sets; late hydration |
| .1-.4, .9 | Existing `components/task-create-dialog-options.test.tsx` plus new `components/task-create-dialog-executor-source-menu.test.tsx`: real resolved Worktree props, enabled folder option, known unsupported vs loading, composed hints |
| .5-.8 | New service `service_repository_clone_source_test.go`: saved/discovered origin, no origin, SSH/HTTPS support, credential URL rejection, cross-workspace ID, changed origin, remote-only branches |
| .6-.8, .10 | HTTP/WS handler tests and existing executor workspace-source tests: new intent parity, forged folders/locators, authoritative descriptor passed to clone, no host-path/file-copy fallback, legacy input |
| .5-.10 | New web `hooks/domains/repositories/use-repository-clone-source.test.tsx`: stale workspace/mode/origin results, cancellation, retry; serializer/defaults tests retain source intent and branch settings |

## E2E tests

The existing desktop and mobile workspace-content, mixed-repository, and mixed-subtask
specs provide the browser coverage for UI-01, UI-02, UI-04, UI-05, ordered sources,
empty/cancel behavior, source-sheet navigation, and set/subtask compatibility. The
remote-origin readiness, stale-result fencing, branch filtering, and selected-row
reinspection paths are covered by the focused web tests and backend service/handler
tests listed below. The real-container clone transport case remains a follow-up
because the standard E2E fixture exposes an intentionally ineligible `file://` origin
and does not provide a reachable credentialed Git origin. It must be added when that
fixture is available; its absence is recorded rather than represented as a passing
transport test.

Use guarded runners, rebuild backend artifacts before browser verification, and run
projects sequentially. The affected browser runs completed without retries or flakes.

## Work orders

- [x] [Task 01: Host folder policy and executor transitions](task-01-host-folder-policy.md)
- [x] [Task 02: Clone local repository origins remotely](task-02-remote-origin-selection.md)

## Companion packages

The workspace-contents creation plan is complete. Its results remain historical.
This package extends it and supersedes its blanket no-auto-switch rule only for
folder-only adjustments. The mixed-repository plan and attachment package remain
predecessors; preserve provider readiness, server inspection and owned-root cleanup.
The task-create executor-default requirement still governs the next fresh dialog.
ADRs 0028/0041 keep preference authority on the backend; runtime source attachment
ownership remains under the existing workspace-source ADR. No new ownership ADR
is required for these additive, explicitly documented creation contracts.

## Verification results

Design validation on 2026-09-15: catalog (267 decisions, 878 specifications),
full specification lint, work-order reference/link checks, and scoped whitespace
checks passed. Implementation verification on 2026-09-15:

- Web task-create unit suite: 55 files and 706 tests passed.
- Web typecheck, lint, i18n checks, production E2E build, and backend lint/build passed.
- Task service tests passed with `go test -tags fts5 ./internal/task/service`.
- Task handler and dto tests passed with `go test -tags fts5 ./internal/task/handlers ./internal/task/dto`.
- Desktop browser checks passed: workspace contents 3/3, mixed repositories 2/2,
  and mixed subtasks 1/1.
- Mobile browser checks passed: workspace contents 1/1, mixed repositories 1/1,
  and mixed subtasks 1/1.
- Public documentation and specification validation remain part of the final gate.

The real-container clone transport test is a documented verification gap until a
reachable credentialed Git fixture exists. No commit or push is implied by this
document; delivery is handled by the implementation workflow.

## Risks

- Broadening the existing isLocalExecutor boolean would corrupt branch semantics.
- Draft auto-switch effects can fight manual selection or asynchronous restoration.
- A known clone URL does not prove runtime network access or credentials.
- Reusing host branch lists can select unpushed refs unavailable to remote clones.
- A remote-origin switch must not silently target a changed origin or leak credentials.
