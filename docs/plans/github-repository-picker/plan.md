---
created: 2026-11-11
status: in_progress
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001
system_design:
  - ../../specs/integrations/system-design/github-repository-picker.md
legacy_specs: []
---

# Implementation Plan: GitHub Repository Picker

## Overview

Move GitHub watch filters and the workspace GitHub repository scope onto the
task-creation dialog's remote repository picker, restricted to the GitHub
provider. The shared panel is extracted first without behavior change; the
watch filter and the scope setting then adopt it through one GitHub
multi-select; a focused E2E pass proves the desktop and phone flows.

- Requirement: `REQ-INTEGRATIONS-GITHUB-REPOSITORY-PICKER-001`
  ([requirements](../../specs/integrations/requirements/github-repository-picker.md))
- System design:
  [github-repository-picker.md](../../specs/integrations/system-design/github-repository-picker.md)
- No schema or persisted-shape change. Task 05 adds the optional
  `refresh=true` parameter to `GET /api/v1/github/repos`.

## Root cause

`RepoFilterSelector`'s `RepoSearchCombobox`
(`apps/web/components/github/repo-filter-selector.tsx`) derives an org from
the text before the first `/` and calls `searchOrgRepos` only when that org is
non-empty. Opening the picker therefore lists nothing, typing `kandev` lists
nothing (only the "Type owner/repo" hint), and results appear only after an
exact `owner/` prefix. The scope setting
(`github-repo-scope-section.tsx`) is a free-text comma-separated field with no
lookup at all. The task-creation remote picker instead loads
`useRemoteRepositories` once and filters client-side by substring.

Reproduction: Settings → Automations → New → add "PR: opened" → turn off "All
repositories" → Add repository → popover lists nothing; typing `kandev` shows
the hint, not `kdlbs/kandev`.

Second cause found during implementation: `github-pr-config.tsx` did not pass
`workspaceId` to `RepoFilterSelector`, so the automation "PR: opened" picker
never queried GitHub even for an exact `owner/` prefix.

Regression test that fails first:
`apps/web/components/github/repo-filter-selector.test.tsx` — "lists accessible
repositories on open and matches a bare repository name" (task 02).

## ASCII UI preview

### UI-01: Watch filter repositories (automation trigger, review/issue watch)

Entry point: automation "When" → GitHub PR trigger card, or GitHub review/issue
watch dialog; "All repositories" off.

Before:

```text
Repositories
Which allowed GitHub repositories to monitor for this watch.
( ) All repositories allowed by this workspace
Organizations  [kdlbs] [acme]
[+ Add repository]
+-----------------------------+
| (search) owner/repo         |
|                             |   <- empty until "owner/" is typed
+-----------------------------+
```

After:

```text
Repositories
Which allowed GitHub repositories to monitor for this watch.
( ) All repositories allowed by this workspace
Organizations  [kdlbs] [acme]
[kdlbs/kandev x] [acme/*  x]  [+ Add repository]
                 +--------------------------------------+
                 | Search repositories or paste a URL   |
                 |--------------------------------------|
                 | (gh) kdlbs/kandev        [check]     |  <- already added
                 | (gh) kdlbs/kandev-plugin-template    |
                 | (gh) acme/api              Private   |
                 | ...                       (scrolls)  |
                 +--------------------------------------+
```

- Structural requirements: list loads on open; substring search; selected
  entries as removable chips before the add control; "Already added" marker;
  org badges and toggle unchanged. Visual details are illustrative and reuse the
  task-creation remote picker.
- Phone: same composition; chips wrap; popover width is
  `min(380px, 100vw - 2rem)` and options/remove/add controls are 44px tall.
- Maps to AC-…-001.1–.5, .7, .8.

### UI-02: Repository scope setting, `repos` mode

Entry point: Settings → Workspace → Integrations → GitHub → Repository scope.

Before:

```text
Mode [Selected repositories v]   Organizations [kdlbs, example-org      ]
                                 Repositories  [kdlbs/kandev, example/api]
```

After:

```text
Mode [Selected repositories v]   Organizations [kdlbs, example-org      ]
                                 Repositories
                                 [kdlbs/kandev x] [example/api x] [+ Add repository]
```

- The popover is UI-01's panel without scope filtering. Disabled when mode is
  not `repos`, matching today's disabled text field.
- Phone: settings card stacks into one column as today; chips wrap.
- Maps to AC-…-001.6, .8.

## Work orders

| # | Work order | Depends on | ACs |
| --- | --- | --- | --- |
| 01 | [Shared remote repository picker panel](task-01-shared-picker-panel.md) | — | .1, .2, .7 |
| 02 | [Watch filter uses the GitHub picker](task-02-watch-filter-picker.md) | 01 | .1–.5, .7, .9 |
| 03 | [Repository scope uses the GitHub picker](task-03-scope-setting-picker.md) | 02 | .6, .9 |
| 04 | [E2E coverage, desktop and phone](task-04-e2e.md) | 03 | .1–.8 |
| 05 | [Organization picker and list refresh](task-05-organization-picker-refresh.md) | 04 | .5, .10, .11 |

Sequential. No delegation is authorized by this plan.

### Review status

Task 03 remains blocked: the ordinary repository endpoint applies the saved
scope, so the scope editor cannot browse additional repositories. It needs an
authorized discovery source that preserves personal and automation access
checks without changing the ordinary endpoint's scope enforcement.

Task 05 includes a maintainer correction for unknown-host URL entry and touch
target sizes. Its new unit and E2E coverage is not executed because the user
excludes builds, tests, linters, and other validation commands for this review.
The earlier Results sections record the contributor's previous verification.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/remote-repository-picker.test.tsx components/task-create-dialog-remote-repo-chip.test.tsx components/github/repo-filter-selector.test.tsx components/github/github-repo-scope-section.test.tsx lib/github/repository-input.test.ts hooks/domains/integrations/use-remote-repositories.test.tsx
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm e2e:run -- tests/automations-github-repository-picker.spec.ts tests/mobile-automations-github-repository-picker.spec.ts tests/integrations/github-workspace-settings.spec.ts tests/task/create-task-remote-repo.spec.ts
make fmt && make typecheck test lint
```

## Risks

- Accessible-repo list is capped at 100 by the backend; typed `owner/name`
  entry covers the rest (AC-…-001.4).
- Extracting the panel touches the task-creation remote chip; its existing unit
  and E2E suites must stay green unchanged.
- New user-facing copy needs all seven locales (`pnpm run i18n:check`).

## Exclusions

- Workspace repository chips ("Repository access") and task creation behavior
  (task creation only inherits the refresh control).
- Backend `/api/v1/github/repos/search` endpoint (left in place).
