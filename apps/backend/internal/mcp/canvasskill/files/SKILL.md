---
name: kandev-canvas-authoring
version: "1"
description: Author a Kandev task canvas as a self-contained web application.
---

# Kandev canvas authoring

Use one `read_canvas_authoring_skill_kandev` call without `path` when you need
the authoring contract. That response is the complete core bundle. It includes
this workflow, the manifest and browser protocol summary, appearance rules,
the minimal scaffold, and the exact supporting-file inventory. Do not read the
core bundle again during the same authoring task.

## Required workflow

1. Call `create_canvas_kandev` with a short title and an application summary.
   It creates an inactive task canvas and returns its source directory,
   manifest scaffold, permission ceiling, and exact scaffold inventory.
2. Use native file tools in that returned directory. The initial files are
   `manifest.yaml`, `index.html`, `appearance.js`, `script.js`, and
   `styles.css`. Replace or extend them in the same directory.
3. Keep every source path relative to the returned directory. Do not write
   outside it. Bundle executable dependencies. Node, a package manager, and a
   network build step are not available at runtime.
4. Run local checks, then call `publish_canvas_kandev` with the returned canvas
   ID and source path. Read validation diagnostics and correct rejected source
   before publishing again.

## Core application contract

- Include `<meta name="viewport" content="width=device-width, initial-scale=1">`.
- Use relative `./_kandev/v1` paths for Kandev data, state, actions, and events.
- Treat Kandev domain data as the source of truth. Derive filters and summaries
  in memory instead of storing a second copy of domain records.
- Store only small application-specific shared values in instance state. Keep
  temporary input in memory and use conditional revisions for writes.
- The canvas has an opaque origin. Do not use browser storage, service workers,
  origin-wide cookies, host URLs, or authorization headers.
- Avoid secrets in source, URLs, query strings, logs, and client state.
- Render loading, empty, error, and retry states. Keep destructive actions
  explicit and explain their result.
- Use accessible labels, keyboard operation, visible focus, and touch targets.

## Minimal manifest

Use the returned `manifest_scaffold` as the starting point. New manifests use
`api_version: 2`, one lowercase web-app key, a package-relative `entry`, and at
least one `task-canvas` or `workspace-canvas` placement. Declare only the
`api_read`, `api_write`, `events`, `state`, and `network_origins` permissions
that the application needs. The entry and all relative assets must be in the
published package.

## Browser protocol summary

Resolve all routes from the application document with `./_kandev/v1`. Use
`context`, the paginated `data` routes, `state/{key}` with `If-Match`, and the
bounded `events` stream as documented by the optional references. Events are
hints that invalidate a read. Refetch authoritative data after an event,
reconnect with `Last-Event-ID`, and perform a full refetch after
`runtime.resync_required`.

## Appearance protocol

The host may send the public presentation-only message
`kandev.web_app.appearance` with `version: 1`, `mode: light|dark`, and exactly
these color tokens: `background`, `foreground`, `card`, `cardForeground`,
`muted`, `mutedForeground`, `border`, `primary`, `primaryForeground`,
`accent`, `accentForeground`, `destructive`, `destructiveForeground`, and
`ring`. Accept it only when `event.source === window.parent`, the type and
version match, the keys are exact, and each serialized color is bounded. Map
the tokens to the same-name kebab-case CSS variables. Keep light and dark
fallbacks so the app remains usable before the first message. The message has
no identity, capability, data, storage, navigation, or action fields.

The generated `appearance.js` implements this listener. It is optional, but
copy its pattern when replacing the scaffold.

Read a supporting reference only when its topic is needed:

- `references/browser-api.md` for detailed browser routes and errors.
- `references/manifest.md` for the full manifest shape and validation rules.
- `references/data-and-state.md` for domain data and instance state.
- `references/events-and-recovery.md` for events, reconnect, and retries.
- `references/security.md` for opaque-origin and source safety rules.
- `references/ui-patterns.md` for responsive and accessible UI patterns.
