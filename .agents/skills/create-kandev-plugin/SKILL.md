---
name: create-kandev-plugin
description: Create, modify, debug, test, package, or publish a Kandev runtime plugin in its dedicated repository. Use only when the requested work targets a Kandev plugin implementation or its release and marketplace lifecycle, including fixing a bug in an existing plugin. Do not use for agent skills, MCP servers, general integrations, or Kandev host, SDK, loader, and registry changes that do not also change a plugin.
---

# Create Kandev Plugin

Build Kandev runtime plugins from the official template and current public
contracts. Keep plugin source outside the Kandev monorepo and prove the packaged
artifact against a disposable development instance before publishing it.

## Establish The Boundary

1. Invoke this skill only when the user intends to create, change, fix, test,
   package, release, or submit a Kandev runtime plugin. A passing mention of
   plugins, a generic extension request, or the presence of plugin host code in
   the Kandev monorepo is not sufficient.
2. Confirm the artifact: a Kandev runtime plugin ships a `manifest.yaml`, a
   platform executable built with the Go `pluginsdk`, and optionally a native
   UI bundle. Agent instruction packages, MCP servers, and other products that
   also use the word "plugin" are outside this skill.
3. Classify the work as a new plugin, an existing plugin change, a Kandev host
   or SDK dependency required by the plugin, or a marketplace-only change.
4. Keep each production plugin in one dedicated repository. For an official
   plugin, use a public `kdlbs/kandev-plugin-<slug>` repository. For a community
   plugin, use the author's public repository. Do not add a production plugin
   implementation to `kdlbs/kandev`; the in-tree plugin fixture is test support,
   not a starter location.
5. Keep host API, SDK, registry runtime, and plugin-loader changes in the Kandev
   repository. If the plugin needs a missing host capability, treat that as a
   separate Kandev change with its own tests and compatibility review.

## Resolve The Owning Repository

Resolve the plugin repository before editing, especially for bug reports:

1. Start with repository information in the request, issue, pull request, or
   current task. Otherwise match the manifest `id` and `repo_url` against the
   official catalog entry or the installed plugin metadata. Do not infer that a
   similarly named fixture or host package in `kdlbs/kandev` owns the bug.
2. Check the repositories already materialized for the current task and locate
   the worktree whose manifest id matches the target plugin.
3. When running as a Kandev task and the repository is not attached, discover
   and call `add_branch_to_task_kandev` with exactly one of `repository_url`,
   `repository_id`, or `local_path`. It defaults to the current task and can
   find or add a repository to the workspace, then materialize its branch as a
   separate worktree.
4. `add_branch_to_task_kandev` only works with the Worktree executor. For other
   executors, or when the task tool is unavailable, ask to attach the repository
   or create a related task that explicitly targets it. Do not clone a nested
   repository inside the Kandev monorepo worktree.
5. Change the working directory to the plugin worktree and read its local agent
   instructions, manifest, build files, tests, and release workflow. For a bug,
   reproduce and fix the behavior there with `/fix` and `/tdd`, then retain this
   skill's artifact verification. If the fix also needs a Kandev host or SDK
   change, keep the two repository deliverables and verification steps explicit.

## Understand The Runtime Model

Treat a plugin as two independently loaded surfaces joined by the manifest:

```text
package tarball -> validate + extract -> supervised plugin executable <-> Host gRPC
                              |-------> static UI bundle -> browser plugin registry
Kandev event bus -> bounded per-plugin delivery queue -> OnEvent
external or UI request -> declared webhook route -> HandleWebhook
```

- Kandev owns the executable lifecycle. It starts the declared host-platform
  binary with the install directory as its working directory, injects a fresh
  Host connection on every start, and supervises crashes and failed health
  checks. Do not launch a second long-running server from the plugin.
- `KANDEV_PLUGIN_DATA_DIR` is the plugin-owned durable file directory. It
  survives restarts and version upgrades and is deleted on uninstall. Host
  state is the better fit for small JSON objects that should participate in
  Kandev backups.
- Install attempts to activate the plugin immediately. Disable preserves its
  config, state, secrets, versions, and data; uninstall removes them. A config
  update restarts an active plugin, so load configuration during startup.
- The UI bundle is served from the extracted package and does not pass through
  the plugin process. Its `initialize` and optional `destroy` hooks may run
  repeatedly as a plugin is disabled and re-enabled in the same browser tab.
- Capabilities gate Host API methods; they are not an operating-system or
  browser sandbox. The plugin executable inherits Kandev's process environment,
  and the UI runs as same-origin JavaScript with host store access. Treat
  installation as privileged code execution and hold official plugins to
  dependency, credential, and data-access review.

Choose the narrowest surface that satisfies the behavior:

| Need | Surface | Contract to design for |
| --- | --- | --- |
| React to Kandev activity | `OnEvent` | Retryable best-effort delivery, bounded in-memory queues, and possible loss require idempotency and reconciliation for critical workflows. |
| Receive an external call or relay a UI request | `HandleWebhook` | Only declared keys are routed; validate method, authentication, and provider signatures inside the plugin. |
| Store small structured data | Host state | Values are JSON objects keyed by scope and key; there is no transaction or compare-and-swap API. |
| Store files or use a plugin-managed database | `KANDEV_PLUGIN_DATA_DIR` | The plugin owns schema, locking, migrations, and recovery. |
| Read Kandev entities | Typed Host readers plus `api_read` | Use opaque pagination cursors and stable SDK DTOs; never query Kandev's database or internal HTTP API. |
| Mutate Kandev entities | Native host UI commands where available | `api_write` is reserved and SDK write methods are not implemented. A missing mutation requires a separate Host API change. |
| Notify another plugin | `Host.EmitEvent` | Events are published as `plugin.<id>.<name>`; keep names and payloads versionable. |
| Add native interface | UI registry and `host.ui` | Use host-owned React and components so themes, contexts, portals, and mobile behavior remain compatible. |

## Read The Current Contracts

Read these sources before designing the plugin:

1. `docs/public/plugins-authoring.md` for the supported backend, Host API,
   native UI, packaging, install, and iteration workflow.
2. `docs/public/plugins-manifest.md` for the authoritative manifest fields,
   capability gates, and event vocabulary.
3. `docs/public/plugins-marketplace.md` when publishing or updating a catalog
   entry.
4. The current `kdlbs/kandev-plugin-template` repository, including its
   `README.md`, `Makefile`, tests, and release workflow.

Prefer the public authoring docs and current template over old examples or
implementation plans. Read `docs/plans/plugins/GRPC-CONTRACT.md` and
`docs/plans/plugins/PLUGIN-API.md` only when changing the host contract or when
the public docs do not answer a low-level compatibility question.

When debugging a contract discrepancy, verify it at the implementation
boundary: manifest and package rules live under
`apps/backend/internal/plugins/manifest` and `pkgtar`; runtime, Host, webhook,
and delivery behavior live under `apps/backend/internal/plugins`; native UI
loading and registration live under `apps/web/lib/plugins`.

## Create A New Repository

Skip this section for changes to an existing plugin after resolving its owning
repository above.

1. Derive a lowercase plugin id and repository name. For an official plugin,
   use the full `kandev-plugin-<slug>` value for both. Keep the manifest `id`,
   Go module, Makefile binary and package names, UI registration id, release
   asset name, and catalog id synchronized as the template documents.
2. Create the repository from `kdlbs/kandev-plugin-template`; do not hand-roll
   files that the template already maintains.
3. For official plugins, create or target `kdlbs/kandev-plugin-<slug>` and set
   `repo_url` to that public repository. Do not publish to the organization or
   mutate repository settings unless the user requested that external action.
4. Inspect repository-local instructions before editing. Replace template
   identity and example behavior without deleting its packaging, test, or
   release safeguards.
5. Materialize the plugin as a sibling of the Kandev checkout, or update the
   template's `go.mod` replacement deliberately. Until the SDK is a standalone
   module, the default `replace` resolves `../kandev/apps/backend`.

If the requested repository does not exist and cannot be created with the
available GitHub tooling, stop after producing a precise repository bootstrap
request. Do not silently substitute a directory in the Kandev monorepo.

## Implement With Least Privilege

1. Write concrete behavior examples and failure cases before implementation.
   Use `/tdd` for backend and manifest logic.
2. Declare only the capabilities the plugin exercises. Treat `api_read`,
   `state`, `secrets`, event subscriptions, and webhooks as permission
   boundaries rather than descriptive metadata.
3. Embed `pluginsdk.UnimplementedPlugin`, override only required methods, and
   access Kandev through the injected Host API. The Host can be unavailable
   during construction and isolated tests, so resolve it when handling work.
   Honor context cancellation and do not reach into Kandev internal packages,
   its database, or undocumented REST endpoints.
4. Make event handling idempotent by `EventID`. Kandev makes one attempt plus
   three retries after 5s, 15s, and 45s, using the same event id. Delivery is
   sequential per plugin, but its queue and error-state buffer are bounded and
   in memory; backend restarts and sustained overload can lose events. Add a
   source-of-truth reconciliation path when missing an event is unacceptable.
5. Keep operator credentials in `config_schema` secret fields or the Host
   secret APIs. `GetConfig` returns the plugin's own secret values in cleartext,
   so never commit real credentials or log complete config objects.
6. For a UI bundle, use `host.jsx`, `host.ui`, `host.store`, and
   `host.api.fetch` as documented. Do not bundle another React or Radix runtime.
   Make `initialize` repeatable and use `destroy` to remove timers,
   subscriptions, and side effects; Kandev revokes registered routes, slots,
   handlers, styles, and navigation separately. Use `/mobile-parity` for
   interaction design and `/e2e` for user-visible flows.
7. Treat webhook routes as public ingress. Kandev limits bodies to 4 MiB and
   rejects undeclared keys, but it does not authenticate callers or enforce the
   manifest's informational `method` field. Validate both before side effects,
   return status codes from 100 through 599, and avoid reflecting unsafe
   headers or bodies.
8. Keep package paths and platform declarations synchronized. Build every
   executable declared in `runtime.executables`; include `.exe` for Windows.

## Verify The Artifact

Run the plugin repository's own formatting, tests, and lint commands first,
then verify the artifact rather than only the source tree:

1. Build a host-only package with the template's `make package-host` target for
   the local loop.
2. Confirm the archive contains `manifest.yaml`, the current host executable,
   optional UI assets, and the generated internal `checksums.txt`. Never author
   the internal checksum file by hand.
3. Install the archive into a disposable dev Kandev instance, enable it, and
   exercise every declared capability. Cover config validation, permission
   failures, lifecycle restart, events or webhooks, and native UI registration
   as applicable.
4. Exercise the failure guarantees that matter to this plugin: duplicate event
   delivery, handler cancellation, missing or invalid webhook authentication,
   unavailable dependencies, denied Host calls, and corrupt state. For native
   UI, run initialize/destroy twice and verify one plugin's initialization
   failure does not break the host or another plugin.
5. Verify an upgrade preserves Host state and the data directory when the
   plugin owns either. Verify disable preserves operator data and uninstall
   removes it when lifecycle behavior is part of the change.
6. Bump the manifest version or uninstall the existing version before
   reinstalling; Kandev rejects reinstalling the same id and version.
7. Before release, run the repository's full cross-platform package workflow
   and confirm its release asset name is `<id>-<version>.tar.gz`.

Do not test with a developer's primary instance, database, or credentials.
Report commands run, artifact name, host platform tested, and any platform or
integration path not exercised.

## Publish When Requested

1. Keep source public before submitting an official marketplace entry.
2. Tag the version through the repository's release workflow. Confirm the
   GitHub Release contains the required plugin tarball; the release-level
   tarball digest file is optional under the current marketplace contract.
3. For the official catalog, add the repository pointer to
   `plugin-registry/plugins.yaml` in `kdlbs/kandev` only after a valid latest
   release exists. Keep catalog `id` equal to manifest `id`; leave `featured`
   to maintainers.
4. Verify the registry build resolves the latest release and package asset.
   Catalog categories are free-form discovery tags and are distinct from the
   manifest category enum.
5. For an official plugin, review dependencies, release provenance, requested
   capabilities, filesystem and network use, secret handling, and UI store
   access. Internal package checksums detect corruption but do not prove
   provenance; release-level digests are advisory and signature verification is
   not wired by default in the shipped product.

Publishing, tagging, creating organization repositories, and marketplace
submission are external side effects. Perform only the actions the user asked
for, while completing local implementation and verification independently.
