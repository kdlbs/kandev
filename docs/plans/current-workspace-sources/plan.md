---
created: 2026-09-15
status: completed
requirements:
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-005
  - REQ-TASKS-ATTACH-WORKSPACE-SOURCES-006
system_design:
  - ../../specs/tasks/system-design/current-workspace-sources.md
legacy_specs: []
---

# Implementation plan: Add sources to the current workspace

## Overview

Move source attachment into the Files + menu, then support adding sources to established Local folder/scratch workspaces and remote scratch workspaces.
The package was implemented from planning baseline `35eb5ab2b` after the previous agent's remediation commits.
The task system owns the root and attachment contract, including all desktop and phone outcomes.

## Scope

In scope: + menu relocation; zero-repository admission; Local folder/repository/mixed additions; unchanged-CWD persistence and rescan; remote clone capability checks; clear folder limits; translated previews and errors.
Out of scope: host-to-remote folder mounts/sync, a new upload system, active-turn attachment, executor switching, root expansion, native-session replacement, and source detachment.
Remote folder handling deliberately retains the existing rule: host folders cannot be live-linked remotely. Existing Upload folder is a separate copy workflow. This is a conservative scope choice, not a claim that remote paths are host paths.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/current-workspace-sources.md) and [requirements](../../specs/tasks/requirements/attach-workspace-sources.md).
1. Move attachment into `CreateMenu` in `file-browser-toolbar.tsx`, retaining file/upload behavior and focus restoration through FilesPanel and TaskFilesPanel.
2. Extend service admission, preview, typed root/placement-base persistence, host materialization, launch/reuse, and projections together. Do not merely delete the empty-repository guard.
3. Exercise the same contract through remote environment materialization. Capabilities must reflect the actual provider, and filesystem rollback must cover later inventory failures.

The implementation removes the repository-count gate from both frontend availability and backend admission. Re-read live source before editing because sibling implementation commits may advance.
The earlier [placement package](../workspace-repository-placement/plan.md) retains Tasks 04/05 and their expansion recovery gate. This package does not depend on root adoption and must not clear that gate.

## ASCII UI preview

### UI-01: Files toolbar, desktop

Current source and supplied screenshot place attachment under overflow. Move it to +.

```text
Before: Files   /workspace/...       [+] [...] [Search]
                                          Add repositories
                                          Open workspace folder

After:  Files   /workspace/...       [+] [...] [Search]
                                    | New file
                                    | Upload files
                                    | Upload folder
                                    | ---------------------------
                                    | Add repositories or folders
                                         [...] Open workspace folder
```

Keep all existing creation/upload actions. If uploads are unsupported, omit those items, but + remains a menu when attachment is available.
Maps to AC-005.1/.2/.3 (all AC abbreviations in previews use the full REQ prefix in frontmatter).

### UI-02: Local folder or scratch, desktop

Entry: Files > + > Add repositories or folders. The current root may have zero repositories.

```text
+------------------------------------------------------------+
| Add repositories or folders                            [X] |
| Workspace: /home/me/research   Executor: Local               |
| [+ Repository v]  [+ Folder]                               |
|                                                            |
| Repository  [payments-api v]  Branch [main v]           [x] |
| Folder      [/home/me/reference          ] [Browse]     [x] |
|                                                            |
| Where should the sources go?                               |
| (*) Directly inside the current folder                     |
|     Short paths. Adds entries beside your existing files.   |
| ( ) Inside ./kandev/                                        |
|     Groups sources together. Adds one path level.           |
|                                                            |
| Result                                                     |
| research/                         Agent CWD stays here     |
|   notes.md                        Existing file            |
|   payments-api/                   Repository               |
|   reference/ -> /home/me/reference Live folder link         |
|                                                            |
| Session and running processes stay unchanged.               |
| Folder edits affect the original. Agent access rules apply. |
| Parent folder instructions can apply to added sources.      |
|                                        [Cancel] [Add sources]|
+------------------------------------------------------------+
```

Preview uses exact server paths and actual materialization semantics: a Local repository link is labelled as a link too. Names above are illustrative.
Scratch shows its existing scratch path instead of research. No repository prerequisite, Git initialization, or root-expansion selector.
Maps to AC-006.1/.2/.3/.4/.8. Keep the earlier Worktree placement cards for Worktree tasks.

### UI-03: Remote/container workspace and unavailable states

```text
+------------------------------------------------------------+
| Add repositories or folders                            [X] |
| Workspace: /workspace/task-123   Executor: SSH (build-host)  |
| [+ Repository v]  [+ Folder: unavailable]                   |
| Host folders cannot be linked into this executor.          |
| Upload folder in + copies files when uploads are supported. |
|                                                            |
| Repository [payments-api v]   Branch [main v]                |
| Destination: /workspace/task-123/payments-api/               |
| Cloned on build-host. Current directory stays unchanged.    |
| Session and running processes stay unchanged.               |
|                                        [Cancel] [Add sources]|
+------------------------------------------------------------+

Loading:      Checking workspace capabilities... [Add disabled]
Disconnected: Reconnect the executor to add sources. [Retry]
Unprepared:   Start the task to prepare its workspace.
Busy:         Wait for the active turn to finish.
Collision:    reference already exists. Choose another name.
Stale:        Workspace changed. Review updated destinations.
Failed:       Could not add sources. [Retry] (rows preserved)
Submitting:   Adding sources... (prevent duplicate submission)
```

The actual executor name, supported clone path, and error are server-derived. Do not show a host folder browser remotely or promise that a local-only repository is cloneable.
Maps to AC-005.2 and AC-006.5/.6/.7/.8. Failure must not silently select another placement.

### UI-04: Phone

```text
Files   /workspace/...   [+] [...]
                         | New file
                         | Upload files
                         | Upload folder
                         | Add repositories or folders

+----------------------------------+
| Add repositories or folders  [X] | fixed header
| Local / research                 |
|----------------------------------|
| [+ Repository v] [+ Folder]      | one scroll body
| [payments-api v]                 |
| Branch [main v]                  |
| [/home/me/reference] [Browse]    |
|                                  |
| (*) Current folder               |
|     Short paths; more entries.   |
| ( ) Inside ./kandev/             |
|     Grouped; longer paths.       |
|                                  |
| Result and unchanged CWD         |
| Folder links edit original files.|
| Access rules still apply.        |
|----------------------------------|
| [Cancel]          [Add sources]  | fixed safe-area footer
+----------------------------------+
```

Use the existing full-height source drawer for the form, not a compressed desktop dialog. Dynamic viewport height, vertical-only content, wrapping paths, visible disabled reasons, >=44px touch targets, and no page overflow are required. Return focus to + after close. Remote phones use UI-03 capabilities in this same composition.
Maps to AC-005.3 and AC-006.1/.5/.6/.7. The menu/dialog transition must not steal focus or close the new surface.

## Tests

Work orders list exact commands and proposed test names. Map AC-005 to toolbar, availability, and dialog focus tests. Map AC-006.1/.2/.3/.4/.8 to service/materializer/reuse tests, including real Git staging protection; map AC-006.5/.6/.7/.8 to executor capability, remote materializer, and failure-compensation tests.
Keep existing tests for Worktree placement and explicit native continuation boundaries.

## E2E tests

- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts`: + entry, new-file/upload regression, Local user folder and scratch, folder-only/mixed/first repository, resume and Changes. AC-005 and AC-006.1-.4/.7/.8.
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts`: same outcomes, phone drawer geometry and focus, error retry and long paths. AC-005.3 and AC-006.1/.2/.7.
- `apps/web/e2e/tests/docker/add-workspace-sources.spec.ts` and `tests/ssh/add-workspace-sources.spec.ts`: scratch first repository, executor-side destination, host-folder reason, disconnect/collision rollback, resume. AC-006.5-.8. Use `containers` project and existing service fixtures.
- Kubernetes and Sprites use their provider materializer integration coverage plus the shared remote contract. Never claim provider parity from enum membership or a mock alone; record unavailable provider execution explicitly.

## Work orders

- [x] [Task 01: Move source attachment to +](task-01-plus-menu.md)
- [x] [Task 02: Grow Local folder and scratch workspaces](task-02-local-cwd.md)
- [x] [Task 03: Support remote scratch attachment](task-03-remote-cwd.md)

Execute sequentially. Each order includes its UI and regression evidence; no separate generic QA work order.

## Verification results

Implementation checks passed for the changed backend packages, focused frontend tests, desktop and mobile attachment flows, Docker and SSH remote flows, frontend typecheck, lint, i18n checks and ratchet, Vite build, changed E2E-file lint, specification validation, public-doc validation, backend lint/build, and diff checks. The Docker and SSH E2E runs are live provider evidence; Kubernetes and Sprites share the remote materializer contract but were not run with live infrastructure. The earlier workspace-repository-placement package still retains its separate explicit expansion and recovery gate on PR #3598.

## Risks

- Linking an existing source is not physical sandbox containment. Show this clearly and preserve the established link contract.
- A user-owned root cannot inherit a recursive cleanup policy from owned scratch/task directories.
- First-repository launch paths can mistakenly switch root based on source count. Cover restart and additional sessions explicitly.
- Remote RPC success followed by database failure requires executor-side compensation, not only row deletion.
- Existing upload/source E2E selectors may assume the old overflow entry; update affected callers together.
