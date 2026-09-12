---
id: "05-prove-plugin-parity"
title: "Prove prompt-history plugin parity"
status: done
wave: 4
depends_on:
  - "04-build-conversation-host-facade"
plan: "plan.md"
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-005
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-005.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-002.11
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
---

# Task 05: Prove Prompt-History Plugin Parity

## Scope

Add a test-only external-style fixture plugin that implements prompt history through `@kandev/plugin-sdk` and the published Host APIs only. Use it to prove the prerequisite boundary; do not ship it as the production plugin or delete core code.

## Acceptance

- The fixture package manifest declares `capabilities.api_read: ["messages"]`, `min_kandev_version: "0.91.1"`, uses a distinct fixture panel ID, and its generated bundle is installed by the existing plugin-package E2E setup.
- It derives newest-first prompt entries, absolute numbering, bounded durations, sender-task marker, expansion, empty/loading/error states, and older-page loading from public DTOs only.
- It uses host UI/i18n/responsive/favorite/mention/navigation contracts and contains no `apps/web` import, `/api/v1` URL, `host.store` access, or raw WS registration.
- Desktop E2E covers panel entry, initial and older pages, live add/update/delete, live turn completion updating duration, localized title/content behavior, favorite highlight, prompt alias preview, and native transcript navigation.
- `mobile-*.spec.ts` Pixel 5 E2E proves the grouped Panels entry, full-height single-scroll surface, 44 px touch controls, older loading, alias drawer, target navigation, and no document horizontal overflow.
- The fixture's turn-completion scenario proves a live completed turn updates the derived duration without retry or remount.
- The fixture's transition controls causally cover message update/delete and turn completion; assertions wait for the control response and then observe the corresponding facade update.
- Existing built-in prompt-history tests and E2E remain unchanged and green because the ordered stream's `legacy-core` adapter projects exactly one legacy notification to existing core handlers; its durable cursor, projection-before-ACK order, event-id/sequence dedupe, reconnect replay, interleaving, and terminal-removal cases are covered. Backend direct message/turn broadcasts are removed rather than duplicated.
- The package build removes the prior generated UI output, then runs the declared fixture source build (`cd apps/web && pnpm run build:e2e-plugin -- --outDir ../../apps/backend/cmd/plugin-fixture/fixture-package/ui`), and packages only after it succeeds. `e2e-plugin-package` depends on the `.PHONY` `e2e-plugin-ui` target, so stale checked-in UI cannot be packaged.
- The E2E runner and CI invoke the exact order `build backend -> build:e2e -> e2e-plugin-ui -> e2e-plugin-package -> install/spec` in host and container modes. Freshness checks hash fixture source/output plus manifest capability, min version, and panel key; the fixture parity spec exercises the production ordered WebSocket protocol, including session removal and reconnect replay. CI uploads `e2e-plugin-identity.json` with the package, every shard/container downloads both, verifies the archive SHA-256 before install, and fails before specs on missing, stale, or schema-mismatched identity.
- `apps/web/package.json` adds the `build:e2e-plugin` script and `apps/web/e2e/fixtures/plugins/prompt-history-plugin/` is its sole source; the script accepts the declared output directory and fails on missing/stale output.
- Host, container, CI, package, and global-setup paths consume one immutable fixture identity at `apps/backend/.build/e2e-plugin-identity.json`, produced by `e2e-plugin-package` after `e2e-plugin-ui` and archive creation, with schema version `2` and `{schemaVersion,pluginId,pluginVersion,bundlePath,sourceHash,generatedOutputHash,manifestHash,archiveSha256,manifestCapability,minKandevVersion,panelKey}`. The archive SHA-256 covers the exact packaged file; every consumer verifies plugin ID/version, bundle path, manifest hash, and archive digest before install or spec execution. `fixture-package/ui/` remains excluded from backend source-mtime checks. Existing generic backend identity remains separate.

## TDD sequence

1. RED: write desktop and mobile fixture-plugin scenarios against the public contract.
2. GREEN: add the smallest fixture bundle/seed/page-object support needed to exercise the Host APIs.
3. REFACTOR: keep test helpers provider-neutral and remove any shortcut into internal state or routes.

## Likely files

- `apps/backend/cmd/plugin-fixture/fixture-package/manifest.yaml` with `capabilities.api_read: ["messages"]`, `min_kandev_version: "0.91.1"`, and a distinct fixture panel identity.
- `apps/web/e2e/scripts/run-e2e.sh` and the owning CI workflow, so host builds call `e2e-plugin-ui` before `e2e-plugin-package` and preserve the same order in container/host modes.
- External-style fixture source under `apps/web/e2e/fixtures/plugins/prompt-history-plugin/`, with a distinct `prompt-history-plugin` panel key and public-SDK-only imports. The backend `e2e-plugin-package` target copies its built UI output into `apps/backend/cmd/plugin-fixture/fixture-package/ui/` before packaging; the generated archive is the only installed artifact.
- `apps/backend/Makefile` and the fixture-package build/copy step, with assertions that the archive contains the manifest capability and fixture bundle/panel key.
- `apps/web/e2e/helpers/api-client.ts`, `apps/backend/internal/office/testharness/routes.go`, and deterministic test-only update/delete/turn-transition control helpers.
- Root `Makefile`, `.github/workflows/e2e-tests.yml`, `apps/web/e2e/scripts/run-e2e.sh`, `apps/web/e2e/global-setup.ts`, and `apps/backend/Makefile` all use the same fixture identity path/schema and build order in host and container modes, including upload/download handoff for every shard.
- `apps/web/e2e/global-setup.ts`, package/archive assertions, and shard setup consume the fixture identity, including source/output hashes, manifest capability, minimum version, and panel key; schema-version mismatch fails before specs.
- `apps/web/e2e/README.md` documents the source-build order, `e2e-plugin-ui` prerequisite, identity artifact, and package freshness checks; a stale-invocation check rejects direct package-before-build usage.
- New desktop spec: `apps/web/e2e/tests/plugins/prompt-history-plugin.spec.ts`.
- `apps/web/e2e/helpers/api-client.ts` methods for `PATCH /api/v1/_test/messages/:id`, `DELETE /api/v1/_test/messages/:id`, and `POST /api/v1/_test/turns/:id/complete`.
- New mobile spec: `apps/web/e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts`.
- Shared prompt-history seed/page-object helpers where existing ones already fit.

## Verification

```bash
make -C apps/backend build
cd apps/web && pnpm run build:e2e
make -C apps/backend e2e-plugin-ui
make -C apps/backend e2e-plugin-package
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/plugins/prompt-history-plugin.spec.ts
cd apps/web && pnpm e2e:raw --project=mobile-chrome e2e/tests/plugins/mobile-prompt-history-plugin.spec.ts
cd apps/web && pnpm e2e:raw --project=chromium e2e/tests/task/prompt-history-panel.spec.ts
```

Use the repository's one-worker/shard guards. Do not package or publish the fixture.

## Result

Added one public-SDK-only fixture source, deterministic source/output and
archive identity checks, response-bounded transition controls, and shared
desktop/mobile parity helpers. CI, host, container, package, and global-setup
paths now consume identity schema version 2 after the required fresh build
order.

Verified:

- `cd apps/web && pnpm run build:e2e`
- `make -C apps/backend e2e-plugin-ui`
- `make -C apps/backend e2e-plugin-package`
- Desktop fixture E2E: 1 passed
- Pixel 5 mobile fixture E2E: 1 passed
- Built-in Prompt History E2E: 2 passed
