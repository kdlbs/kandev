# @kandev/plugin-sdk

Runtime-free TypeScript contracts for Kandev UI plugins.

Use `import type` from this package and render with the `host.React`, `host.jsx`,
and `host.ui` values supplied to `initialize`. The package deliberately has no
dependency on React and no Zustand or Kandev web-application imports, so an
external plugin can typecheck from an isolated or sparse checkout without
inheriting the host's private module graph.

Official plugins use `host.context` for typed provider-neutral reads. Do not
copy `AppState` slice shapes or import files from `apps/web`; extend this package
and the host context implementation when a reusable capability is missing.

The `PluginHostV2*` interfaces describe capability and command data that a
plugin backend may project into its own UI. They are data types only. Browser
plugins do not receive the privileged Go Host connection or call backend Host
RPCs directly. Managed conversation input contracts include durable receipts,
bounded sequence-cursor reads, exact cancellation, and immediate dispatch
outcomes. A `busy` immediate-dispatch result never means that the host queued
the input; use enqueue when the caller needs durable delivery while a turn is
running. Only periodic inputs may request coalescing with a non-empty key.

`PluginHostV2TaskCompletionGateSnapshot` is a bounded projection type for a
plugin UI. Only the Go Host SDK can call the privileged exact completion
commands; browser plugins cannot set criteria or evidence directly.

The web Host SDK also exposes reusable `WorkspaceAgentChat`,
`WorkspaceTaskStatus`, and `WorkspaceTaskUsage` components, plus typed
`host.queries` and `host.interactions` facades. Managed-session recovery is
available only when the Host reports both the session resource version and the
opaque execution generation; plugins must pass those observed fences back to
the exact command and must not infer a recovery target from task state alone.

Register an English translation catalog with `registry.registerTranslations`
and render user-facing copy through `host.i18n`. The host scopes catalogs to the
plugin, supplies locale fallback and plural interpolation, and removes them with
the plugin generation.

Registration `icon` fields accept a curated host icon name or a plugin-owned
component. Build custom brand glyphs with `host.jsx`; Kandev renders that component
through its React runtime on navigation, repository, task-link, settings, and review
surfaces. This keeps provider assets in the provider repository.

Composer slot consumers can import `PluginComposerSlotProps` and the
`PluginTaskCreatedHandler` types. `registerTaskCreatedHandler` is available only
on create-mode `task-create-input-actions`; its returned disposer unregisters
the callback. Task-created callbacks may return a promise and receive a readonly
`{ id, workspace_id }` identity.
