---
created: 2026-11-02
status: draft
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-003
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-004
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-005
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-006
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001
  - REQ-PLUGINS-AZURE-DEVOPS-EXTRACTION-001
  - REQ-PLUGINS-PLUGINS-001
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
system_design:
  - ../../specs/integrations/system-design/azure-devops-plugin.md
  - ../../specs/integrations/system-design/azure-devops-plugin-sprints.md
  - ../../specs/integrations/system-design/azure-devops-plugin-board-children.md
  - ../../specs/plugins/system-design/azure-devops-extraction.md
  - ../../specs/plugins/system-design/plugins-03.md
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
legacy_specs:
  - ../../specs/integrations/requirements/azure-devops-integration.md
---

# Implementation Plan: Azure DevOps Plugin Extraction

## Overview

The first-party Azure DevOps integration is replaced by the runtime plugin
`kandev-plugin-azure-devops`. As agreed with the maintainer on 2026-11-03, the
Kandev repository receives exactly two changes: one that removes the built-in
integration, and one that lists the plugin in the registry. No data is
migrated; the user reconnects after installing the plugin. The plugin also
gains Microsoft account sign-in, ported from the closed PR #4915.

Order of work:

1. **Removal, locally first (task 08 code).** The web plugin registry rejects
   `azure_devops` and `azure-devops` while core owns them, so the plugin can
   only be developed against a Kandev checkout that already has the removal.
   The removal branch lives in the task's `kdlbs-kandev` worktree and is
   rebased on `main` as needed. Its PR opens last.
2. **Plugin to parity (tasks 01 to 06)** against that checkout.
3. **Plugin packaged E2E (task 07).**
4. **Kandev PR K-A, the removal (task 08).** After it is released, the plugin
   pins `min_kandev_version` to that release and publishes `v0.1.0`.
5. **Kandev PR K-B, the registry entry and docs (task 09).**

The product keeps working at every step: nothing changes for the user until
K-A ships, and by then the plugin is tested and packaged.

## Scope

### In scope

- The connector plugin with parity to the built-in integration, Microsoft
  sign-in, and an agent pull-request tool.
- Removal of the built-in integration from Kandev, including its tables and
  credentials.
- Registry listing and public docs.

### Out of scope

- Data migration.
- New host contracts. Known differences are listed in the requirements and
  below. Possible follow-ups, with their earlier designs archived outside the
  repository at `design-archive-2026-11-03/` in the task workspace:
  - a fence so a plugin can reveal only its own secrets (security gap found in
    `internal/plugins/host.go` `RevealSecret`; to be reported to the
    maintainer separately);
  - broker addresses for local Docker and SSH without a public URL;
  - delivery-ledger links and sidebar watch classification for plugin
    providers;
  - launch delegations for plugin watches on owned executors.
- Transferring the plugin repository to `kdlbs`.

## Delivery and review gates

| Gate | Condition | Owner |
| --- | --- | --- |
| G0 Maintainer alignment | Done 2026-11-03: the maintainer asked for one removal PR and one registry PR, and confirmed no migration is needed. | done |
| G1 Removal builds | Task 08 code complete on the local branch: backend and web build, tests pass, inventory check empty. | agent |
| G2 Plugin parity | Tasks 01 to 07 pass their verification against the removal checkout, and the user validates against `dev.azure.com/contoso`. | agent, user |
| G3 Removal merged and released | K-A reviewed and merged; a Kandev release contains it. | maintainer |
| G4 Plugin released | `min_kandev_version` pinned to that release; GitHub release `v0.1.0` with package and checksums; user installs it from the package. | agent, user |
| G5 Listed | K-B merged; the registry index resolves the release. | maintainer |

Rollback before G3: nothing changed for the user. After G3: downgrade and
restore the boot backup.

### PR grouping

| PR | Work orders | Notes |
| --- | --- | --- |
| P1 plugin | 01 to 07, 10 | The whole plugin in one PR on one branch: bootstrap, connection, Microsoft sign-in, screens, tasks and links, watches, packaged E2E, and teams and sprints (added 2026-11-10 at the owner's request). The release tag follows its merge. |
| K-A `kdlbs/kandev` | 08 | Removal, one PR, breaking-change title |
| K-B `kdlbs/kandev` | 09 | Registry entry and public docs |

The owner wants three PRs in total: one in the plugin repository and two in
Kandev. Plugin PR #2 (Claude workflow hardening) predates this rule and is
merged on its own, because claude-code-action skips any PR that changes its own
workflows; folding it into P1 would stop the Claude review of P1.

One Kandev task holds both repositories as sibling worktrees under the task
directory (`kdlbs-kandev` and `JnManso-kandev-plugin-azure-devops-main`). A
`kandev` symlink to `kdlbs-kandev` in the task directory satisfies the plugin's
`../kandev/...` SDK paths.

## Technical approach

### Plugin (`JnManso/kandev-plugin-azure-devops`)

- **Bootstrap (task 01):** template identity rename; SDK import migration to
  `github.com/kdlbs/kandev/pluginsdk` and `@kdlbs/plugin-sdk` with sibling
  replacements; `recipes/source-control/` adopted as one slice; Bitbucket
  toolchain (`ui/src/*.ts` with esbuild, Vitest, Playwright packaged-host,
  `internal/` packages); template demo surfaces deleted; README, CHANGELOG,
  LICENSE, icon notice, PR template; CI/release workflows on the new SDK
  paths; the owner installs the GitHub review apps they want.
- **Connection (task 02):** port of `rest_client.go`, `client_models.go`,
  `auth.go`, `ValidateOrganizationURL`, `parseAzure*Remote`, mock client;
  `connection` state and secrets; `registerIntegrationSettings` form;
  health poll; `registerRepositoryProvider`; `GitCredentialHandler`.
- **Microsoft sign-in (task 03):** port from `7226134a2` of `entra.go`,
  `service_oauth*.go`, `oauth_application.go`, the sign-in UI hooks, and the
  concurrency tests.
- **Browse (task 04):** port of `azure-devops-page-client.tsx` and components
  onto host integration components; `registerRoute`, `registerNavItem`.
- **Tasks and links (task 05):** `TaskCreateDialog`, `registerTaskAction`,
  `registerReviewProvider`, `createChangeRequest`, agent tool,
  `EntityReferenceHandler`.
- **Watches (task 06):** port of the watch service and poller onto Host state
  and `Tasks().Create`.
- **Teams and sprints (task 10):** my-teams grouping and remembered team,
  work-item Team and Sprint filters, and a read-only Sprints taskboard, per the
  teams and sprints system design.
- **Release (task 07):** packaged E2E, release workflow, `v0.1.0`.

### Kandev

- **Removal (task 08):** the inventory in the extraction design; migration
  dropping tables and secrets; reservations released; docs and spec lifecycle;
  `rg` completion check.
- **Registry (task 09):** `plugin-registry/plugins.yaml` entry;
  `docs/public/integrations.md` pointer if not already in K-A.

### Executor compatibility matrix

| Executor | Plugin Git credentials | Requirement |
| --- | --- | --- |
| local, worktree | broker lease on loopback | none |
| local Docker, remote Docker, SSH, Kubernetes, Sprites | broker lease | `githubCredentialBroker.publicBaseUrl` (HTTPS) reachable from the executor; documented in the plugin README, as for Bitbucket |

## ASCII UI preview

Spacing is illustrative. Control order, grouping, and the primary actions are
structural. Copy is localized. Browse screens keep the built-in composition;
the built-in desktop and mobile E2E specs are their parity reference.

UI-01: Plugin settings, Microsoft account, signed in. Desktop. Settings >
Integrations > Azure DevOps (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.1 to
002.7).

```text
+--------------------------------------------------------------------------+
| Azure DevOps integration                                          [on]   |
| +----------------------------------------------------------------------+ |
| | v Authenticated . checked just now                                   | |
| | Organization URL   [https://dev.azure.com/org                     ]  | |
| | Authentication     [Microsoft account                          v]    | |
| | +------------------------------------------------------------------+ | |
| | | (v) Signed in as Ana Silva          [Sign in again] [Sign out]   | | |
| | |     Application in use: 04b0...6                                 | | |
| | | Application (client) ID [                                     ]  | | |
| | | Leave blank to use this installation's default.                  | | |
| | +------------------------------------------------------------------+ | |
| | Default project    [Platform                     v]                  | |
| | [Test connection] [Update]                                [Remove]   | |
| +----------------------------------------------------------------------+ |
| Watches: work items | pull requests                                     |
+--------------------------------------------------------------------------+
```

UI-02: The same settings on a phone. Controls stack full width
(AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-002.10).

```text
+------------------------------+
| < Integrations  Azure DevOps |
+------------------------------+
| Organization URL             |
| [https://dev.azure.com/org]  |
| Authentication               |
| [Microsoft account       v]  |
| (v) Signed in as Ana Silva   |
| [ Sign in again           ]  |
| [ Sign out                ]  |
| Application (client) ID      |
| [                         ]  |
| Default project              |
| [Platform                v]  |
| [ Update                  ]  |
| [ Test connection         ]  |
+------------------------------+
```

UI-03: Watch disabled because its profile is no longer available. Desktop and
phone share the row anatomy (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-007.3).

```text
| Work-item watch "Bugs to triage"                      [disabled] [Edit] |
| ! Executor "Ana's laptop" is no longer available to this plugin. Edit   |
|   the watch and choose another executor.                                |
```

UI-04: Sprints mode. Desktop. Browse > Sprints
(AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.1, 001.3 to 001.6).

```text
+---------------------------------------------------------------------------+
| Board | Sprints | Work items | Pull requests                              |
| [Contoso    v] [Offers         v] [Sprint 23 Current v] [Person: All v]   |
| Nov 2 - Nov 13 . 4 working days left                       [Collapse all] |
+-------------------------+------------+------------+-----------+-----------+
| Backlog item            | New        | Active     | Resolved  | Closed    |
+-------------------------+------------+------------+-----------+-----------+
| v #1001 Send customer   |            | [#1003]    |           |           |
|   Active . A. Lovelace  |            |  Map ...   |           |           |
| v #1002 Send address    | [#1004]    |            |           |           |
| > Unparented tasks (2)  |                                                 |
+-------------------------+------------+------------+-----------+-----------+

Team selector, open:            Sprint selector, open:
+---------------------------+   +-------------------------------+
| [Filter teams           ] |   | [Filter sprints             ] |
| My teams                  |   | Sprint 22             Past    |
|   Platform                |   | Sprint 23             Current |
|   Offers                v |   | Sprint 24             Future  |
| All teams                 |   +-------------------------------+
|   Payments                |
|   ...                     |
+---------------------------+
```

UI-05: Sprints mode on a phone (AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-SPRINTS-001.7).

```text
+------------------------------+
| [Offers                   v] |
| [Sprint 23 Current        v] |
| [Person: All              v] |
| Nov 2 - Nov 13 . 4 days left |
+------------------------------+
| v #1001 Send customer ..     |
|   Active (1)                 |
|   [#1003 Map the address ..] |
| > #1002 Send address ..      |
+------------------------------+
```

UI-06: Board card children. Desktop. Browse > Board, Features board
(AC-INTEGRATIONS-AZURE-DEVOPS-PLUGIN-CHILDREN-001.1 to 001.4, 001.6). The
phone shows the same card inside the column navigator (001.5).

```text
| [Contoso v] [Offers v] [Features v]                [Expand all]   |
| New                              2 | Active                         |
| +--------------------------------+ |                                |
| | :: Execution Details           | |                                |
| | #2005  Feature                 | |                                |
| | v User Story: 1/3     [+ Task] | |                                |
| |   #951201 Story  Closed  Ana   | |                                |
| |   #951202 Story  Active  Rui   | |                                |
| |   v Task: 1/2                  | |                                |
| |   | #951210 Task  Closed  Rui  | |                                |
| |   | #951211 Task  Active  Ana  | |                                |
| |   #951203 Story  New     -     | |                                |
| +--------------------------------+ |                                |
| | :: Status Workflow ...         | |                                |
| | #2006  Feature                 | |                                |
| | > User Story: 0/2     [+ Task] | |                                |
| +--------------------------------+ |                                |
```

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| PLUGIN-001.1, 001.4 | plugin `scripts/test-verify-package.sh`, `test-verify-release-version.sh`; `plugin-registry` node tests |
| PLUGIN-001.2, EXTRACTION-001.2 | `apps/web/lib/plugins/registry-provider-ownership.test.ts`, `registry.test.ts`; `orchestrator/executor/executor_resume_test.go` plugin-managed cases |
| PLUGIN-002, 003 | plugin `internal/auth/*_test.go`, `internal/plugin/connections_test.go`, concurrency tests ported from `service_oauth_concurrency_test.go`; `ui/test/*.test.ts` |
| PLUGIN-004 | plugin `ui/test/browse*.test.ts`; packaged E2E |
| PLUGIN-005 | plugin `internal/plugin/{links,review,agent_tools,references}_test.go` |
| PLUGIN-006 | plugin `internal/plugin/credentials_test.go`, `provider_test.go` |
| PLUGIN-007 | plugin `internal/watches/*_test.go` |
| PLUGIN-SPRINTS-001 | plugin `internal/azure/sprints_test.go`, `internal/plugin/actions_browse_test.go`; `ui/src/browse/sprint-board.test.ts`, `search-query.test.ts` |
| PLUGIN-CHILDREN-001 | plugin `internal/azure/children_test.go`, `rest_client_children_test.go`, `internal/plugin/browse_children_test.go`; `ui/src/browse/board-children.test.ts` |
| EXTRACTION-001.1 | removal inventory `rg` check in the K-A PR body; `go build ./...`, `pnpm run typecheck` |
| EXTRACTION-001.3 | `task/repository/sqlite/base_migrations_azure_removal_test.go` (SQLite + PostgreSQL); `storeconformance` upgrade fixture |

## E2E tests

| Flow | AC | File and project |
| --- | --- | --- |
| Connect with PAT; Microsoft sign-in via mock authority; renewal; sign-out | PLUGIN-002, 003 | plugin `ui/e2e/desktop-packaged-plugin.spec.ts`, `mobile-packaged-plugin.spec.ts` |
| Browse boards, work items, PRs; saved views | PLUGIN-004 | same files |
| Task from work item; Link; status indicators; Create PR; agent tool | PLUGIN-005 | same files |
| Repository and branch pickers; private clone on local executor | PLUGIN-006 | same files |
| Watch creates one task; disabled reason | PLUGIN-007 | same files |
| My teams first; Team and Sprint filters; Sprints taskboard, person filter, collapse, empty states | PLUGIN-SPRINTS-001 | same files |
| Board card child counts, expand and collapse, child detail and task actions, expand all remembered, unavailable notice | PLUGIN-CHILDREN-001 | same files |
| Removed routes and reservations released | EXTRACTION-001.1, 001.2 | `apps/web/e2e/tests/plugins/bitbucket-plugin-contract.spec.ts` stays green; removed Azure specs deleted |

## Work orders

- [ ] [Task 01: Plugin repository bootstrap](task-01-plugin-bootstrap.md)
- [ ] [Task 02: Connection with PAT, repositories, Git credentials](task-02-plugin-connection.md)
- [ ] [Task 03: Microsoft sign-in](task-03-plugin-microsoft-sign-in.md)
- [ ] [Task 04: Browse screens](task-04-plugin-browse-screens.md)
- [ ] [Task 05: Tasks, links, PR status, Create PR, agent tool](task-05-plugin-tasks-links.md)
- [ ] [Task 06: Watches](task-06-plugin-watches.md)
- [ ] [Task 07: Packaged E2E and release](task-07-plugin-release.md)
- [ ] [Task 08: Remove the built-in integration from Kandev](task-08-core-removal.md)
- [ ] [Task 09: Registry entry and public docs](task-09-registry-entry.md)
- [ ] [Task 10: Teams and sprints](task-10-plugin-teams-sprints.md)
- [ ] [Task 11: Board card children](task-11-plugin-board-children.md)

## Verification results

Pending.

## Risks

- **Entra redirect.** The popup return to the plugin route origin must be
  revalidated against `dev.azure.com/contoso` in task 03.
- **Removal size.** K-A deletes about 17k lines plus tests across backend and
  web. The inventory check and a green CI are the review aids; the PR body
  lists the inventory.
- **Release ordering.** `min_kandev_version` cannot be pinned until K-A is
  released. Until the registry lists the plugin, the user installs it from the
  built package.
- **Executors.** Docker, SSH, and Kubernetes need the public HTTPS broker
  address for private Azure repos, as for Bitbucket. This is documented, not
  solved, in this initiative.
- **SDK.** The sibling-checkout build depends on `.kandev-sdk-ref`; bumping it
  must be tested against the pinned host.

## Open questions

- Whether the maintainer wants the secret reveal fence reported as a separate
  security issue now. The design here does not depend on it.
