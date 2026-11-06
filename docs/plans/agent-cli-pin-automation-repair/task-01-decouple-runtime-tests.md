---
id: "01-decouple-runtime-tests"
title: "Make runtime tests independent of shipped pins"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-001
  - REQ-AGENTS-COPILOT-PLUGINS-001
  - REQ-AGENTS-COPILOT-PLUGINS-004
  - REQ-AGENTS-COPILOT-PLUGINS-005
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-001.5
  - AC-AGENTS-RUNTIME-UPDATES-001.7
  - AC-AGENTS-RUNTIME-UPDATES-001.9
  - AC-AGENTS-COPILOT-PLUGINS-001.1
  - AC-AGENTS-COPILOT-PLUGINS-004.3
  - AC-AGENTS-COPILOT-PLUGINS-005.4
  - AC-AGENTS-COPILOT-PLUGINS-005.8
system_design:
  - ../../specs/agents/system-design/runtime-updates-01.md
  - ../../specs/agents/system-design/copilot-profile-plugins-native-qualification.md
---

# Task 01: Make runtime tests independent of shipped pins

## Summary

Repair default-version assertions and plugin fixtures without changing provider
behavior. Demonstrate the regression through a temporary Go catalogue overlay
before changing the permanent tests.

## In scope

- Read default versions from the central catalogue in command and metadata
  assertions. Preserve independent package, flag, and argument-order checks.
- Give Copilot discovery, route, cache, and passthrough success fixtures an
  explicit supported selection through existing dependency injection.
- Cover supported and unknown selected versions independently of the default.
- Inspect the complete six-package failure output for remaining default
  assumptions. Preserve fixed versions used for historical or override cases.

## Out of scope

Runtime pins, adapter admission, production runtime logic, and workflow changes.

## Acceptance

1. The six-package suite passes with both the tracked and synthetic catalogues.
2. Command tests still reject wrong packages, missing exact versions, and wrong
   arguments. Override and migration fixtures retain their intended versions.
3. Plugin success fixtures select an explicit version. Capability and inventory
   failures remain visible without another runtime attempt.

## Verification

Run from the repository root. The synthetic run requires no npm access.
Before the repair, this run must reproduce the fixed-default failures.

```bash
set -euo pipefail
node --test scripts/update-agent-runtime-pins.test.mjs
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/runtime/lifecycle ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/handlers)
node --input-type=module <<'NODE'
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { DEFAULT_CATALOGUE_PATH, readCatalogue, formatCatalogue } from './scripts/update-agent-runtime-pins.mjs';
const root = await fs.mkdtemp(path.join(os.tmpdir(), 'kandev-pin-regression-'));
try {
  const current = await readCatalogue();
  const next = Object.fromEntries(Object.entries(current).map(([name, version]) => {
    const [major, minor, patch] = version.split('+')[0].split('.').map(Number);
    return [name, `${major}.${minor}.${patch + 100}`];
  }));
  const candidate = path.join(root, 'catalogue.json');
  const overlay = path.join(root, 'overlay.json');
  await fs.writeFile(candidate, formatCatalogue(next));
  await fs.writeFile(overlay, JSON.stringify({ Replace: { [DEFAULT_CATALOGUE_PATH]: candidate } }));
  const packages = ['agents', 'managedruntime', 'runtime/lifecycle', 'hostutility', 'settings/controller', 'settings/handlers'];
  const result = spawnSync('go', ['test', '-trimpath', '-count=1', '-tags', 'fts5', `-overlay=${overlay}`, ...packages.map(name => `./internal/agent/${name}`)], {
    cwd: path.resolve('apps/backend'), stdio: 'inherit', timeout: 600000,
  });
  if (result.error) throw result.error;
  process.exitCode = result.status ?? 1;
} finally {
  await fs.rm(root, { recursive: true, force: true });
}
NODE
git diff --check
```

Run `bash scripts/lint-go-changed` with the exact changed Go paths before
completion. Record those paths and the command in Results.

## Files likely touched

- `apps/backend/internal/agent/agents/codex_app_server_test.go`
- `apps/backend/internal/agent/agents/opencode_acp_test.go`
- `apps/backend/internal/agent/settings/controller/agent_discovery_test.go`
- `apps/backend/internal/agent/settings/controller/copilot_plugin_discovery_test.go`
- `apps/backend/internal/agent/settings/handlers/copilot_plugin_discovery_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/copilot_plugins_passthrough_initial_test.go`
- Adjacent Copilot test fixtures only where the six-package run identifies the
  same implicit-default dependency.

## Dependencies

None.

## Risks

Replacing every literal would erase meaningful override and compatibility
coverage. Only assertions about the mutable shipped default use the catalogue.

## Parallelism

`sequential`

## Inputs

- The plan's CI evidence and local overlay reproduction.
- Runtime design: validation fixtures and delivery records.
- Copilot design: qualification/admission and process-boundary verification.
- Backend testing patterns in `.agents/skills/tdd/references/backend-tests.md`.

## Results

Completed on 2026-11-05.

- Codex, OpenCode family/package metadata, OpenCode discovery metadata, and the
  OpenCode automatic-update target now derive mutable shipped defaults from the
  trusted catalogue. Package identities, exact version selection, argument
  order, native-family fixtures, override fixtures, and migration coverage
  remain independent assertions.
- Copilot controller, route, preview, cache, initial passthrough, and review
  success fixtures select `1.0.104` through the existing selection seam.
  Current admission checks runtime capabilities and inventory instead of
  rejecting an unfamiliar version alone.
- The six-package suite passed with the tracked catalogue and with the temporary
  overlay that changed every pin, both with the `fts5` build tag. The pre-repair
  overlay reproduced the reported fixed-default and unsupported-plugin failures.
- The derived target helpers validate through the shared managed-runtime SemVer
  parser, accept build metadata with and without dot components, and keep
  generated targets strictly newer in the same major. A focused table test
  covers plain and metadata-bearing versions.
- `node --test scripts/update-agent-runtime-pins.test.mjs` passed 9/9.
- `bash scripts/lint-go-changed $(git diff --name-only a8b8ac18e0f08cafc48f303eafc88ffd546f31f4...HEAD -- '*.go')`
  passed for every changed Go path.
- `git diff --check` passed.
- PR #5076 backend CI exposed a temporary-directory cleanup race in
  `TestAgentctlResolverSingleflightsConcurrentDownloads`: cancellation releases
  cache leases asynchronously. The test now waits for lease cleanup under the
  cache lock before its temporary directory is removed; runtime behavior is
  unchanged. The affected test and the shared-download cancellation test passed
  100 repetitions with the Go race detector.
