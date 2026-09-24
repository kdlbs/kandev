---
created: 2026-09-12
status: completed
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-001
  - REQ-TASKS-MIXED-REPOSITORIES-002
  - REQ-TASKS-MIXED-REPOSITORIES-003
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
legacy_specs: []
---

# Implementation Plan: Mixed Repository Selection

## Overview

Replace task-wide source modes with an ordered mixed repository draft.
Add Local and eligible provider tabs inside the selector. Deliver provider
readiness first, then the complete New Task path, then shared consumers and Sets.
All work orders are sequential. This package does not authorize delegation.

Requirements: [Mixed repository selection](../../specs/tasks/requirements/mixed-repository-selection.md).
Design: [Mixed repository selection](../../specs/tasks/system-design/mixed-repository-selection.md).

## Scope

### In scope

- Configured, enabled, successfully tested provider eligibility.
- Named tabs above search, stable selections, and mixed per-row submission.
- Desktop and phone creation, provider presets, New Subtask, and existing Sets.
- Empty task and folder compatibility, URL entry, failure recovery, localization.
- An additive plugin readiness callback, host lifecycle support, and fixture evidence.

### Out of scope

- New providers, executor capabilities, or attachment editing on running tasks.
- Arbitrary URL persistence in repository sets and automatic checkout substitution.
- Quick Chat redesign, new auth storage, or a new connection-testing service.
- Production plugin changes inside the monorepo, publishing, and releases.

## Technical approach

1. Extend `useRemoteRepositories` with a readiness catalog independent of list
   results. Combine existing enabled state with verified authentication.
   Add the proposed `RepositoryProviderRegistration.getAvailability` callback
   and wrap it in `registry-provider-lifecycle.ts`.
2. Introduce the ordered `TaskRepositorySelection` union and reducer. Migrate
   `RepoChipsRow`, picker rendering, state hydration, and `buildRepositoriesPayload`.
   Reuse branch and remote inspection helpers. Maintain stable row identities.
3. Map each row into existing `CreateTaskParams.repositories` and
   `TaskRepositoryInput`. Prove mixed input through current server preflight.
4. Migrate New Subtask, presets, Sets, empty/folder rendering, and source-mode
   tests. Update affected specifications and public instructions with behavior.

No database migration or task-create API change is planned. The SDK addition is
optional for binary/source compatibility, but missing readiness hides a plugin
from the new picker. Older plugins must adopt it before their tabs can appear.
This is a visible rollout dependency, not proof that old plugins are disconnected.

Production adoption must happen in each provider's dedicated repository. Before
release, record the provider manifest ID, repository URL, accepted plugin commit,
minimum host version, and configured/disabled/failed/passed connection evidence.
The fixture proves host behavior only. Do not declare production Bitbucket
compatibility based on the fixture. No persistent follow-up tasks are created.

## ASCII UI preview

### UI-01: Desktop mixed draft and Add repository

Entry: New Task, two selected repositories, Add repository open.

```text
[ kandev | Local  | from: origin/main v | x ]
[ plugin | GitHub | from: main v        | x ]  [ + Add ] [ Sets v ]

+---------------------------------------------------------+
| Local | GitHub | Bitbucket | Azure DevOps                |
|         ======                                          |
| Search GitHub repositories...                       [R] |
+---------------------------------------------------------+
| acme/plugin                                             |
| acme/design-system                                      |
| acme/api                                                |
+---------------------------------------------------------+
| Paste repository URL                                    |
+---------------------------------------------------------+
```

Tabs precede search. Names remain visible, including when the strip scrolls.
The header and URL action are fixed. Only results scroll vertically.
`R` illustrates the existing refresh icon. Providers shown are examples of
eligible integrations, not a fixed provider allowlist.
Maps to AC-TASKS-MIXED-REPOSITORIES-001.1 through 001.3 and 002.1 through 002.7.

### UI-02: Empty and failure states

Entry: New Task after removing the final repository.

```text
Repositories
No repositories attached.  [ + Add ] [ Sets v ]
Starting folder (optional): [ Choose folder ]
                                          [ Start task ]
```

The folder action appears only with its existing local-executor eligibility.
A removed last row is not restored by asynchronous defaults.

Entry: a selected remote provider loses its connection.

```text
[ plugin | GitHub | from: main v | x ]
Connection unavailable. [ Retry ] [ Integration settings ]
                                         [ Start: disabled ]
```

Its browse tab is absent. Local and other eligible tabs remain usable.
A list-only request failure instead keeps the eligible tab and shows Retry
inside its results. Initial status loading shows Local plus a loading notice.
An eligible empty source shows `No repositories found`, with its tab retained.
Maps to AC-TASKS-MIXED-REPOSITORIES-001.4, 001.5, 002.2, 002.3, 002.6, and 003.5.

### UI-03: Phone repository management

Entry: New Task, then tap Repositories (2).

```text
+----------------------------------+
| New task                         |
| [ Repositories (2) v ]           |
| Describe your task...            |
+----------------------------------+

+----------------------------------+
|              ----                |
| Repositories              Done   |
+----------------------------------+
| kandev     Local           [ x ] |
| from: origin/main v              |
|                                  |
| acme/plugin  GitHub        [ x ] |
| from: main v                     |
+----------------------------------+
| [ + Add repository ] [ Sets v ] |
+----------------------------------+
```

Add navigates inside the same drawer:

```text
+----------------------------------+
| < Back          Add repository   |
| Local | GitHub | Bitbucket | ... |
|         ======                   |
| Search GitHub repositories...    |
+----------------------------------+
| acme/plugin                      |
| acme/design-system               |
| acme/api                         |
+----------------------------------+
| Paste repository URL             |
+----------------------------------+
```

Use the MobilePickerSheet interaction: one drawer, fixed header, one vertical
scroll body, dynamic viewport height, and safe-area footer. Branch and URL views
replace the body and return through Back. Tab scrolling stays inside the strip.
Touch actions measure at least 44px. Desktop actions retain their normal 28px size.
Maps to AC-TASKS-MIXED-REPOSITORIES-003.3 and 003.4.

Tab position, shared selected state, source labels, navigation, and scroll
ownership are structural requirements. Example names, spacing, and icons are
illustrative. All product text must use the five locale catalogs.

## Tests

New filenames and test names below are planned. Existing files are extended
where named. Add `@covers` annotations for the listed AC mappings.

| AC suffixes (AC-TASKS-MIXED-REPOSITORIES-) | Evidence file and test name |
| --- | --- |
| 002.1, 002.2, 002.3 | `hooks/domains/integrations/use-remote-repositories.test.tsx`: `exposes only enabled verified providers, including empty results` |
| 002.4, 002.5, 002.6 | Same file: `rejects stale workspace and registration results`; new `components/task-create-dialog-repository-picker.test.tsx`: `restores eligible tabs and falls back to Local` |
| 002.2, 002.6 | `lib/plugins/registry-provider-lifecycle.test.ts`: `aborts readiness on unload and rejects old generation`; SDK contract tests |
| 001.1, 001.2, 001.4 | New `components/task-create-dialog-repository-selection.test.ts`: `preserves mixed order and sibling branches`; `keeps deliberate empty input after delayed defaults` |
| 001.1, 001.5, 003.5 | `components/task-create-dialog-helpers.multi-repo.test.ts`: `serializes local remote local with independent bases`; backend tests described below |
| 001.3, 002.7 | New picker component test: `shows named tabs above search and retains URL entry without providers` |
| 003.1 | `components/task/use-subtask-submit.test.ts`: `submits mixed sources and respects inherited restrictions` |
| 003.2 | `components/task-create-dialog-repository-sets.test.ts`: `appends set members without changing remote rows`; save-as-set tests |
| 003.3, 003.4 | New mobile Playwright scenarios in the next table |

Backend files: `internal/task/service/repository_selection_test.go` gains
`TestCreateTaskMixedRepositorySelections` and
`TestCreateTaskMixedPluginFailureLeavesNoWrites`.
`internal/task/handlers/task_http_handlers_test.go` and `task_ws_handlers_test.go`
gain mixed transport cases. Preserve preflight authority and row order.

## E2E tests

All files live under `apps/web/e2e/tests/task/`. Use the fixture plugin and seeded
repositories. Use causal HTTP/WS waits and assert backend attachment readback.
Capture desktop and phone screenshots during these runs and compare UI-01 to UI-03.

| File | Project | Scenarios and AC mapping |
| --- | --- | --- |
| `create-task-mixed-repositories.spec.ts` (new) | chromium | Local plus built-in and plugin sources, distinct branches and ordered attachment readback (001.1-001.3, 003.5); eligibility matrix and empty results (002.1-002.4); tab restoration/loss, retry, public URL entry (002.5-002.7); remove-all, folder, unsupported executor (001.4-001.5) |
| `mobile-create-task-mixed-repositories.spec.ts` (new) | mobile-chrome | Same mixed submit and connection recovery (001.1, 002.6, 003.5); one drawer, Back, branch selection, named tabs, keyboard viewport, focus, 44px geometry, no page overflow (003.3-003.4) |
| `create-task-repository-sets.spec.ts` and `mobile-create-task-repository-sets.spec.ts` | respective desktop/phone | Apply to mixed and empty drafts, repeat apply, saved bases, save exclusions (003.2) |
| `create-subtask-mixed-repositories.spec.ts` and `mobile-create-subtask-mixed-repositories.spec.ts` (new) | respective desktop/phone | Mixed editable subtask, inherited restrictions, preset restoration (003.1) |
| Existing remote/plugin/local creation specs | respective desktop/phone | Replace removed mode-switch interactions while preserving current behaviors (001.5, 002.7, 003.5) |

## Work orders

- [x] [Task 01: Provider readiness catalog](task-01-provider-readiness.md)
- [x] [Task 02: Mixed New Task selection](task-02-mixed-task-creation.md)
- [x] [Task 03: Shared consumers and documentation](task-03-consumers-and-docs.md)

## Verification results

Implementation verification on 2026-09-13:

- `python3 scripts/list-docs.py validate`: passed (265 decisions, 822 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: passed (62 tests).
- `node scripts/validate-public-docs.mjs`: passed (46 published pages).
- `git diff --check`: passed.
- `pnpm --filter @kandev/plugin-sdk test` and `typecheck`: passed.
- `pnpm run lint`: passed with zero warnings and errors.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: passed for all five catalogs and copy gates.
- `pnpm run build:e2e`: passed.
- Affected web unit tests: 17 files and 163 tests passed.
- Affected desktop E2E: 23 tests passed.
- Affected phone E2E: 35 tests passed.
- `go test -tags fts5 ./internal/task/...`: passed, including the changed task
  service and HTTP/WS handler coverage.
- `make -C apps/backend lint` and `make -C apps/backend build`: passed.
- `make -C apps/backend test` reached all packages but exited nonzero on existing
  process/config discovery, launcher, and office SQLite migration tests. The
  changed task packages passed in that run; the failures are unrelated to mixed
  repository selection and reflect shared environment state.

The host readiness callback, fixture, and SDK contract are implemented. Adoption
in production provider repositories remains a release dependency as specified;
the fixture is not used as production provider evidence. Each work order records
its implementation and verification results below.
Before the first pnpm command in a fresh worktree, run
`(cd apps && pnpm install --frozen-lockfile)` once.
E2E commands use the guarded runner and rebuild artifacts. Run projects sequentially.

## Related packages and reconciliation

- `docs/plans/plugin-repository-task-resolution/`: preserve server trust and
  preflight tests. Its existing native E2E selectors need migration in Task 02.
- `docs/plans/repository-sets/` and `docs/plans/repository-set-base-branches/`:
  preserve ID-based storage, saved bases, and completed historical results.
  Task 03 updates the owning specification's obsolete mode-visibility wording.
- Observed predecessor status: plugin resolution `implemented`, repository sets
  `completed`, and repository set bases `complete`. Their historical results remain intact.
- No active predecessor work order is claimed complete by this package.
  Recheck companion statuses when implementation starts and record any conflicts.

## Risks

- Existing provider plugins lack a readiness callback. Production adoption is
  required before claiming that their tabs work under the stricter contract.
- Repository Sets cannot save unregistered remote selections. The UI must explain
  this limit instead of silently losing them or introducing registration writes.
- Defaults, PR metadata, and policy snapshots can be lost if hydration or row
  replacement still relies on global source flags.
- A local checkout is a host path. Remote executor compatibility remains governed
  by existing materialization rules, not by the source-tab label.
- The many source-mode E2E callers must migrate with the component. Hidden stale
  tests are not adequate evidence of compatibility.

## Subsequent workspace-contents extension

The [workspace contents creation package](../workspace-contents-creation/plan.md)
extends this completed implementation. Its previews replace UI-01 through UI-03
for future task-creation work: folders coexist with repos, one Add menu handles
all sources, and the scratch hint moves to the bottom executor explanation.
This package's completed results remain historical evidence, not verification of
the new extension.
