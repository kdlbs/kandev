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

Register an English translation catalog with `registry.registerTranslations`
and render user-facing copy through `host.i18n`. The host scopes catalogs to the
plugin, supplies locale fallback and plural interpolation, and removes them with
the plugin generation.

Registration `icon` fields accept a curated host icon name or a plugin-owned
component. Build custom brand glyphs with `host.jsx`; Kandev renders that component
through its React runtime on navigation, repository, task-link, settings, and review
surfaces. This keeps provider assets in the provider repository.
