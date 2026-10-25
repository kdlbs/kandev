---
id: "09-provider-conformance"
title: "Prove packaged provider behavior and document the API"
status: done
wave: 9
depends_on:
  - "08-retention-status"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-PLUGIN-001
  - REQ-EXECUTORS-PLUGIN-002
  - REQ-EXECUTORS-PLUGIN-003
  - REQ-EXECUTORS-PLUGIN-004
  - REQ-EXECUTORS-PLUGIN-005
acceptance_criteria:
  - AC-EXECUTORS-PLUGIN-001.4
  - AC-EXECUTORS-PLUGIN-001.5
  - AC-EXECUTORS-PLUGIN-002.1
  - AC-EXECUTORS-PLUGIN-002.3
  - AC-EXECUTORS-PLUGIN-002.4
  - AC-EXECUTORS-PLUGIN-003.1
  - AC-EXECUTORS-PLUGIN-003.2
  - AC-EXECUTORS-PLUGIN-004.1
  - AC-EXECUTORS-PLUGIN-004.4
  - AC-EXECUTORS-PLUGIN-005.4
system_design:
  - ../../specs/executors/system-design/remote-executor-plugins.md
---

# Task 09: Prove packaged provider behavior and document the API

## Summary

Exercise the integrated extension through a packaged fixture plugin and real host transport. Publish the implemented authoring and operational contracts with the same change.

## In scope

- Extend cmd/plugin-fixture with a deterministic provider and fake HTTPS service, ephemeral credentials, stable operation identities and injectable failure barriers. Keep test provider controls out of production registration.
- Exercise provision, lease-authenticated HTTPS, token rotation and revocation, provider-process restart, operation recovery, exact attachment, and cleanup through the packaged SDK gRPC path. Agentctl and gateway tests cover normal session transports. Browser tests cover profile and retention flows, plus feature-off admission.
- Update public executor/plugin guides and protocol references; document reverse connectivity, retention, disable/uninstall, version compatibility, and provider conformance requirements. Update scoped AGENTS guidance if architecture descriptions changed.

## Out of scope

- Production Lambda repository, cloud account tests, built-in executor migration, publication or PR creation.

## Acceptance

- Packaged fixture proves one allocation, credential renewal and reconnect to the same resource without an AWS account or Docker dependency.
- Provider-process interruption preserves the allocation and exact resource through recovery and attachment; feature-off prevents provider allocation.
- Public contracts match implemented wire fields and tests; all nine work orders record actual results and the design status reflects implementation evidence.

The browser E2E does not launch a complete remote agent session or drive every prompt, permission,
terminal, file, Git, editor, and preview action through the fixture. Those paths use the host-owned
agentctl and gateway transports and have focused unit/integration coverage. No production provider or
cloud account validation is included.

## Verification

Use TDD for changed logic. Run commands from the repository root.
Before the first pnpm command in a fresh worktree, run `(cd apps && pnpm install --frozen-lockfile)`.
Tests named below are new unless an existing path is listed. Do not accept a no-tests result.

```bash
make -C apps/backend e2e-plugin-package
(cd apps/backend && go test -race ./cmd/plugin-fixture ./pkg/pluginsdk ./internal/plugins/... -count=1)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/remote-executor-plugin.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Required evidence:

- `internal/plugins/executor_conformance_test.go: TestPluginExecutorPackagedRecovery` verifies provider-process restart, operation recovery, exact attachment, HTTPS lease rotation/revocation, and cleanup over managed plugin gRPC.
- `cmd/plugin-fixture/executor_provider_test.go: TestPluginExecutorFixtureContract` exercises provider operations and test HTTPS requests.
- `e2e/tests/plugins/remote-executor-plugin.spec.ts` verifies packaged provider retention disclosure and feature-off profile admission.
- Profile/status desktop and phone E2Es plus agentctl, lifecycle, and gateway tests cover the remaining host-owned flows.

## Files likely touched

- `apps/backend/cmd/plugin-fixture/ (new executor_provider.go and executor_provider_test.go)`
- `apps/backend/internal/plugins/ (new executor_conformance_test.go)`
- `apps/web/e2e/fixtures/ and helpers/ packaged plugin support`
- `apps/web/e2e/tests/plugins/remote-executor-plugin.spec.ts (new)`
- `docs/public/executors.md`
- `docs/public/plugins-authoring.md and plugins-manifest.md`
- `docs/plans/plugins/GRPC-CONTRACT.md and PLUGIN-API.md`
- `apps/backend/AGENTS.md and apps/web/AGENTS.md if affected`

## Dependencies

[Task 08](task-08-retention-status.md).

## Risks

The fixture must exercise the managed plugin RPC boundary, not only an in-process fake. TLS fixtures must not create a production insecure-transport switch.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/executors/requirements/remote-executor-plugins.md), using the IDs in frontmatter.
- [System design](../../specs/executors/system-design/remote-executor-plugins.md), including the named contract and flow.
- Existing source and nearby tests in the file list; follow scoped `AGENTS.md`.
- [Accepted ADR](../../decisions/2026-09-26-remote-executor-plugin-boundary.md).

## Results

Implemented and validated. The packaged conformance test drives provision, connection lease resolution,
HTTPS requests, credential rotation and stale-token rejection, plugin subprocess interruption, recovery,
exact resource attachment, and destroy through the actual managed gRPC boundary. The fixture verifies
that its persistent allocation count remains one and that bootstrap/profile secrets are absent from its
resource inventory. `TestPluginExecutorFixtureContract` also exercises the provider operations directly.

The managed Chromium E2E covers packaged-provider retention disclosure and feature-off admission. The
profile editor and retention/status desktop and phone E2Es are recorded in Tasks 07 and 08. Focused
agentctl, lifecycle, and gateway suites cover the host-owned remote session transports and cleanup paths.
There is no browser-driven full remote session against the fixture, and no AWS or production-provider
validation was performed.

Verification passed:

- `make -C apps/backend e2e-plugin-package`.
- `(cd apps/backend && go test -race ./cmd/plugin-fixture ./pkg/pluginsdk ./internal/plugins/... -count=1)`.
- `(cd apps/backend && go test -race ./internal/agent/runtime/agentctl ./internal/agent/runtime/lifecycle ./internal/gateway/websocket -run 'TestPluginExecutor' -count=1)`.
- `(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/remote-executor-plugin.spec.ts)` — 2 passed.
- `node --test scripts/validate-public-docs.test.mjs` — 62 passed.
- `node scripts/validate-public-docs.mjs` — 47 public documents validated.
- `python3 scripts/list-docs.py validate`.
- `python3 scripts/lint-spec-files.test.py` — 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`.
- `git diff --check`.
