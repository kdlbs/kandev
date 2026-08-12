# @kandev/plugin-sdk

Runtime-free TypeScript contracts for Kandev UI plugins.

Use `import type` from this package and render with the `host.React`, `host.jsx`,
and `host.ui` values supplied to `initialize`. The package deliberately has no
runtime dependency on React and no Zustand or Kandev web-application imports,
so external plugin builds do not inherit the host's private module graph.

Official plugins use `host.context` for typed provider-neutral reads. Do not
copy `AppState` slice shapes or import files from `apps/web`; extend this package
and the host context implementation when a reusable capability is missing.
