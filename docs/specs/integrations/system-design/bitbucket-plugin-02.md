---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001
created: 2026-07-31
owners:
  - kandev
---
# Bitbucket Connector Plugin System Design Part 2

## Purpose and boundaries

This design preserves the technical source detail for `REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-INTEGRATIONS-BITBUCKET-PLUGIN-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

- **GIVEN** the independently released Bitbucket UI bundle, **WHEN** it compiles or
  launches native task creation, **THEN** it consumes `@kandev/plugin-sdk` and typed
  `host.context` reads without copied host interfaces or private Zustand slice parsing.
- **GIVEN** Bitbucket is active and a user pastes a URL owned by GitHub, GitLab, or
  another self-hosted provider, **WHEN** Kandev probes registered repository providers,
  **THEN** Bitbucket's workspace-scoped structured inspector returns no match and does
  not steal the URL by hostname/path heuristics or registration order.

- **GIVEN** a connected Cloud or Data Center workspace, **WHEN** a user selects a
  Bitbucket repository in native task creation, **THEN** Kandev persists the complete
  plugin-inspected descriptor and exact credential-free clone URL without host-side
  Bitbucket URL parsing.
- **GIVEN** two Data Center connections share one authority but use different context
  roots, **WHEN** either imports the same project/repository slug, **THEN** Kandev keeps
  distinct rows and managed clones using the plugin's connection scope and immutable
  repository ID; it never adopts the other root or a legacy unscoped row.
- **GIVEN** a persisted Bitbucket repository is materialized for a task session,
  **WHEN** Kandev requests the initial HTTPS clone credential, **THEN** the plugin
  receives the host-derived workspace, task, session, repository, exact-host, and
  exact-path scope, and missing or mismatched scope fails before Git runs.
- **GIVEN** that managed checkout already exists and pull-before-worktree is enabled,
  **WHEN** a task session launches, **THEN** Kandev refreshes origin through the same
  exact plugin credential scope and performs no unauthenticated follow-up fetch or pull.
- **GIVEN** an eligible task, **WHEN** a user opens a desktop task context/dropdown or
  visible mobile task action menu, **THEN** **Link → Bitbucket Pull Request** is
  represented by a **Bitbucket Pull Request** child item and invokes the plugin with
  verified current context.
- **GIVEN** that item is selected, **WHEN** the user enters an invalid or valid Bitbucket
  pull-request reference, **THEN** the host renders GitHub-parity dialog geometry and
  footer behavior, keeps validation errors inline, and on success closes the dialog and
  shows a success toast after the authenticated task-scoped link completes.
- **GIVEN** a Bitbucket pull request linked to a task, **WHEN** the user opens review
  on desktop or mobile, **THEN** the native review surface selects a normalized
  Bitbucket item and mounts the host-owned change-request detail anatomy used by GitHub,
  with Bitbucket data and only the actions its adapter declares.
- **GIVEN** the plugin is disabled after a review panel, task action, or repository
  provider was registered, **WHEN** Kandev refreshes the plugin registry, **THEN**
  those registrations disappear, in-flight work is aborted, review selections close
  safely, and no host Bitbucket fallback remains.
- **GIVEN** the plugin is active, **WHEN** a user opens the native integrations index
  or a workspace settings tree, **THEN** Bitbucket appears with its plugin-owned brand icon and
  native settings page, receives the selected workspace ID, and disappears from both
  surfaces when the plugin unloads.
- **GIVEN** a connected Bitbucket workspace, **WHEN** the user opens `/bitbucket`,
  **THEN** the page uses the same list-first hierarchy, density, toolbar geometry,
  row anatomy, and restrained loading/error/empty treatment as `/github` and
  `/gitlab`; the host renders shared primitives rather than plugin-cloned markup.
- **GIVEN** more than one provider page of repositories or pull requests, **WHEN** the
  user searches the repository filter or changes result pages, **THEN** the dashboard
  consumes opaque cursors, exposes every repository, and keeps a deterministic
  provider-neutral queue order without truncating before pagination.
- **GIVEN** a pull request in that list, **WHEN** the user opens its **Task** menu,
  **THEN** the same compact preset menu as GitHub/GitLab appears and the chosen preset
  opens Kandev's native task dialog directly with repository, source branch, title,
  and prompt prefilled; the three preset rows use the exact native eye, message, and
  tool glyphs.
- **GIVEN** a Bitbucket task is created or already linked, **WHEN** the user returns to
  the dashboard or opens task Review, **THEN** the row exposes the native linked-task
  indicator and review opens through the registered task review provider; there is no
  dashboard Review button or intermediate launch dialog on desktop or mobile.
- **GIVEN** a linked Bitbucket pull request has normalized task status, **WHEN** its
  task-list indicator renders or its lazy desktop refresh completes, **THEN** the glyph
  uses the same failure, pending, review-wait, passing, merged, and muted color hierarchy
  as first-party code hosts without accepting provider CSS or Bitbucket-specific tones.
- **GIVEN** a committed custom query or repository filter differs from a built-in
  preset, **WHEN** the user saves and names it, **THEN** it appears in the shared saved
  menu, survives reload for that user/workspace, restores the displayed query and
  repository filter when selected, and can be deleted.
- **GIVEN** a pull request repository is not yet persisted in the workspace, **WHEN**
  the native task dialog submits or retries, **THEN** the authenticated plugin action
  creates from the server-resolved source repository exactly once for that dialog
  launch; opening the Task menu again may create another linked task.
- **GIVEN** that Bitbucket repository is persisted, **WHEN** the native task dialog
  loads its branch picker, **THEN** Kandev invokes the active Bitbucket plugin with the
  stored provider descriptor and renders its branches without a GitHub request, a 500,
  or a local-clone fallback.
- **GIVEN** a `#` Bitbucket pull-request result was selected, **WHEN** the message is
  submitted after the plugin was disabled, moved workspaces, or access changed,
  **THEN** submission reauthorization rejects it and no unapproved reference metadata
  reaches the queued message.
- **GIVEN** a watch finds the same pull request concurrently or restarts during task
  creation, **WHEN** recovery runs, **THEN** the durable reservation and plugin-owned
  task metadata result in at most one created task; reset/delete previews and removes
  only that owned tree, and the recovered task remains associated with the pull request
  in dashboard, Review, and status queries.
- **GIVEN** a task creates a Bitbucket pull request successfully, **WHEN** the result is
  returned or the task is reopened, **THEN** the new pull request is already associated
  with that task; a later association-state failure is reported without inviting a
  retry that could create a duplicate remote pull request.
- **GIVEN** an eligible Bitbucket task has commits and no linked pull request, **WHEN**
  the user submits native **Create PR**, **THEN** Kandev pushes the selected checkout
  branch, verifies the selected session/repository worktree and passes its server-derived
  head branch to the active provider, associates the created pull request, and reports a
  post-push create failure as retryable without trusting a browser source branch or
  adding Bitbucket logic to `agentctl`.
- **GIVEN** one or more Bitbucket pull requests are linked to a task, **WHEN** association
  data loads or one link is removed, **THEN** the native task-row/card glyph count and
  shared unlink controls update reactively on desktop and mobile while other links
  remain intact.
- **GIVEN** an agent or user creates an open Bitbucket pull request externally from a
  task's checkout branch, **WHEN** that task refreshes, **THEN** the plugin discovers
  the exact source-branch match from host-verified repository data and links it without
  a manual URL paste.
- **GIVEN** a linked pull request has source-commit pipeline/build statuses, **WHEN**
  the task topbar control or composer CI chip is opened on desktop or mobile, **THEN**
  the exact shared GitHub-grade status surface shows those checks, refreshes on open,
  and can open the registered Bitbucket Review panel without a provider-specific route.

## Out of scope

- Bitbucket Issues, Cloud app passwords, and a generic host Bitbucket API client.
- Bitbucket-specific changes in `agentctl` pull-request automation or a new
  provider-specific WebSocket contract.
- External Bitbucket webhooks for watches; authenticated polling is v1 behavior.
- Pretending Cloud Pipelines and Data Center build statuses have identical APIs.
- A Bitbucket-specific signing verifier or trust-root policy. The initial release uses
  the current checksum-verified, explicitly unsigned plugin contract.
- Marketplace publication before required host contracts are released, package,
  desktop/mobile, and container credential-flow acceptance passes.

## Implementation plan

[Bitbucket plugin implementation plan](../../../plans/bitbucket-plugin/plan.md)
