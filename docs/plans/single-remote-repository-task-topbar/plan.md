---
created: 2026-09-24
status: complete
requirements:
  - REQ-UI-REMOTE-REPO-TOPBAR-001
system_design:
  - ../../specs/ui/system-design/single-remote-repository-task-topbar.md
legacy_specs: []
---

# Implementation Plan: Single remote repository task topbar

## Overview

Show a provider icon and short repository name before the task title when the
task has exactly one remote repository. Make the repository a direct external
link when its saved URL is safe for browser navigation. One work order carries
the derived data through both topbars and verifies the complete interaction.

## Scope

### In scope

- Desktop and phone task topbar presentation for exactly one remote repository.
- Provider icon, short name, browser link, accessible name, and safe URL
  fallback.
- Focused unit and browser coverage for eligible and ineligible cases.

### Out of scope

- Repository metadata migration or API changes.
- Task card, filter, sidebar, and multi-repository label changes.
- General redesign of task or mobile topbars.

## Technical approach

Add a topbar-only descriptor in
`apps/web/components/task/task-page-content-helpers.ts::resolveTaskProps`,
using `Task.repositories` and the resolved `Repository`. Pass it through
`task-page-inner.tsx` and `task-layout.tsx`. Keep `repositorySlug` and
`repositoryLabel` unchanged for other consumers.

`task-top-bar.tsx` uses the descriptor for its final parent crumb.
`page-topbar.tsx` renders an optional provider icon and external anchor while
retaining truncation and collapsed crumbs. Reuse
`RemoteRepositoryProviderIcon` for first-party and plugin providers.
`mobile/session-mobile-top-bar.tsx` gives the remote repository its own link
before the existing task-picker button. Add one localized accessible-name key
in all required catalogs. Put safe HTTPS-to-browser URL normalization in a
focused helper with URL and fallback tests.

## ASCII UI preview

`UI-01: Desktop task topbar`, task detail with one remote repository.
The repo link and task title are separate controls; the project ancestor, if
present, remains before the repository. Illustrative spacing only.

```text
Current: [Untrivial-ai/agent-orch...] > [Explain agent connections]
Planned: [GitHub icon agent-orchestrator] > [Explain agent connections]
                repo link                  editable task title
```

`UI-02: Phone task topbar`, same task. This is one scroll-free header row;
the task picker keeps its existing branch/diff summary on its second line.
The repository link is a separate touch target.

```text
[Back] [GitHub icon agent-...] > [Explain agent... v] [Actions]
                                      branch / diff summary
```

When the remote URL is unusable, the repository label and icon remain static.
For local, multiple, or zero repositories, the current header composition
remains. The icon, order, link, fallback, and reachable task title are
requirements (`AC-UI-REMOTE-REPO-TOPBAR-001.1` to `.5`); spacing and shown
truncation lengths are illustrative. Rendered checks cover both views.

## Tests

- `apps/web/components/task/task-page-content-helpers.test.ts` proves remote
  eligibility, short names, and unchanged labels (`AC-...001.1`, `.3`).
- A focused browser-URL helper test proves accepted HTTPS and rejected
  credential, SSH, malformed, or missing URLs (`AC-...001.2`, `.4`).
- `apps/web/components/task/task-top-bar.test.tsx` and
  `apps/web/components/task/mobile/session-mobile-top-bar-repository.test.tsx`
  prove provider icon, anchor attributes, fallback, and separate task action
  (`AC-...001.1` through `.5`).

## E2E tests

- `apps/web/e2e/tests/task/remote-repository-topbar.spec.ts` (chromium) opens
  a task created with one `repositories` entry containing an HTTPS
  `remote_url`, checks the actual browser link and short label, and confirms
  the task title remains editable (`AC-...001.1`, `.2`).
- `apps/web/e2e/tests/task/mobile-remote-repository-topbar.spec.ts`
  (mobile-chrome) checks the same destination, touch activation, picker,
  truncation, and viewport containment (`AC-...001.5`).

## Work orders

- [x] [Task 01: Render a linked single remote repository](task-01-linked-remote-repository.md)

## Verification results

Passed final frontend validation after review remediation:

- Vitest: six focused files, 102 tests passed. Coverage includes Bitbucket
  Server's unsupported browser-route fallback and plugin provider registration
  and unregistration after desktop and phone headers mount.
- `pnpm run typecheck` and `pnpm run i18n:check` passed.
- Targeted ESLint passed with no warnings.
- `pnpm run build` produced the production Vite bundle.
- Desktop and `mobile-chrome` Playwright scenarios passed. Both E2E runs built
  the backend and frontend. The Pixel 5 check verified 44px controls,
  truncation, no horizontal overflow, external navigation, and task picking;
  the mobile composition was visually inspected during the implementation run.
- Specification validation and lint passed.

## Risks

- A clone URL is not always a browser page, especially for plugin code hosts.
  Only GitHub, GitLab, and Azure DevOps are mapped directly; Bitbucket Server's
  `/scm/TEAM/fixture.git` clone route differs from its
  `/projects/TEAM/repos/fixture` browser route. Plugin providers stay static
  until their contract supplies an authoritative browser URL.
- Long repository and task names compete for the phone header. The phone
  rendered check and Playwright geometry assertions must confirm both controls
  stay usable.
