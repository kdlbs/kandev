---
title: "Plugins"
description: "Enable, install, manage, and troubleshoot experimental Kandev plugins without losing sight of their code and network trust boundary."
---

# Plugins

Plugins extend Kandev with supervised backend processes and optional native web UI. A package can subscribe to Kandev events, keep plugin-scoped state, request named secrets, expose webhook handlers, declare tools, and add routes, navigation, or workbench slots through a frontend bundle.

The plugin system is **experimental and off in the production profile**. Its package, SDK, UI, and compatibility contracts can change between releases. Treat every plugin as executable code with the privileges of the Kandev backend and browser origin.

## Enable the plugin system

Use one of these startup-time opt-in paths:

1. Open **Settings > System > Feature Toggles**.
2. Enable **Plugins**.
3. Restart Kandev when prompted.

For a service or container, set the environment lock before starting Kandev:

```bash
KANDEV_FEATURES_PLUGINS=true kandev run
```

An explicit `KANDEV_FEATURES_PLUGINS` value wins over the database toggle and locks its control. The source checkout's development and E2E profiles enable plugins; installed production builds leave them off. After restart, open **Settings > Plugins**. A blank page or missing navigation item usually means the effective flag is still off or the restart has not happened.

## Review the trust boundary first

Install only packages whose publisher, source, and release process you trust:

- A backend plugin is a platform-specific binary spawned and supervised by Kandev. It runs as the Kandev operating-system account, not in an executor sandbox.
- A frontend bundle is JavaScript loaded into the Kandev SPA. It shares the application origin and browser session.
- `checksums.txt` protects package integrity in transit and at rest, but it does not establish publisher identity. The current runtime does not wire a signature verifier, so packages are reported as **unsigned** even when a signature file is present.
- URL install accepts HTTP(S) and limits downloads to 100 MiB, but it does not block private or loopback destinations. Anyone who can install a URL can make the backend request an internal address.
- Plugin webhook routes have no Kandev-provided authentication or HMAC layer. Put them behind an authenticated gateway or require and verify provider signatures inside the plugin.

Kandev itself has no user-login boundary on these management endpoints. On a shared or remote deployment, protect the whole origin with an authenticated reverse proxy and restrict plugin management to trusted operators. See [Security and trust](security.md) before enabling the feature.

## Install a package

Open **Settings > Plugins**, select **Install plugin**, and choose one input:

- **From URL** downloads a `.tar.gz` package from an HTTP(S) URL visible to the backend host.
- **Upload file** sends a local `.tar.gz` or `.tgz` package directly.

The installer rejects a package when:

- `manifest.yaml` is missing, invalid, or does not declare a managed binary runtime;
- `checksums.txt` is missing, incomplete, or does not match every packaged file;
- an archive path is absolute, escapes the package root, or is a symbolic/hard link;
- no executable matches the backend host's operating system and architecture;
- `min_kandev_version` is newer than the running build;
- one unpacked file or the complete unpacked package exceeds 200 MiB; or
- that exact plugin version is already installed.

On success, Kandev extracts the package under `<home>/plugins/<plugin-id>/<version>/`, starts its binary through the gRPC plugin runtime, and records it as active. A valid package can still finish in **error** when its process cannot start, complete the handshake, or become healthy. The install dialog surfaces that partial-install warning and keeps the record so it can be diagnosed.

## Understand plugin states

| State | Meaning | Next action |
|---|---|---|
| Active | The process is running and its event, webhook, and UI surfaces can be used. | Disable it before maintenance or removal when you need a controlled stop. |
| Disabled | The package and configuration remain installed, but the process and frontend contribution are stopped. | Select **Enable** to start it again. |
| Error | Startup, health, or the installed filesystem failed. | Inspect backend logs, select **Disable**, correct the package/runtime issue, then enable it again. |
| Registered | Installation was recorded and activation is still pending. | Wait for activation or inspect logs if it does not settle. |

Kandev restarts plugins that were active before a normal backend restart. Runtime health failures use supervised restart behavior; repeated failures remain visible through status and backend logs.

## Enable, disable, sync, and uninstall

The management row shows the plugin id, version, categories, status, description, and unsigned state.

- **Disable** stops the plugin process and unloads its frontend routes, navigation, and slots without removing the package or stored configuration.
- **Enable** starts a disabled package and loads its frontend contribution without a full browser reload.
- **Uninstall** stops the process and removes the plugin record, every installed version under its plugin directory, writable plugin data, and plugin-scoped database state. This is destructive; reinstalling the same id does not recover that state.
- **Sync** reconciles the registry with `<home>/plugins`. It registers manually extracted `<id>/<version>/manifest.yaml` directories as disabled and unsigned, installs `.tar.gz` files dropped directly in the plugins directory, and marks records whose install path disappeared as error. Item failures are listed without aborting the rest of the scan.

Startup performs only the conservative directory/missing-file scan. It does not install a dropped tarball automatically; select **Sync** to make that execution decision explicit.

## Know what is available today

The current runtime supports:

- at-least-once event delivery to declared event subjects, with retries and bounded buffering while a plugin is unhealthy;
- instance, workspace, task, or agent-scoped JSON state isolated by plugin id;
- named secret retrieval when the manifest declares the secrets capability;
- declared GET/POST webhook relays with a 4 MiB request-body limit;
- native UI bundles that can register plugin routes, navigation entries, and supported workbench slots; and
- an HTTP listing of tools declared by active plugins.

Current boundaries are important:

- Declared plugin tools are listed by the backend but are **not yet added to a coding agent's invocable MCP/tool set**.
- Manifest `api_read` and `api_write` capabilities are reserved; general task, repository, or workflow Host RPCs are not implemented.
- The first-party Plugins page does not render a generic form from `config_schema`. A plugin can provide its own UI, or an operator can use the experimental management API.
- Plugin processes run on the Kandev host even when tasks use Docker, SSH, Sprites, or another executor.
- Compatibility is not versioned beyond the manifest API and optional minimum Kandev version checks. Test upgrades on a disposable instance before changing a shared deployment.

## Troubleshoot a plugin

- **Plugins page is missing:** verify the effective `features.plugins` state or `KANDEV_FEATURES_PLUGINS`, then restart.
- **Package is rejected:** check `checksums.txt`, archive paths, the host platform key, package size, and `min_kandev_version`.
- **Status becomes error after install:** inspect backend logs for executable permissions, platform mismatch, handshake, health, or process-exit errors. Disable before retrying enable.
- **Plugin UI is absent:** confirm the plugin is active and its manifest declares a valid `ui.bundle`. Browser console and backend bundle responses can identify load failures.
- **Events appear twice:** delivery is at least once. Plugin handlers must deduplicate with the event id and make side effects idempotent.
- **Webhook returns 404 or 503:** 404 means the key is not declared or the plugin id is unknown; 503 means the plugin is not active or the subprocess call failed.
- **Sync reports a path:** fix only that package or directory and run Sync again; other valid items may already have been reconciled.

## Build a plugin

The authoring contract is experimental. Start from the Go interfaces under [`apps/backend/pkg/pluginsdk`](https://github.com/kdlbs/kandev/tree/main/apps/backend/pkg/pluginsdk), the current [plugin specification](https://github.com/kdlbs/kandev/blob/main/docs/specs/plugins/spec.md), and the [package/transport contract](https://github.com/kdlbs/kandev/blob/main/docs/plans/plugins/GRPC-CONTRACT.md). A backend embeds `pluginsdk.UnimplementedPlugin`, implements the event/tool/webhook methods it needs, and calls `pluginsdk.Serve`; its release package supplies the manifest, host executables, checksums, and optional UI bundle.

Do not copy a draft manifest blindly. Keep capabilities narrow, validate external webhook signatures, make event handling idempotent, avoid logging secrets, and test install/start/disable/restart/uninstall on every packaged platform.

Related: [Feature status](feature-status.md), [Configuration](configuration.md), [Security and trust](security.md), [Automation and MCP](automation-and-mcp.md), and [Extending Kandev](extending-kandev.md).
