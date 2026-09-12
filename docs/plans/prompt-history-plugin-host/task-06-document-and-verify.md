---
id: "06-document-and-verify"
title: "Document and verify Host prerequisites"
status: done
wave: 5
depends_on:
  - "02-add-plugin-conversation-reads"
  - "03-extend-task-panel-capabilities"
  - "04-build-conversation-host-facade"
  - "05-prove-plugin-parity"
plan: "plan.md"
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.4
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
---

# Task 06: Document and Verify Host Prerequisites

## Scope

Finish the public reference/how-to documentation and run the package-level validation matrix. Update artifact statuses/results only after the behavior and docs are proven. Keep external-plugin and core-removal work explicitly deferred.

## Acceptance

- `docs/public/plugins-authoring.md` explains `host.conversation`, task-panel context/navigation, lifecycle cleanup, and a UI-only session-read example.
- `docs/public/plugins-manifest.md` documents `api_read:messages` and required `min_kandev_version: "0.91.1"` for every manifest that declares that resource, covering both Go `Messages().List` and the browser facade, while preserving the admin-action floor. `apps/backend/internal/plugins/manifest/min_version_policy.go` owns one `MinimumMessagesCapabilityVersion` constant and capability-aware predicate used by manifest validation, archive/Inspect, install/sideload, boot/load, fixture, and direct-build tests; it rejects missing/malformed/lower values in dev/E2E and stamped builds and covers missing, lower, equal, and higher values. `docs/plans/plugins/PLUGIN-API.md` remains canonical.
- Public docs distinguish browser Host reads from Go Host `Messages().List` and warn against `host.store`, raw WS payloads, and first-party `/api/v1` URLs.
- The documented route error table is reflected by public SDK retryability and browser tests; inaccessible resources never reveal existence.
- Update `docs/public/websocket-api.md` so its ordinary notification and
  connection-local subscription statements explicitly exclude the Host-only
  prompt-history stream, and link the canonical ordered subscribe/ACK,
  per-consumer identity, replay, and terminal-removal contract.
- Focused and repository checks below pass against fresh generated/build artifacts. The fresh fixture order is backend build, web `build:e2e`, `e2e-plugin-ui`, `e2e-plugin-package`, plugin install, then desktop/mobile/core specs; every root Makefile, runner, CI host path, and container path replaces direct package invocation with this order and consumes identity schema version `2`. The identity includes plugin ID/version, bundle path, source/output hashes, manifest hash, capability, minimum version, panel key, and archive SHA-256; every shard/container verifies the digest before install/spec execution.
- Requirements, design, plan, and work-order statuses/results reflect actual completion; no core prompt-history ownership artifact is retired.

## TDD sequence

1. RED: add/update public-doc validation examples or reference assertions where the repository already tests contract snippets.
2. GREEN: write the smallest public reference/how-to updates and fix only failures caused by this package.
3. REFACTOR: remove duplicate prose and link the canonical contract for exhaustive field definitions.

## Likely files

- `docs/public/plugins-authoring.md`
- `docs/public/plugins-manifest.md`
- `docs/plans/plugins/PLUGIN-API.md`
- this requirements/design/plan package for final status and result records
- `apps/backend/internal/plugins/manifest/min_version_policy.go`, `docs/public/plugins-authoring.md`, `docs/public/plugins-manifest.md`, `docs/plans/plugins/PLUGIN-API.md`, `apps/web/e2e/README.md`, `apps/web/e2e/global-setup.ts`, `.github/workflows/e2e-tests.yml`, `apps/web/e2e/scripts/run-e2e.sh`, and root/backend Makefiles

## Verification

```bash
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
cd apps/web && pnpm run typecheck
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web test -- --run lib/plugins components/task/plugin-task-panel.test.tsx components/task/dockview-add-panel-items.test.tsx components/task/mobile lib/state/layout-manager/serializer.test.ts lib/state/layout-manager/panel-titles.test.ts lib/state/layout-manager/plugin-panels.test.ts lib/layout/layout-profiles.test.ts
make -C apps/backend test
make -C apps/backend lint
make -C apps/backend build
cd apps/web && pnpm run build:e2e
make -C apps/backend e2e-plugin-ui
make -C apps/backend e2e-plugin-package
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/plugins/prompt-history-plugin.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/task/prompt-history-panel.spec.ts
```
Do not commit unless the user explicitly authorizes a commit. Do not implement the external plugin, remove core code, or migrate saved layout IDs.

## Current result

Published the browser facade, manifest/version floor, lifecycle, navigation,
and ordered WebSocket guidance while preserving the separate Go Host API and
core ownership. Final status reconciliation is complete.

Verified:

- `python3 scripts/lint-spec-files.py --all`
- Public-doc validator tests: 61 passed; 46 pages validated
- `cd apps/web && pnpm run typecheck`
- Frontend i18n checks and ratchet
- Focused browser Host, scope, registry, task-panel, and retention suites
- Backend `golangci-lint run ./...` (0 issues)
- `make -C apps/backend build`
- Fresh fixture desktop/mobile and built-in Prompt History E2E: 4 tests passed

The bounded repository-wide backend test run was attempted with
`go test -p 2 -tags fts5 ./...`; unrelated environment-sensitive failures
remain in agentctl config, update-channel, and invalid metadata tests, along
with temporary disk-quota linker failures. The changed packages passed focused
verification.
