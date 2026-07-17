---
id: "03-e2e-and-verification"
title: "Verify ACP configuration summary workflows"
status: done
wave: 3
depends_on: ["01-backend-contract-and-baseline", "02-task-selector-ux"]
plan: "plan.md"
spec: "../../specs/ui/acp-model-configuration-summary.md"
---

# Task 03: Verify ACP Configuration Summary Workflows

## Acceptance

- Desktop task chat proves compact changed-value rendering and provider descriptions when opened.
- Agent-profile settings prove the closed selector still lists all values.
- Mobile task chat proves every option and description is reachable without hover, and restart coverage proves the baseline survives backend recreation where the fixture supports it.

## Verification

```bash
cd apps/web && pnpm e2e:run --project chromium tests/settings/agent-profile-acp.spec.ts tests/chat/model-selector-error.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-model-selector.spec.ts
make fmt
node apps/web/scripts/generate-release-notes.mjs
node apps/web/scripts/generate-changelog.mjs
make typecheck
make test
make lint
```

## Files likely touched

- `apps/web/e2e/tests/settings/agent-profile-acp.spec.ts`
- `apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`
- Existing task-chat desktop model selector spec or a focused new spec

## Dependencies

Tasks 01 and 02.

## Inputs

- All spec scenarios.
- Existing ACP profile and mobile chat E2E fixtures.

## Output contract

Report desktop/mobile workflows exercised, exact commands and outcomes, artifacts for failures, and any unverified restart or visual behavior.
