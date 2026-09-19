---
created: 2026-09-15
status: complete
requirements:
  - REQ-TASKS-REMOTE-OPTIONS-001
  - REQ-TASKS-REMOTE-OPTIONS-002
  - REQ-TASKS-REMOTE-OPTIONS-003
system_design:
  - ../../specs/tasks/system-design/remote-repository-options.md
legacy_specs: []
---

# Implementation plan: Remote repository options

## Overview

Add a gear to each remote repository chip in task creation, backed by task-owned
checkout options. Implement the contract first, host preparation second, remote
preparation third, and expose the complete responsive UI last. Work sequentially.

User-confirmed: per-remote-repository gear; settings apply only to this task.
Implemented controls: Standard/Download on demand and All/Selected folders. Standard
preserves existing behavior; on-demand is opt-in. This package does not redesign
preparation progress or remove the existing five-minute timeout.

## Scope

### In scope

- Task attachment persistence, validation, capability admission, and resume.
- Real filtered downloads and per-task sparse checkout before file population.
- Existing managed credential leases for later object reads.
- Desktop panel, phone drawer, row summaries, and localized validation.

### Out of scope

- Repository defaults, automatic directory browsing/dependency selection, shallow-depth UI, and arbitrary flags.
- Editing materialized workspaces, changes to user-owned local checkouts, and general startup timeout/progress redesign.
- Enabling options on providers or custom scripts without demonstrated support.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/remote-repository-options.md).
Add a typed metadata codec and request/response field; no SQL schema change.
Preserve `Repository.LocalPath` as shared state and carry effective partial-clone
cache paths on the task launch request. Apply sparse configuration on the task
worktree, never the shared clone. Host credentials, primary remote bootstrap,
and sibling agentctl materialization are separate paths that all need evidence.
Capability admission rejects any path that cannot enforce the requested policy.

## ASCII UI preview

### UI-01: Desktop, selected remote row and expanded settings

```text
[raycast/extensions] [main v] [gear*] [x]
  On demand, 1 folder

  Repository settings: raycast/extensions
  Applies to this task only
  Download       [Download on demand v]
  Folders        [Selected folders v]
  [extensions/my-extension                 ]
  [packages/shared                         ]
  One directory per line
  [Reset]                  [Cancel] [Apply]
```

The gear is always visible on a resolved remote row; `*` illustrates a subtle
non-default indicator. The panel is bounded and the parent dialog stays open.
Root/ancestor files are included by directory-based sparse checkout; shared
subdirectories can be added explicitly. No success claim is based on this sketch.

### UI-02: Phone, gear opens an inset bottom drawer

```text
[raycast/extensions]
[main v]              [gear*] [x]
On demand, 1 folder

+--------------------------------+
| Repository settings            |
| raycast/extensions             |
| Applies to this task only      |
|--------------------------------|
| Download                       |
| [Download on demand v]         |
| Folders                        |
| [Selected folders v]           |
| [extensions/my-extension     ] |
| One directory per line         |
| [Reset]                        |
|--------------------------------|
| [Cancel]               [Apply] |
+--------------------------------+
```

Header and safe-area footer are fixed; only the middle region scrolls. Existing
mobile picker-sheet geometry is the exemplar. Controls have 44px touch targets;
ordinary desktop controls retain 28px density. Folder edits use this same drawer,
not another stacked picker. Label width and ASCII spacing are illustrative.
Control grouping, row ownership, scroll ownership, and action order are required.

### UI-03: Validation and capability states

```text
[../outside]
Use a folder path inside this repository.
                              [Apply disabled]

Download on demand unavailable
This executor cannot fetch missing files after setup.
[Standard v]
```

Empty repository row: no gear until a repository URL is selected or submitted.
Capability loading: retain current applied settings, disable Apply for custom settings until current
identity/executor eligibility is known. Capability error: Retry without resetting
folder inputs. Changing executors does not silently clear incompatible settings.
These views cover AC-001.1 through AC-001.4, AC-002.2/002.4, and AC-003.2.

## Tests

All new/changed logic uses TDD: demonstrate the relevant failure first, implement,
then refactor and run the work order's exact checks. Targeted test suites:

| Acceptance | Evidence |
| --- | --- |
| 001.1-001.4 | desktop/phone E2E: opener, draft apply/cancel/reset, row summary; desktop/phone E2E |
| 002.1-002.4 | `repository_checkout_options_test.go`: codec, request/DTO persistence, legacy absent values, ownership and capability rejection; `task-create-dialog-checkout-options.test.ts`: identity/reset/payload |
| 003.1, 003.3 | `clone_checkout_options_test.go`: filter-capable server, omitted historical blob, later lazy fetch, authorized lazy fetch; credential lease tests cover expiry/revocation, mode-specific cache reuse |
| 003.1, 003.2, 003.4, 003.5 | `manager_checkout_options_test.go`: first checkout is sparse, two task scopes, dirty reuse, absent directories, scope mismatch |
| 002.1/002.4, 003.1-003.5 | `repository_checkout_options_test.go` in lifecycle plus `workspace_checkout_options_test.go` in agentctl API: primary/sibling propagation and remote staging |

Numbers abbreviate the `AC-TASKS-REMOTE-OPTIONS-` prefix. Full IDs live in each
work order. Tests must assert outcomes, not only generated Git argument strings.

## E2E tests

- `e2e/tests/task/remote-repository-options.spec.ts` (`chromium`): two remote rows,
  one customized, payload persisted after task creation, unaffected sibling,
  cancel/reset, invalid paths, and repeat creation defaults. Unsupported executor
  responses and stale capability requests have service/hook tests.
- `e2e/tests/task/mobile-remote-repository-options.spec.ts` (`mobile-chrome`): visible gear and drawer, Apply, focus return, bounded drawer, no document
  overflow, and measured 44px targets. A real software keyboard and explicit
  767/768px breakpoint measurements have not been exercised.
- `repository_checkout_options_container_test.go`: the generated production
  primary-clone script runs in `kandev-agent:e2e` against a disposable authenticated
  Git server. It verifies selected files, omitted historical objects, lazy reads
  after clone-helper disposal, and revocation without secrets in Git config.
  Native agentctl tests separately cover sibling materialization and reuse.
  No live Raycast clone or personal credentials are used.


## Work orders

| Order | Work order | Dependency | Status |
| --- | --- | --- | --- |
| 1 | [Task policy contract](task-01-policy-contract.md) | None | done |
| 2 | [Host preparation](task-02-host-preparation.md) | 01 | done |
| 3 | [Remote preparation](task-03-remote-preparation.md) | 02 | done |
| 4 | [Repository gear and responsive settings](task-04-repository-gear.md) | 03 | done |

## Verification results

Design validation on 2026-09-15:

- `python3 scripts/list-docs.py validate`: passed (269 decisions, 925 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- Local package link and acceptance-ID validation: passed for all seven files.
- `git diff --check -- docs/specs docs/plans/remote-repository-options`: passed.
- `git status --short -- docs/specs docs/plans/remote-repository-options`: confirmed the seven new design files are untracked and available for review.

Implementation validation on 2026-09-15:

- Targeted race tests passed across ten backend packages: clone, worktree,
  executor, credentials, lifecycle, agentctl client/API, models, service, handlers.
- Authenticated Docker primary preparation test passed with selected folders,
  omitted historical blobs, post-bootstrap lazy fetch, and revoked access.
- Frontend regression unit suites: 74 tests passed. Capability hook tests:
  stale executor responses and failed-request retry passed.
- Seven phone browser tests passed, including existing remote picker regressions,
  the new drawer, measured touch targets, focus restoration, and viewport bounds.
- All ten existing desktop remote picker tests passed. The new two-repository
  settings test passed after waiting for closing popovers to unmount. It checks
  independent row settings, persistence, invalid paths, Cancel/Reset, and fresh
  task defaults. The final phone drawer smoke test also passed on that build.
- Final frontend typecheck, focused ESLint, and i18n checks passed. Existing
  i18n orphan-key warnings remain; no new untranslated copy was introduced.
- Public docs validator and its test passed (46 pages). Specification catalog
  and all-file linter passed (269 decisions, 925 specifications).
- `git diff --check` passed. No commit or push was requested or performed.

The Docker test exercises the generated production preparation script inside a
container; it does not claim a complete live-backend Docker task launch. The real
Raycast repository has not been recloned. Individual work orders own commands.

## Risks

- Transient clone credentials alone cannot support partial clones; the later
  credential test is a release prerequisite, not an optional follow-up.
- Shared cache conversion or shared sparse patterns could affect other tasks;
  mode-specific cache paths and worktree-specific patterns prevent that coupling.
- Primary remote bootstrap and sibling materialization differ today. Capability
  advertisement must follow coverage; a successful sibling test is insufficient.
- Sparse checkout can omit build dependencies. Explain shared-folder selection
  and Git cone semantics; do not infer dependency completeness.
- Partial cloning can still be slow. The existing five-minute limit remains until
  a separate preparation-lifecycle package changes it.
