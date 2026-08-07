---
status: draft
created: 2026-08-02
owner: kandev
---

# Plugin Authoring Experience

## Why

Plugin authors and coding agents cannot currently move from discovery to a
tested package without reconciling public tutorials, frozen contract notes,
frontend types, the Go SDK, the manifest implementation, packaging code, and
ADRs. Some of those references have drifted behind the shipped contract, which
makes otherwise copy-pasteable guidance unsafe.

## What

- `docs/public/plugins-authoring.md` is the canonical developer entry point and
  is linked directly from the root, backend, and web `AGENTS.md` files.
- The entry point teaches one workflow: choose a supported recipe, edit
  `manifest.yaml`, implement against the authoritative contract, run the
  available validation, package, install, and smoke test.
- It explains the install-to-load lifecycle, package and per-plugin data layout,
  backend/UI-focused/combined plugin shapes, capability boundary, and the rule
  that plugins use Host APIs rather than Kandev databases or internal packages.
- A storage decision table distinguishes plugin-scoped Host state, the absence
  of per-user `host.storage` in this branch, operator config and vault-backed
  secrets, and files rooted under `KANDEV_PLUGIN_DATA_DIR`.
- A frontend matrix covers every registration and Host value currently exposed
  by `apps/web/lib/plugins/types.ts` and `apps/web/lib/plugins/host-api.ts`. It
  names every mounted component slot and records input, manifest requirement,
  lifecycle/cleanup, and an example or authoritative source link.
- A backend matrix covers `Plugin.OnEvent`, `Plugin.HandleWebhook`, every
  `pluginsdk.Host` method/resource accessor, the required capability, and the
  relevant SDK/proto source.
- The guide explicitly says that `registerTaskPanel`, `registerTaskMenuAction`,
  per-user `host.storage`, `RichTextEditor`, `RichTextReadOnly`, and direct
  Kanban-card injection are not part of the current branch. It does not invent
  future signatures.
- At least six current-contract recipes are runnable or link to maintained
  source: a UI-focused plugin with the required no-op managed binary; a Go
  backend using Host state; a `task-sidebar` contribution backed by task-scoped
  Host state; a webhook receiver; an event subscriber; and a Kanban-aware route
  that reads `host.store` without injecting into first-party cards.
- The reproducible workflow documents the exact checks already available in
  the plugin template and monorepo, including their limits. Package creation
  generates checksums; the install path performs manifest, checksum, archive,
  extraction, and host-binary checks; the in-tree fixture package tests provide
  the closest no-production-instance validation. This change does not add a
  second validator or new runtime behavior.
- Existing plugin pages, contract notes, relevant README-style example docs,
  and the `create-kandev-plugin` skill link back to the canonical entry point
  instead of duplicating or contradicting the API.
- The plugin system remains marked experimental unless a separate product
  stability decision changes the public status evidence.

## Authoritative contracts

This feature does not add plugin runtime hooks or Host RPCs. It documents the
current contract and points readers to:

- Frontend pair: `docs/plans/plugins/PLUGIN-API.md` and
  `apps/web/lib/plugins/types.ts`.
- Concrete frontend Host exports: `apps/web/lib/plugins/host-api.ts`.
- Backend author API: `apps/backend/pkg/pluginsdk`.
- Wire API: `apps/backend/proto/kandev/plugin/v1/plugin.proto`.
- Manifest model and semantics: `apps/backend/internal/plugins/manifest`.
- Package integrity and install semantics:
  `apps/backend/internal/plugins/pkgtar`.

The canonical guide summarizes those sources for discovery, but does not become
another type, proto, or schema definition.

## Permissions and storage

- Frontend bundles run in the Kandev origin and receive the current shared
  React instance and curated host/store surface. Frontend registrations are not
  individually capability-gated; `ui.bundle` and `ui.keybindings` declare the
  relevant UI entry points.
- Host state requires `state`; secrets require `secrets`; reads require the
  matching `api_read` resource; task/message writes require the matching
  `api_write` resource; utility-agent invocation requires `agent_invoke`; and
  event delivery requires a matching event subscription.
- `Host.GetConfig` and `Host.EmitEvent` are ungated. Webhook keys must be
  declared. The external-auth response directive additionally requires `auth`.
- Plugins do not receive sanctioned direct database access. Domain reads and
  writes go through typed Host RPCs and Kandev's service layer.
- Host state is plugin-scoped structured data and is not per-user browser
  storage. Arbitrary plugin files belong below `KANDEV_PLUGIN_DATA_DIR`.

## Scenarios

- **GIVEN** an agent starts at a relevant `AGENTS.md`, **WHEN** it follows the
  plugin-authoring link, **THEN** it reaches the canonical guide in one step and
  sees the contract map and supported workflow.
- **GIVEN** a UI-focused plugin author, **WHEN** they follow the recipe, **THEN**
  they include the no-op managed executable required by the current installer
  and a native UI bundle without inventing unsupported backend behavior.
- **GIVEN** an author needs durable task-related state, **WHEN** they consult the
  storage table and sidebar recipe, **THEN** they use task-scoped Host state and
  do not mistake it for unavailable per-user `host.storage`.
- **GIVEN** a manifest declares `api_write: ["tasks"]`, **WHEN** an author reads
  the capability matrix, **THEN** it documents `Tasks().Create/Update` as
  implemented and links to the SDK/proto.
- **GIVEN** a package whose `ui.bundle` omits the leading slash, **WHEN** an
  author follows the workflow, **THEN** the guide identifies that error before
  install and explains which remaining checks happen during packaging and
  install.
- **GIVEN** a disabled or uninstalled UI plugin, **WHEN** an author consults the
  lifecycle matrix, **THEN** it explains `destroy`, bulk unregistration, modal
  closure, stylesheet removal, and plugin-owned side-effect cleanup.
- **GIVEN** a request for a future task-panel, task-menu, per-user storage,
  rich-text, or Kanban-card hook, **WHEN** an author checks the canonical page,
  **THEN** it says the API is unavailable in this branch and points to the
  closest supported recipe.

## Out of scope

- Adding a package checker or changing manifest, packaging, installation,
  runtime supervision, capability enforcement, signing, or database behavior.
- Adding frontend hooks, per-user plugin storage, rich-text components, or
  Kanban-card injection.
- Publishing or modifying external plugin repositories.
- Creating a second manifest, proto, TypeScript, or Go SDK definition in docs.
- Promoting the plugin system from experimental to stable.
