---
title: "Authoring a Plugin"
description: "Build a kandev plugin backend and optional native UI, package it, and iterate against a local kandev instance."
---

# Authoring a Plugin

This is a build tutorial for a kandev plugin: a Go backend spawned by kandev
over gRPC, with an optional native frontend bundle. See [Plugins](plugins.md)
for the operator-facing install/enable/disable flow, and the [Plugin manifest
reference](plugins-manifest.md) for the full `manifest.yaml` schema.

Two working example plugins accompany this guide:

- `kandev-plugin-hello` — a nav item, native route, sidebar slot component, a
  WS-driven counter, a Host-state-backed event handler, an agent tool, and a
  webhook.
- `kandev-plugin-github` — a connector PoC that shells out to the `gh` CLI
  from a webhook handler used as a request relay, reads live kanban data from
  the host store, and opens kandev's real create-task dialog
  (`host.ui.TaskCreateDialog`) prefilled from a pull request.

## Prerequisites

- Go (matching the version in `apps/backend/go.mod`).
- The kandev plugin SDK, `github.com/kandev/kandev/pkg/pluginsdk`. It is
  **not yet published as a standalone Go module** — it lives inside the
  kandev monorepo (`apps/backend/pkg/pluginsdk`), so a plugin repo develops
  against a local checkout via a `replace` directive:

  ```go
  // go.mod
  module my-plugin

  require github.com/kandev/kandev v0.0.0-00010101000000-000000000000

  replace github.com/kandev/kandev => ../kandev/apps/backend
  ```

  Adjust the relative path to wherever you've checked out the kandev repo.
  This is what both example plugins do; see their `go.mod` for the exact
  pinned dependency versions.

## Backend: minimal plugin

A plugin backend implements `pluginsdk.Plugin` and calls `pluginsdk.Serve`,
which owns the entire go-plugin/gRPC transport — handshake, plugin map, and
Host injection. There is no HTTP server, no listen address, and no
credentials to configure.

```go
// main.go
package main

import "github.com/kandev/kandev/pkg/pluginsdk"

func main() {
	pluginsdk.Serve(&myPlugin{})
}
```

```go
// plugin.go
package main

import (
	"context"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

type myPlugin struct {
	pluginsdk.UnimplementedPlugin // embed to get HostSetter + no-op defaults
}

var _ pluginsdk.Plugin = (*myPlugin)(nil)
```

`pluginsdk.Plugin` is the interface kandev calls into:

```go
type Plugin interface {
	// OnEvent handles a single bus event delivery. A non-nil error causes
	// kandev to retry (3 retries, 5s/15s/45s backoff).
	OnEvent(ctx context.Context, e *Event) error

	// InvokeTool handles an agent-invoked tool call declared in tools: in
	// manifest.yaml.
	InvokeTool(ctx context.Context, req *ToolRequest) (*ToolResponse, error)

	// HandleWebhook handles an inbound request relayed from
	// POST /api/plugins/{id}/webhooks/{key}.
	HandleWebhook(ctx context.Context, req *WebhookRequest) (*WebhookResponse, error)
}
```

Embed `pluginsdk.UnimplementedPlugin` and override only the methods you need
— it's a no-op base that also implements `HostSetter`, so `Serve` injects a
live `Host` into your plugin once the broker connection back to kandev is
established (retrieve it later via `p.Host()`).

## The Host API

A plugin calls back into kandev through the injected `Host`:

```go
type Host interface {
	GetState(ctx context.Context, scope, scopeID, key string) (value map[string]any, found bool, err error)
	SetState(ctx context.Context, scope, scopeID, key string, value map[string]any) error
	DeleteState(ctx context.Context, scope, scopeID, key string) error
	ListState(ctx context.Context, scope, scopeID string) ([]StateEntry, error)

	// GetConfig returns the plugin's own operator-editable config (set via
	// Settings > Plugins > <plugin>, generated from manifest config_schema).
	// Ungated — always readable, secret values included. Kandev restarts a
	// running plugin when its config changes, so re-read at startup.
	GetConfig(ctx context.Context) (map[string]any, error)

	// GetSecret/SetSecret/DeleteSecret manage a plugin-owned secret in
	// kandev's encrypted vault, namespaced to this plugin. Require the
	// `secrets` capability.
	GetSecret(ctx context.Context, key string) (value string, found bool, err error)
	SetSecret(ctx context.Context, key, value string) error
	DeleteSecret(ctx context.Context, key string) error

	// RevealSecret resolves an operator-provided secret reference (e.g. a
	// config value pointing at a shared kandev secret) to its cleartext
	// value. For secrets the plugin itself owns, use GetSecret/SetSecret.
	RevealSecret(ctx context.Context, ref string) (string, error)

	EmitEvent(ctx context.Context, name string, payload map[string]any) error

	// Tasks/Sessions/Workspaces/Workflows/AgentProfiles/Repositories return
	// read-only accessors for the Host data API (ADR 0043), each gated on
	// its own capabilities.api_read entry (e.g. "tasks", "sessions").
	// Write RPCs (capabilities.api_write) are reserved and not yet
	// implemented.
	Tasks() TaskReader
	Sessions() SessionReader
	Workspaces() WorkspaceReader
	Workflows() WorkflowReader
	AgentProfiles() AgentProfileReader
	Repositories() RepositoryReader
}
```

`scope` is one of `instance`, `workspace`, `task`, `agent` (`scopeID` empty
for `instance`). `EmitEvent` publishes `plugin.<your-plugin-id>.<name>` on
kandev's internal event bus for delivery to any subscriber (including other
plugins).

The data-reader accessors return typed, paginated readers — e.g.
`host.Tasks().List(ctx, TaskFilter{...}, Page{Limit: 50})` returns
`([]Task, *PageInfo, error)` with an opaque `PageInfo.NextCursor` for the
next page. See `pkg/pluginsdk/data_types.go` for the full `Task`,
`Workspace`, `Workflow`, `WorkflowStep`, `AgentProfile`, `Repository`,
`Session`, and filter/page types.

**Capability gating.** Every Host RPC except `GetConfig` and `EmitEvent` is
checked against your manifest's `capabilities` before the handler runs:
`GetState`/`SetState`/`DeleteState`/`ListState` require
`capabilities.state: true`; `GetSecret`/`SetSecret`/`DeleteSecret`/
`RevealSecret` require `capabilities.secrets: true`; each data-reader
accessor requires its resource in `capabilities.api_read` (e.g. `tasks`,
`sessions`, `workspaces`, `workflows`, `agent_profiles`, `repositories`).
Calling one without the declared capability returns gRPC `PermissionDenied`
with a message naming the missing capability — declare what you use.

**Writable data directory.** Kandev injects `KANDEV_PLUGIN_DATA_DIR` into
every spawned plugin subprocess — a per-plugin writable directory
(`~/.kandev/plugins/<id>/data`) for anything you'd rather keep on disk than
in `Host` state.

```go
func (p *myPlugin) OnEvent(ctx context.Context, e *pluginsdk.Event) error {
	host := p.Host()
	if host == nil {
		return nil // broker dial still in progress
	}
	value, found, err := host.GetState(ctx, "instance", "", "count")
	// ...
	return host.SetState(ctx, "instance", "", "count", map[string]any{"n": 1})
}
```

## Optional: native UI

A plugin may ship `ui.bundle` in its manifest: a **hand-written, no-build**
plain-JS ES module. There is no bundler step — kandev serves the file
verbatim from the extracted package directory, so you edit the bundle and
repackage.

The single entry point: the bundle, once evaluated, calls
`window.registerKandevPlugin(id, { initialize(registry, host), destroy?() })`.
Kandev's frontend host imports the bundle on boot (or on runtime enable),
then calls `initialize(registry, host)`. On disable/uninstall it calls
`destroy?.()` and bulk-revokes every registration the plugin made.

**Registry surface** (`registry: PluginRegistry`, passed to `initialize`):

```ts
interface PluginRegistry {
  // Top-level SPA route, exact-match against window.location path. The host
  // wraps the page in kandev's title bar by default; configure or opt out
  // via options.topbar.
  registerRoute(path: string, Component: React.ComponentType, options?: PluginRouteOptions): void;
  // Sidebar/main nav entry, rendered by <PluginNavItems/>.
  registerNavItem(item: NavItem): void;
  // Route under /settings/plugins/{id}/..., rendered inside the settings shell.
  registerSettingsRoute(path: string, Component: React.ComponentType): void;
  // Named slot injection. Initial slots: "task-sidebar", "settings-nav", "main-nav-footer".
  registerComponent(slot: string, Component: React.ComponentType<{ slotProps?: unknown }>): void;
  // WS action handler, bridged into the existing lib/ws dispatch.
  registerWsHandler(action: string, handler: (payload: unknown) => void): void;
}

interface NavItem {
  id: string;
  label: string;
  path: string;
  // Curated icon name (see lib/plugins/icons.ts); unknown names render the puzzle glyph.
  icon?: string;
  // "main" (default): top-level sidebar entry. "integrations": renders inside
  // the sidebar's Integrations section alongside first-party integration links.
  section?: "main" | "settings" | "integrations";
}

interface PluginRouteOptions {
  // Kandev-style title bar above the page. Default: enabled with a derived
  // title (from the matching nav item, else the plugin's display name).
  // Pass a PluginPageChrome to configure it, or `false` for a full-bleed
  // page that owns its own chrome (e.g. with host.ui.PageTopbar).
  topbar?: boolean | PluginPageChrome;
}

interface PluginPageChrome {
  title?: string;
  subtitle?: string;
  icon?: string;               // same curated set as NavItem.icon
  backHref?: string;           // default "/"
  backLabel?: string;          // default "Kandev"
  actions?: React.ComponentType; // rendered on the right side of the topbar
}
```

**Host API** (`host: PluginHostApi`, passed to `initialize`):

```ts
interface PluginHostApi {
  pluginId: string;
  React: typeof import("react");       // shared host React instance — MUST use this, never bundle your own React
  jsx: typeof React.createElement;     // convenience alias
  store: {                              // kandev's live app store — read access is the common
    getState(): AppState;               // case; setState is exposed but writes affect the whole SPA
    setState(partial): void;
    subscribe(listener): () => void;
  };
  api: {
    // fetch scoped to /api/plugins/{id}/...; relayed to your webhook handler.
    fetch(path: string, init?: RequestInit): Promise<Response>;
    // Backend API origin ("" when the SPA and API share an origin) — lets a
    // plugin reach first-party kandev REST endpoints directly.
    baseUrl: string;
  };
  ui: Record<string, unknown>;          // curated @kandev/ui subset — see below
  theme: "light" | "dark";
  // Soft SPA navigation (history push/replace), same as the app's own router.
  navigate(href: string, options?: { replace?: boolean }): void;
}
```

A plugin bundle must render with `host.React` / `host.jsx` — bundling your
own React copy breaks hook identity against the host tree.

`host.ui` is a curated `@kandev/ui` subset — Alert, Badge, Button, Card,
Checkbox, Dialog, DropdownMenu, Input, Label, Pagination, ScrollArea, Select,
Separator, Sheet, Skeleton, Spinner, Switch, Table, Tabs, Textarea, Tooltip
(each with their compound sub-parts, e.g. `DialogContent`, `TableRow`) — plus
first-party app UI: `Combobox` (the app's picker), `PageTopbar` (the title
bar a route gets by default via `registerRoute`'s `options.topbar`, exposed
here for routes that opt out and render their own chrome), and
`TaskCreateDialog`, so a plugin can hand off task creation to kandev's real
create-task flow (repo/branch/agent pickers, validation) instead of POSTing
directly. See `apps/web/lib/plugins/host-api.ts` for the exact current list.

## Three integration patterns

Both example repos are full, working plugins — read them rather than
copy-pasting fragments:

1. **Event handling with Host state** (`kandev-plugin-hello/server/plugin.go`).
   `OnEvent` receives `task.created` deliveries (declared in
   `capabilities.events`) and increments a persistent counter via
   `Host.GetState`/`SetState`, scoped `instance`. Demonstrates the full
   Host state round trip and idempotent-by-design event handling.

2. **Live host-store reads from native UI**
   (`kandev-plugin-hello/ui/bundle.js`, `useKanbanTasks`). A plugin page
   subscribes directly to `host.store` and re-renders on every store change
   — reading the exact same live state the first-party kanban UI renders
   from, no polling and no extra API calls.

3. **Webhook-as-relay + `TaskCreateDialog`**
   (`kandev-plugin-github/server/plugin.go` and `ui/bundle.js`). The backend
   declares a `prs` webhook and, on `HandleWebhook`, shells out to the `gh`
   CLI (inheriting the plugin subprocess's own environment for GitHub auth)
   and returns JSON. The UI page calls `host.api.fetch("webhooks/prs")` —
   which kandev relays over gRPC `HandleWebhook` to the same handler — to use
   an external CLI as a same-origin data source without a webhook secret.
   Each PR row opens `host.ui.TaskCreateDialog` prefilled from the PR,
   reading `workspaceId`/`workflowId`/`steps` from the live host store.

## Packaging

A plugin ships as `<id>-<version>.tar.gz`:

```
manifest.yaml                         # authoritative; read BEFORE any code runs
server/plugin-<goos>-<goarch>[.exe]   # any subset of platforms declared in runtime.executables
ui/bundle.js                          # optional
ui/*.css / assets/icon.svg            # optional
checksums.txt                         # generated — do not author this file
checksums.txt.sig                     # optional ed25519 signature
```

Cross-compile one executable per platform you declare in
`runtime.executables`, then pack the staged directory with
`github.com/kandev/kandev/cmd/plugin-pack`:

```
plugin-pack -dir <staged-dir> -out <id>-<version>.tar.gz [-platform-only]
```

`plugin-pack` walks the directory, requires `manifest.yaml` to be present,
and **generates `checksums.txt`** covering every other file — pre-supplying
either checksum file yourself is a packaging error. `-platform-only`
restricts the package to the current host's platform, for faster local
iteration than a full multi-platform build.

A sample `Makefile` (trimmed from `kandev-plugin-hello/Makefile`):

```makefile
STAGE := .build/stage
VERSION := 1.0.0
PKG_OUT := my-plugin-$(VERSION).tar.gz

package:
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/server
	cp manifest.yaml $(STAGE)/manifest.yaml
	cp -r ui $(STAGE)/ui
	GOOS=linux   GOARCH=amd64 go build -o $(STAGE)/server/plugin-linux-amd64       ./server
	GOOS=linux   GOARCH=arm64 go build -o $(STAGE)/server/plugin-linux-arm64       ./server
	GOOS=darwin  GOARCH=amd64 go build -o $(STAGE)/server/plugin-darwin-amd64      ./server
	GOOS=darwin  GOARCH=arm64 go build -o $(STAGE)/server/plugin-darwin-arm64      ./server
	GOOS=windows GOARCH=amd64 go build -o $(STAGE)/server/plugin-windows-amd64.exe ./server
	go run github.com/kandev/kandev/cmd/plugin-pack -dir $(STAGE) -out $(PKG_OUT)
	rm -rf $(STAGE)

# Faster local loop: host platform only.
package-host:
	rm -rf $(STAGE)
	mkdir -p $(STAGE)/server
	cp manifest.yaml $(STAGE)/manifest.yaml
	cp -r ui $(STAGE)/ui
	go build -o $(STAGE)/server/plugin-$$(go env GOOS)-$$(go env GOARCH)$$(go env GOEXE) ./server
	go run github.com/kandev/kandev/cmd/plugin-pack -dir $(STAGE) -out $(PKG_OUT) -platform-only
	rm -rf $(STAGE)
```

Install the resulting tarball via **Settings > Plugins** (upload) or:

```bash
curl -F "package=@my-plugin-1.0.0.tar.gz" http://localhost:38429/api/plugins/install
```

## Iterate loop

- **UI-only or backend-only change:** edit the source, repackage
  (`make package-host` for a fast host-only build), then reinstall. There is
  no kandev rebuild needed for plugin-only changes — the extracted package
  and running subprocess are entirely separate from the kandev binary.
- **`registry`/`host.ui`/host-core changes:** these live inside kandev
  itself (`apps/web/lib/plugins/`), not the plugin package — changing them
  requires a kandev build (`make build-web` / `make build-backend`), not a
  plugin repackage.
- **Reinstalling the same version is rejected.** `pkgtar.Install` fails with
  `ErrVersionExists` (HTTP 409) if `~/.kandev/plugins/<id>/<version>/`
  already exists. To pick up a rebuilt package during iteration, either bump
  `version` in `manifest.yaml` or uninstall the plugin first.

## Gotchas

- **`ui.bundle` and `ui.styles` are root-relative paths**, e.g.
  `/ui/bundle.js` — not `ui/bundle.js`. Kandev serves them from
  `GET /api/plugins/{id}/bundle` and `GET /api/plugins/{id}/ui/*`, resolved
  against the extracted package directory.
- **Plugin pages are "unknown" SPA routes, but still get the full app
  shell.** `registerRoute` paths are resolved after every static/nested
  first-class route and before the kanban catch-all — a plugin can't shadow
  a built-in route, but its page still renders inside the normal boot-payload-hydrated
  app shell (workspaces, settings, theme), not a bare unstyled page.
- **Version bump or uninstall required to reinstall** — see "Iterate loop"
  above.
- **Windows executables need the `.exe` suffix** in both the built file name
  and the `runtime.executables` manifest value (e.g.
  `windows-amd64: server/plugin-windows-amd64.exe`) — kandev does not append
  it for you.

Related: [Plugins](plugins.md), [Plugin manifest reference](plugins-manifest.md).
