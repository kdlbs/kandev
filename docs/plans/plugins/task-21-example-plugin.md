---
id: task-21
title: Example plugin repo (importable)
status: done
wave: C
depends_on: [task-16]
plan: docs/plans/plugins/plan.md
---

# Example plugin repo (importable)

## Title
A standalone git repo implementing a complete native-UI kandev plugin, usable as
the reference/import example.

## Inputs
- `PLUGIN-API.md` (frontend contract + "Example plugin must") and the manifest
  format (spec + task-16 `ui.bundle`).
- The plugin is a **separate process + its own git repo**. Create it OUTSIDE the
  kandev repo at `/home/jcfs/.kandev/tasks/kandev-plugin-system_t6e/kandev-plugin-hello/`
  and `git init` it (do not nest inside kandev's git). Also drop a copy/symlink note
  in kandev docs pointing to it.

## Acceptance
1. Repo `kandev-plugin-hello/` with `git init` + initial commit:
   - `manifest.yaml`: id `kandev-plugin-hello`, api_version 1, base_url
     `http://localhost:9100`, endpoints (health/events/tools/webhooks),
     `ui.bundle: /ui/bundle.js`, capabilities.events `["task.created"]`, one tool
     `echo`, config_schema minimal.
   - `server/` a small stdlib-only Go HTTP server (or Node) serving: `/health`,
     `/events` (HMAC verify, logs), `/tools/echo`, `/webhooks/*`, and **`/ui/bundle.js`**
     (the built ES module) + any `/ui/*` assets.
   - `ui/` source for the bundle: an ES module that calls
     `window.registerKandevPlugin("kandev-plugin-hello", { initialize(registry, host) {...} })`
     and registers: nav item "Hello" → route `/plugins/hello` (a native page using
     `host.jsx` + `host.ui` showing a task-created counter), a `task-sidebar` slot
     component, and a `registerWsHandler("task.created", ...)` incrementing the counter.
     No bundled React (uses `host.React`). Provide a tiny build (esbuild) producing
     `server/public/ui/bundle.js`, or hand-write the bundle as plain JS using
     `host.jsx` so no build step is needed (prefer no-build for portability).
   - `README.md`: what it demonstrates, how to run the server, how to register it
     against a running kandev (`POST /api/plugins/register` with manifest, or via the
     Plugins settings page), and how the bundle loads.
   - `Makefile`/`run.sh` to build+run.
2. Manually loadable: running the server + registering the manifest makes the
   "Hello" nav item and `/plugins/hello` page appear in kandev (verified by task-22 e2e).

## Files
- new repo at `/home/jcfs/.kandev/tasks/kandev-plugin-system_t6e/kandev-plugin-hello/**`
- `docs/public/` or `docs/` pointer in kandev (a short "example plugin" note)

## Verification
- Build the bundle (if a build step) and `go build`/`node -c` the server.
- `curl /health`, `curl /ui/bundle.js` return 200; bundle is a valid ES module that
  references `window.registerKandevPlugin`.

## Output contract
Report repo path, files, whether a build step is needed, and the exact registration
command. This repo is a deliverable the user asked for explicitly.

## Dependencies
task-16 (manifest bundle field) + PLUGIN-API.md.
