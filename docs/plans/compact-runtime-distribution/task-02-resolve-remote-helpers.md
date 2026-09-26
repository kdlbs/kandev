---
id: "02-resolve-remote-helpers"
title: "Resolve and cache exact remote helpers"
status: complete
wave: 2
depends_on:
  - "01-extract-mcp-contract"
plan: "plan.md"
requirements:
  - REQ-RELEASE-COMPACT-RUNTIME-001
  - REQ-RELEASE-COMPACT-RUNTIME-002
  - REQ-RELEASE-COMPACT-RUNTIME-003
acceptance_criteria:
  - AC-RELEASE-COMPACT-RUNTIME-001.2
  - AC-RELEASE-COMPACT-RUNTIME-002.1
  - AC-RELEASE-COMPACT-RUNTIME-002.2
  - AC-RELEASE-COMPACT-RUNTIME-002.3
  - AC-RELEASE-COMPACT-RUNTIME-002.4
  - AC-RELEASE-COMPACT-RUNTIME-002.5
  - AC-RELEASE-COMPACT-RUNTIME-002.6
  - AC-RELEASE-COMPACT-RUNTIME-003.1
  - AC-RELEASE-COMPACT-RUNTIME-003.3
system_design:
  - ../../specs/release/system-design/compact-runtime-distribution.md
---

# Task 02: Resolve and cache exact remote helpers

## Summary

Accept the new standard and full bundle layouts at startup and resolve a
remote helper only when a remote executor needs one. Fetch the matching
release asset into a verified local cache without exposing partial downloads
to concurrent callers. This task defines the manifest schema and tests with
fixtures; existing published archive names and contents stay complete.

## In scope

- Validate standard/full manifests against the running version and commit;
  preserve legacy complete-bundle behavior without a manifest. Resolve the
  bundle root from `KANDEV_BUNDLE_DIR` or the running executable's `bin/`
  parent, and pass the validated root from launcher to backend child.
- Update the Tauri Rust runtime validator to accept a standard resource tree
  with both host binaries and a root manifest, and an older complete resource
  tree without a manifest. Reject a missing-manifest slim tree and a partial
  legacy tree. Keep Desktop release packaging unchanged until Task 04.
- Implement override, bundled helper, verified cache, then exact-platform
  fetch precedence. Disable host-native fallback for manifest-bearing bundles.
- Pass build identity from launcher/backend construction and thread context
  from SSH, Docker, Sprites, and Kubernetes calls through the resolver.
- Bound fetch by 90 seconds and the remaining launch deadline; limit size,
  honor proxies, verify SHA-256, and publish atomically. Use `singleflight`
  for same-key requests and test distinct-process atomic writes.
- Keep current and two previous cache versions and protect paths mounted by
  active or reconnectable containers. If inventory is uncertain, defer
  cleanup. Test rollback and pinned-container paths.
- Emit a typed running/completed/failed download step through the existing
  prepare-progress callback. Add optional kind/platform fields to the wire
  step, make the web panel show typed steps with an empty raw name, and
  translate the label and recovery text in all six locales; prove the phone
  view shows the same state.
- Test startup, local operation, offline cache reuse, upgrade isolation,
  malformed metadata, interrupted fetches, concurrent resolution, progress,
  and timeouts against each executor's launch budget.

## Out of scope

- Changing executor transport/upload logic, archive production, and
  release-channel cutover.

## Acceptance

- Standard local startup and sessions require no helper download; a remote
  operation downloads only its supported platform's exact asset.
- Full and legacy complete bundles operate offline; matching cached helpers
  work after restart, while old-version entries cannot serve new releases.
- Rust Desktop startup validation accepts both future standard and current
  complete resource fixtures before a slim Desktop installer is published.
- Overrides keep current precedence; bad or unavailable downloads never
  execute a stale, partial, or host-native helper.
- Concurrent fetches share one transfer; cleanup retains the rollback window
  and every possibly mounted helper. The prepare panel shows a localized
  download step and timeout/failure state on desktop and phone.

## ASCII UI preview

`UI-01: Remote helper download`, existing session preparation panel:

```text
Preparing environment
  [spinner] Downloading remote helper (linux/amd64)
  [pending] Starting remote agent
```

Desktop and phone keep the current inline panel and scroll owner; see the
[full preview](plan.md#ascii-ui-preview). The row changes to completed or
failed when the fetch ends. This covers `AC-RELEASE-COMPACT-RUNTIME-002.6`.

## Verification

```bash
(cd apps/backend && go test ./internal/launcher ./internal/agent/runtime/lifecycle)
(cd apps/backend && go test ./internal/agent/executor/...)
(cd apps/desktop/src-tauri && cargo test --features desktop-runtime)
(cd apps/web && pnpm exec vitest run components/session/prepare-progress.test.tsx)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/session/remote-helper-progress.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-remote-helper-progress.spec.ts)
```

## Files likely touched

- `apps/backend/internal/launcher/bundle.go`
- `apps/backend/internal/launcher/bundle_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/agentctl_resolver.go`
- `apps/backend/internal/agent/runtime/lifecycle/agentctl_resolver_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_helper_manifest.go`
- `apps/backend/internal/agent/runtime/lifecycle/remote_helper_manifest_test.go`
- `apps/backend/internal/backendapp/agents.go`
- `apps/backend/internal/backendapp/helpers.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_kubernetes.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_sprites_operations.go`
- `apps/backend/internal/agent/runtime/lifecycle/container.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/env_preparer.go`
- `apps/desktop/src-tauri/src/backend.rs`
- `apps/web/components/session/prepare-progress.tsx`
- `apps/web/components/session/prepare-progress.test.tsx`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/src/locales/` (the six complete `task.json` catalogs)
- `apps/web/e2e/tests/session/mobile-remote-helper-progress.spec.ts`
- `apps/web/e2e/tests/session/remote-helper-progress.spec.ts`
- `apps/web/e2e/fixtures/backend.ts` (progress event fixture only)

## Dependencies

Task 01. This task owns the manifest schema consumed by Task 03's producer.

## Risks

- A resolver used from read-only package-manager paths must write only under
  the resolved Kandev home.
- The native host binary may use CGO and is unsafe as a remote replacement.
- Container bind mounts retain a host cache path across reconnects; cleanup
  must fail closed if references cannot be inventoried.
- A download consumes the existing launch budget and must show progress
  before a slow proxy or network request.

## Parallelism

`sequential`

## Inputs

- `REQ-RELEASE-COMPACT-RUNTIME-001` through `003` and runtime-resolution design.
- Existing launcher and resolver tests; SSH/Docker/Sprites/Kubernetes executor
  call sites; Desktop's Rust runtime validator; the current desktop/phone
  `PrepareProgress` panel.

## Results

Implemented identity-bound standard/full manifests, launcher validation and
bundle-root propagation, exact helper resolution with verified cache and
bounded downloads, pinned cache retention, and typed localized prepare
progress across Go, Desktop, and web clients.

Validation passed:

- `env -u KANDEV_INTERNAL_CONFIG_FILE go test ./internal/launcher ./internal/agent/runtime/lifecycle`
- `go test ./internal/agent/executor/...`
- `cargo test --features desktop-runtime` (68 passed)
- `pnpm exec vitest run components/session/prepare-progress.test.tsx lib/state/slices/session-runtime/prepare-result.test.ts lib/state/slices/session/set-task-sessions-prepare.test.ts lib/ws/handlers/executor-prepare.test.ts` (16 passed)
- `pnpm run i18n:check`
- Desktop and phone remote-helper progress Playwright cases (2 passed each)

The inherited `KANDEV_INTERNAL_CONFIG_FILE` points to `/root/.kandev/config.yaml`
and conflicts with launcher config-isolation tests; unsetting it for the Go
verification gives the expected isolated test environment.
