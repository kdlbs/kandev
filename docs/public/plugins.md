---
title: "Plugins"
description: "Install and manage kandev plugins: Go backends kandev spawns and supervises, with an optional native frontend bundle."
---

# Plugins

Plugins extend kandev without forking core: a plugin ships a **Go backend**
that kandev spawns and supervises as a subprocess over a strict typed gRPC
protocol, and can optionally ship a **native frontend bundle** that kandev
loads into the SPA. This page covers what plugins are, how to install and
manage them, and the current security posture. For building a plugin, see
[Authoring a plugin](plugins-authoring.md). For the manifest schema, see
[Plugin manifest reference](plugins-manifest.md).

Plugins are an operator-level, instance-wide capability — there is no
per-user plugin access. They are gated behind the `plugins` feature flag
(**Settings > System > Feature Toggles**), off by default in production
builds and on by default in dev/e2e builds. Enabling the flag requires a
backend restart and surfaces **Settings > Plugins** in the sidebar.

## How it works

```
                 install (URL / upload / filesystem sync)
                              │
                              ▼
                 verify checksums.txt, validate manifest.yaml
                              │
                              ▼
        extract to ~/.kandev/plugins/<id>/<version>/
                              │
                              ▼
        spawn platform executable as a subprocess (hashicorp/go-plugin)
                              │
              ┌───────────────┼────────────────────────┐
              ▼               ▼                         ▼
      gRPC DeliverEvent   gRPC InvokeTool        gRPC HandleWebhook
      (bus events, at-    (agent tool calls)     (external webhook
       least-once,                                relayed via
       buffered while                             POST /api/plugins/
       unhealthy)                                  {id}/webhooks/{key})
              │               │                         │
              └───────────────┴────────────┬────────────┘
                                            ▼
                              plugin calls back over the
                              same connection: Host.GetState /
                              SetState / DeleteState / ListState /
                              RevealSecret / EmitEvent
                                            │
                                            ▼
                       (optional) SPA loads ui.bundle at boot,
                       registers native routes/nav/slots/WS handlers
```

Kandev owns the whole process lifecycle: it extracts the package, spawns the
binary, completes the go-plugin handshake, health-checks it (`Ping` every
30s), and restarts it on crash or repeated health-check failure (backoff,
max 5 attempts). There is no separate operator-managed plugin process to run
or babysit — install a package and kandev does the rest.

## Installing a plugin

Open **Settings > Plugins** and click **Install plugin**. You can install
from a URL (kandev downloads the tarball) or by uploading a `.tar.gz` file
directly. No credentials are ever shown or copied — installing a plugin has
nothing to reveal, unlike a webhook-secret/API-key registration flow.

The same operations are available over HTTP:

```bash
# Install from a URL
curl -X POST http://localhost:38429/api/plugins/install \
  -H 'Content-Type: application/json' \
  -d '{"url": "https://example.com/acme-tools-1.0.0.tar.gz"}'

# Install by uploading a local tarball
curl -X POST http://localhost:38429/api/plugins/install \
  -F "package=@acme-tools-1.0.0.tar.gz"
```

Either path runs the same pipeline:

1. Verify `checksums.txt` covers every other file in the tarball and every
   hash matches (always enforced).
2. If `checksums.txt.sig` is present, verify the signature; if absent,
   install proceeds with a surfaced "unsigned plugin" warning.
3. Parse and validate `manifest.yaml` **before any code runs**: schema, `id`
   pattern, capability vocabulary, and that `runtime.executables` contains
   an entry for the host's OS/arch.
4. Extract to `~/.kandev/plugins/<id>/<version>/` and record the
   installation.
5. Spawn the platform-matched binary and complete the go-plugin handshake.
   Status is `registered` while this is pending, `active` once the
   handshake succeeds, or `error` if spawn/handshake fails (the operator can
   retry via **Enable**).

A successful install that failed to spawn returns HTTP 201 with a
`warning` field rather than failing outright — the package is installed,
just not yet running.

## Filesystem sideload and Sync

Besides install-by-URL/upload, an operator with shell access to the host can
place plugin content directly under `~/.kandev/plugins/` without going
through the install endpoint. The **Sync** button in Settings > Plugins (and
`POST /api/plugins/sync`) reconciles kandev's registry with what is actually
on disk:

- **A dropped directory** — `~/.kandev/plugins/<id>/<version>/manifest.yaml`
  placed manually with no existing record — is validated and registered
  with status **`disabled`**, never auto-enabled. Directory sideloads skip
  the checksum/integrity gate the URL/upload pipeline runs, so an operator
  must explicitly inspect and enable one. If more than one version
  directory exists for the same unregistered id, the lexically greatest
  version is registered and the others are reported as skipped.
- **A dropped tarball** — any `*.tar.gz` sitting directly in
  `~/.kandev/plugins/` — is run through the same verified install pipeline
  `POST /api/plugins/install` uses. On success the tarball file is deleted;
  on failure it is left in place and the failure is reported.
- **A missing install** — a registered record whose `install_path` no
  longer exists on disk — is stopped (if running) and marked `error`.

At boot, kandev runs only the directory-sideload and missing-install steps
(never the tarball-install step), as part of resuming plugins that were
already active. This is conservative by design: starting up never spawns a
binary an operator hasn't explicitly approved via install or Sync.

## Enable, disable, uninstall

- **Disable** stops the subprocess. Config and state are preserved; no
  events, tools, or webhooks are delivered while disabled.
- **Enable** respawns the subprocess and re-completes the handshake.
- **Uninstall** stops the subprocess and deletes the plugin's package,
  registration record, and all persisted state — there is no grace period.

## Signed vs. unsigned packages

Every package's `checksums.txt` is verified at install time — this integrity
gate is always enforced. Signing is a separate, optional layer: if a package
includes a `checksums.txt.sig` (an ed25519 signature over `checksums.txt`),
kandev verifies it and marks the plugin signed. An unsigned package still
installs, with a surfaced warning — signing is not required in v1.

## On-disk layout

```
~/.kandev/plugins/
├── <id>.yml                    # registration record (status, install_path, signed, ...)
├── <id>.config.yml             # operator-editable config (PATCH /api/plugins/{id})
└── <id>/
    └── <version>/              # extracted package (InstallPath)
        ├── manifest.yaml
        ├── server/plugin-<goos>-<goarch>[.exe]
        ├── ui/bundle.js         # optional
        └── data/                # KANDEV_PLUGIN_DATA_DIR for this plugin
```

## Security posture

- **Auth is the spawn relationship.** Kandev spawns the plugin subprocess
  itself over a unix domain socket (macOS/Linux) or loopback TCP with
  AutoMTLS (Windows) — never a routable network address. There is no
  `api_key`, `webhook_secret`, or HMAC signing anywhere in the contract; the
  go-plugin handshake plus AutoMTLS authenticate the channel.
- **Capability-based access control.** A plugin can only call the Host RPCs
  it declared in its manifest (`state`, `secrets`); an undeclared capability
  returns gRPC `PermissionDenied` with a message naming the missing
  capability, checked by a server interceptor before the handler runs.
- **Native UI plugins run in-origin with full app-store access.** This is an
  accepted tradeoff, not an oversight: a plugin bundle shares the kandev
  React instance and Zustand store so it can build UI indistinguishable from
  first-party pages. In v1 only **active, operator-installed** plugins load;
  a failing bundle or `initialize` is caught and never breaks boot; slot
  components render behind error boundaries. Hard sandboxing (a worker or
  realm boundary) is explicit future work — see below.
- **Package integrity is always checked; signing is optional.** See
  "Signed vs. unsigned packages" above.
- **No plugin marketplace.** Install-by-URL/upload is a manual, single-plugin
  action; there is no central catalog, one-click install, or automatic
  discovery.

Related: [Authoring a plugin](plugins-authoring.md), [Plugin manifest
reference](plugins-manifest.md), and [Extending Kandev](extending-kandev.md).
