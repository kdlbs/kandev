---
id: "06-plugin-owned-task-lifecycle"
title: "Rich plugin-owned task lifecycle"
status: completed
wave: 2
depends_on: ["01-design-package", "03-protocol-manifest-actions"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 06: Rich plugin-owned task lifecycle

## Intent

Let plugins create/read plugin-owned tasks with complete remote descriptors, launch
profiles, namespaced metadata, and ownership-safe preview/cascade deletion.

## Owned paths

- `apps/backend/internal/plugins/host_write.go`
- `apps/backend/internal/plugins/host_data.go`
- `apps/backend/internal/plugins/host_write_test.go`
- `apps/backend/internal/plugins/host_data_test.go`
- `apps/backend/internal/plugins/host_data_wire_test.go`
- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/adapters_plugin_starter.go`
- `apps/backend/internal/task/service/service_requests.go`
- `apps/backend/internal/task/service/service_tasks.go`
- `apps/backend/internal/task/service/remote_repository_test.go`

## Dependencies

Tasks 01 and 03.

## Acceptance

1. Create/read shapes support existing repository ID or complete arbitrary-provider
   remote descriptor, workflow/step, plan, agent/executor profile, prompt/plan mode,
   and namespaced metadata.
2. Host writes reserved `metadata.source = plugin:<id>`, rejects reserved-key
   overwrite, exposes executor-profile reads behind `api_read:executor_profiles`, and
   preserves exact credential-free Data Center clone URLs.
3. Preview/delete only cascades trees whose source matches caller plugin; adopted or
   user-owned tasks survive.

## Verification

```sh
cd apps/backend && go test ./internal/plugins ./internal/task/service ./internal/backendapp ./pkg/pluginsdk
make -C apps/backend lint
```

## Risks

Task provenance is destructive-action authorization. Missing or mismatched ownership
must fail closed; do not use plugin state links as authority to delete user tasks.
