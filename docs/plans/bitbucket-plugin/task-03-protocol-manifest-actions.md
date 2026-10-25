---
id: "03-protocol-manifest-actions"
title: "Plugin protocol, manifest ownership, and authenticated actions"
status: completed
wave: 1
depends_on: ["01-design-package"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 03: Plugin protocol, manifest ownership, and authenticated actions

## Intent

Add every additive RPC, SDK, manifest, and authenticated-action shape required by later
task, reference, credential, and repository-provider work. No later task edits proto
or generated stubs.

## Owned paths

- `apps/backend/proto/kandev/plugin/v1/plugin.proto`
- Generated `apps/backend/proto/kandev/plugin/v1/plugin.pb.go`
- Generated `apps/backend/proto/kandev/plugin/v1/plugin_grpc.pb.go`
- `apps/backend/pkg/pluginsdk/plugin.go`
- `apps/backend/pkg/pluginsdk/serve.go`
- `apps/backend/pkg/pluginsdk/data_types.go`
- `apps/backend/pkg/pluginsdk/host.go`
- `apps/backend/internal/plugins/manifest/manifest.go`
- `apps/backend/internal/plugins/manifest/validate.go`
- `apps/backend/internal/plugins/handlers.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/backendapp/plugins_test.go`
- `apps/backend/internal/webapp/payload.go`
- `apps/backend/internal/plugins/runtime/testdata/fixtureplugin/main.go`
- Focused sibling tests under `apps/backend/pkg/pluginsdk/` and
  `apps/backend/internal/plugins/{manifest/,handlers_test.go}`.

## Dependencies

Task 01.

## Acceptance

1. Protocol exposes optional action, reference search/authorization, credential
   resolution, executor-profile read, and rich plugin-owned task lifecycle shapes;
   old plugins receive clear unsupported responses.
2. Manifest validates declared actions, repository-provider ownership, and reference
   source ownership/collisions before dispatch.
3. Active UI-plugin boot payload projects manifest `repository_providers` as optional
   JSON `repositoryProviderIds`, preserving an explicitly present empty declaration.
4. Authenticated action handler rejects undeclared/unauthorized resources, bounds
   bodies, passes verified context apart from payload, honors cancellation/timeout, and
   allows only safe response headers.

## Verification

```sh
cd apps/backend && go test ./pkg/pluginsdk/... ./internal/plugins/... ./internal/plugins/manifest/...
make -C apps/backend lint
```

## Risks

Generated protobuf changes are shared contract files. Keep additions wire-compatible;
never hand-edit generated files or expose Bitbucket-specific fields.
