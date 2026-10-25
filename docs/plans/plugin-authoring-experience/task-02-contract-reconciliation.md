---
id: "02-contract-reconciliation"
title: "Plugin contract reconciliation"
status: completed
wave: 2
depends_on: ["01-canonical-guide"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/authoring-experience.md"
---

# Task 02: Plugin contract reconciliation

## Acceptance

1. Frozen contract notes, the plugin spec, and example docs agree with source on
   Host writes, UI path rules, mounted slots, and authoritative ownership.
2. `docs/plugins-example.md` and the in-tree fixture references describe only
   real current examples and commands.
3. Runtime source is unchanged except for optional comment-only corrections
   that directly prevent a maintained example from lying.

## Verification

```sh
(cd apps/backend && go test ./cmd/plugin-pack ./cmd/plugin-fixture ./internal/plugins/manifest ./internal/plugins/pkgtar ./pkg/pluginsdk)
make -C apps/backend e2e-plugin-package
(cd apps && pnpm --filter @kandev/web test -- lib/plugins/host-api.test.ts lib/plugins/host.test.ts lib/plugins/registry.test.ts lib/ws/plugin-bridge.test.ts)
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/plans/plugins/PLUGIN-API.md`
- `docs/plans/plugins/GRPC-CONTRACT.md`
- `docs/specs/plugins/requirements/plugins.md`
- `docs/plugins-example.md`
- maintained fixture/type comments only if necessary

## Results

- Reconciled `api_write` task/message writes, root-relative UI paths, mounted
  slots, fixture comments, and example documentation against current source.
- `(cd apps/backend && go test ./cmd/plugin-pack ./cmd/plugin-fixture ./internal/plugins/manifest ./internal/plugins/pkgtar ./pkg/pluginsdk)` — 167 passed in 5 packages.
- `make -C apps/backend e2e-plugin-package` — produced the linux-amd64 fixture archive.
- Frontend-focused plugin tests passed with 50 tests across 4 files.
