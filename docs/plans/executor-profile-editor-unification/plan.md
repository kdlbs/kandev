---
created: 2026-09-17
status: draft
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Unified executor profile editor

## Overview

Repair [issue #3740](https://github.com/kdlbs/kandev/issues/3740) by routing all
profile entry points to the existing complete editor. One sequential work
order covers route generation, bookmark compatibility, and regression tests.

The [requirement](../../specs/executors/requirements/profile-editor.md) and
[design](../../specs/executors/system-design/profile-editor.md) are draft
artifacts for review. Implementation remains pending.

## Evidence and root cause

Investigation used commit `f32eb373781bcec12583c37e4a9d52b1ee12072f`, also the
local `origin/main` reference. The issue contained no comments or attachments.

The hub hardcodes the plural route. `executorProfileSettingsPath` generates an
executor-scoped singular route for every type except `k8s`.
`renderExecutorSettingsRoute` then mounts `LegacyExecutorSettingsRoute`, which
renders a separate reduced form for non-Kubernetes profiles.

A read-only Node execution of the actual helper produced:

| Type | Destination for `exec-1` / `profile-1` |
| --- | --- |
| local, worktree, local_docker, remote_docker, ssh, sprites | `/settings/executor/exec-1/profile/profile-1` |
| k8s | `/settings/executors/profile-1` |

A source trace found `DockerSections`, `SSHAgentReadinessCard`,
`SSHTaskDirReclamationCard`, `KubernetesProfileSections`, `SpritesSections`,
and `McpPolicyCard` only in the complete editor. Existing component tests
explicitly expect the reduced SSH route, so passing tests currently preserve
the defect.

Smallest user reproduction: open a saved Local Docker profile from the hub,
then from the desktop settings tree. Compare the URL and the Dockerfile,
image-tag, and build controls. This investigation used executable route and
source evidence, not a live browser reproduction.

The issue's remote-Docker connection-card claim is not a requirement to invent
new controls. This repair preserves the complete editor's current capabilities
and existing executor connection pages.

## Scope

### In scope

- Canonical profile URLs from every production entry point.
- Valid legacy bookmark replacement and invalid-pair recovery.
- One editor implementation with existing type-specific controls and save behavior.
- Desktop and phone regression coverage.

### Out of scope

- Runtime execution, new controls, connection redesign, backend changes, and migrations.
- Generic settings redesign or unrelated form cleanup.

## Technical approach

Change `executorProfileSettingsPath` to accept one profile ID. Migrate all
existing callers and hardcoded production profile destinations. Keep executor
connection helpers unchanged.

Handle explicit profile IDs first in `LegacyExecutorSettingsRoute`. Validate
executor/profile ownership before redirecting. Preserve executor-only behavior
and the unavailable-profile state. Remove the reduced editor's form logic.
Retain a minimal old-page wrapper if needed for existing file references.

Reuse the complete editor and `SettingsRedirect`. Preserve discovery fragments,
encoded identifiers, shared save coordination, and existing role gates.

No existing executor requirement explicitly covers entry-point parity. The new
vertical requirement belongs to executors, not the reusable UI system. The
existing card-spacing contract remains a dependency. No new architectural
ownership boundary or persistence model requires an ADR.

## ASCII UI preview

### UI-01: Saved Docker profile, desktop

Entry: hub card, settings tree, executor profile list, task disclosure, or bookmark.

```text
BEFORE: sidebar                    AFTER: every entry point
+-----------------------+          +----------+---------------------------+
| Profile details       |          | Settings | Profile name              |
| Environment variables |          | tree     | Profile details           |
| Prepare / cleanup     |          |          | Dockerfile + image tag    |
| No Docker build card  |          |          | [Build Image]             |
+-----------------------+          |          | Applicable credentials    |
                                   |          | Environment / scripts     |
                                   |          | MCP policy                |
                                   +----------+---------------------------+
                                              [Save changes] when dirty
```

### UI-02: Saved Docker profile, phone

Entry: Settings index > Executors > profile card, or a valid bookmark.

```text
+-----------------------------+
| Settings > Executors        |
| Profile name                |
| Profile details             |
| Dockerfile + image tag      |
| [Build Image]               |
| Applicable credentials      |
| Environment / scripts      |
| MCP policy                  |
|                             |
| [Save changes] when dirty   |
+-----------------------------+
```

The phone opens a full settings destination without a desktop sidebar. Both
views retain one content scroll region and the shared floating save control.
Cards and labels are illustrative. Existing card order, type applicability,
localized copy, and spacing remain authoritative. Controls stay inside the
viewport, and the save control clears the safe area.

### UI-03: Invalid legacy bookmark

```text
+-----------------------------+
| Profile not found           |
| [Back to executor]          |
+-----------------------------+
```

Use the existing localized recovery control. Missing executors can recover to
the hub. No editor or save control mounts. Views map to AC-001.1 through
AC-001.6 under `AC-EXECUTORS-PROFILE-EDITOR`.

## Tests

The work order lists exact paths and commands. Required regression cases:

| Criteria | Evidence |
| --- | --- |
| .1, .7 | Helper unit tests, sidebar/profile-list tests, discovery and route tests |
| .2, .5 | Desktop Docker edit/build/save/reload plus existing executor-specific E2E |
| .3, .4 | Legacy route component tests for valid, missing, and mismatched pairs |
| .5 | Discard, unsaved-navigation guard, and unchanged role gates |
| .6 | Phone navigation, save/reload, and viewport containment |

First failing regression: `routes every profile to the canonical editor` in
`apps/web/lib/settings/executor-settings-routes.test.ts`. A current
non-Kubernetes call returns the singular path instead of the expected plural
path. Add this regression against the current two-argument helper before
changing its signature, then migrate the test with its callers.

## E2E tests

Add `settings/executor-profile-routing.spec.ts` for the `chromium` project.
Open one disposable Docker profile through hub, sidebar, profile list, and
legacy bookmark. Assert the same ID and Docker controls. Mock one successful
build, save an image-tag edit, reload, and verify persistence. Exercise discard
and unsaved navigation. Cover task-disclosure navigation with its existing
seed/store fixture pattern.

Add `settings/mobile-executor-profile-routing.spec.ts` for `mobile-chrome`.
Tap the hub profile card, edit, save, reload, and open the legacy bookmark.
Assert complete controls, safe save reachability, and no horizontal overflow.
Use disposable records and clean them in `finally`.

Existing Docker persistence, Kubernetes settings, SSH connection-link, and
executor-agent-config suites provide the type-specific and permission checks.
No Docker daemon is required for these settings tests.

## Work orders

- [ ] [Task 01: Unify profile navigation and bookmark handling](task-01-unify-profile-editor.md)

## Verification results

Design validation on 2026-09-17:

- `python3 scripts/list-docs.py validate`: passed (285 decisions, 988 specifications).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/executor-profile-editor-unification`: passed.
- `git status --short -- docs/plans/executor-profile-editor-unification`: both new plan files accounted for.
- Baseline Vitest run: 43 tests passed across `legacy-executor-settings-route.test.tsx`,
  `executor-profiles-card.test.tsx`, and `settings-menu-branches.test.ts`.
- Read-only helper execution and section source trace reproduced the route split.
- Issue assignment: `carlosflorencio`.

Implementation and browser checks: not run. No production or permanent test
files changed during investigation.

## Risks

- Unconditional redirects can open a profile owned by another executor.
- Removing the reduced form changes its header and delete-return destination
  to the canonical editor's existing behavior.
- Removing the old file can break guard inventories or imports. Prefer a
  minimal compatibility wrapper when the repository expects the path.
- Role-gated controls and discovery fragments must survive route convergence.
- Browser coverage must activate navigation controls, not only visit URLs.
