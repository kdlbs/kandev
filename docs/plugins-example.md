# Example plugin

`kandev-plugin-hello` is a standalone, importable reference implementation of
the kandev native-UI plugin contract (manifest + Go SDK backend, spawned by
kandev as a gRPC subprocess via `pkg/pluginsdk` + hand-written, no-build ES
module UI bundle) — it registers a "Hello" nav item and route, a
task-sidebar slot widget, a live WebSocket-driven counter, and a backend
tool/event/webhook, and is meant to be copied as the starting point for a
new plugin. It lives in its own git repository, sibling to this one, at
`../kandev-plugin-hello/` (see that repo's `README.md` for how to build,
package, and install it against a running kandev instance); see
`docs/plans/plugins/GRPC-CONTRACT.md` for the backend/transport contract and
`docs/plans/plugins/PLUGIN-API.md` for the frontend contract it implements.
