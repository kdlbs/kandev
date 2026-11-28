---
id: "01-provider-readiness"
title: "Provider readiness catalog"
status: completed
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-TASKS-MIXED-REPOSITORIES-002
acceptance_criteria:
  - AC-TASKS-MIXED-REPOSITORIES-002.1
  - AC-TASKS-MIXED-REPOSITORIES-002.2
  - AC-TASKS-MIXED-REPOSITORIES-002.3
  - AC-TASKS-MIXED-REPOSITORIES-002.4
  - AC-TASKS-MIXED-REPOSITORIES-002.5
  - AC-TASKS-MIXED-REPOSITORIES-002.6
system_design:
  - ../../specs/tasks/system-design/mixed-repository-selection.md
---

# Task 01: Provider readiness catalog

## Summary

Separate provider readiness from repository listing. Built-ins and plugin registrations expose one workspace-scoped readiness catalog, with safe lifecycle handling.

## In scope

- Combine existing workspace toggles with verified built-in connection status.
- Add optional getAvailability to the SDK and lifecycle wrapper. Missing callbacks remain unknown.
- Expose ready, loading, unavailable, and failed source states independently from search results.
- Update SDK documentation and the in-tree fixture readiness response. Keep existing picker consumers compatible until Task 02.

## Out of scope

Mixed form state, New Subtask, Sets, production plugin repositories, and releases.

## Acceptance

- Disabled, unknown, untested, failed, and unloaded providers never become eligible. Empty successful lists do not affect readiness.
- Workspace and registration changes fence late callbacks and abort obsolete work. Repeated refresh does not share health across workspaces.
- The SDK contract, host wrapper, fixture, and author documentation agree on configured/enabled/tested semantics.

## Verification

Run from the repository root. New test files named here must be created in this work order.
Use TDD and preserve red/green evidence. Install workspace dependencies once as described in the plan.

```bash
(cd apps && pnpm --filter @kandev/plugin-sdk test)
(cd apps && pnpm --filter @kandev/plugin-sdk typecheck)
(cd apps/web && pnpm exec vitest run hooks/domains/integrations/use-remote-repositories.test.tsx lib/plugins/registry-provider-lifecycle.test.ts lib/plugins/host.repository-providers.test.ts)
(cd apps/backend && go test ./cmd/plugin-fixture)
(cd apps/web && pnpm run typecheck)
node scripts/validate-public-docs.mjs
```

Run ESLint on the changed web source and test files. This exact command includes
tracked changes and new files. Record its result and the file list in Results.

```bash
python3 - <<'CHECK_LINT'
import subprocess
from pathlib import Path
changed = subprocess.check_output(['git', 'diff', '--name-only', 'HEAD', '-z']).split(b'\0')
new = subprocess.check_output(['git', 'ls-files', '--others', '--exclude-standard', '-z']).split(b'\0')
paths = sorted({p.decode() for p in changed + new if p})
files = [p for p in paths if p.startswith('apps/web/') and p.endswith(('.ts', '.tsx')) and Path(p).is_file()]
print('\n'.join(files))
if files:
    subprocess.run(['pnpm', 'exec', 'eslint', *[p[len('apps/web/'):] for p in files]], cwd='apps/web', check=True)
CHECK_LINT
```
Do not use all-worker overrides. Run desktop and phone projects sequentially.

## Files likely touched

- `apps/web/hooks/domains/integrations/use-remote-repositories.ts`
- `apps/web/lib/plugins/registry-provider-lifecycle.ts`
- `apps/web/lib/plugins/registry.ts`
- `apps/web/lib/plugins/types.ts`
- `apps/packages/plugin-sdk/src/index.ts`
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js`
- `apps/backend/cmd/plugin-fixture/plugin.go`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/public/plugins-authoring.md`

## Dependencies

None.

## Risks

A successful list is not an authentication test. Do not infer plugin readiness from registration, required config keys, or an empty cached result.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/mixed-repository-selection.md).
- [System design](../../specs/tasks/system-design/mixed-repository-selection.md).
- [Plan and test mapping](plan.md).
- Scoped AGENTS.md, TDD, mobile-parity, and E2E guidance.

## Results

Completed on 2026-09-13.

- Built-in providers now require enabled state plus verified authentication.
- Plugin providers expose the optional configured/enabled/tested readiness
  callback through the lifecycle registry with workspace and generation fencing.
- The picker boundary exposes independent readiness, list, and bounded error
  states. The fixture and public SDK contract use the same semantics.
- `pnpm --filter @kandev/plugin-sdk test`, `pnpm --filter @kandev/plugin-sdk
  typecheck`, affected provider unit tests, `pnpm run typecheck`, `pnpm run
  lint`, `node scripts/validate-public-docs.mjs`, and `go test ./cmd/plugin-fixture`
  passed.
- Production provider adoption remains a release dependency. Existing provider
  repositories must add the readiness callback before their tabs are eligible.
