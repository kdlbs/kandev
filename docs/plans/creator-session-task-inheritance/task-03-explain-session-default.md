---
id: "03-explain-session-default"
title: "Explain creator-session default"
status: done
wave: 3
depends_on: ["02-creator-session-resolution"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/mcp-task-agent-profile-default.md"
---

# Task 03: Explain creator-session default

## Acceptance

- The `current_task` option displays as **Creating session profile** and explains
  effective model/options inheritance, precedence, cost, and exclusions.
- Task-mode and external-mode MCP descriptions accurately state their different
  session-context behavior without changing the public input schema.
- Public task documentation matches the shipped setting and fallback chain.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/settings/mcp-task-agent-profile-default-settings.test.tsx && pnpm --filter @kandev/web i18n:check)
(cd apps/backend && go test ./internal/mcp/server)
(node --test scripts/validate-public-docs.test.mjs)
(node scripts/validate-public-docs.mjs)
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/web/components/settings/mcp-task-agent-profile-default-settings.tsx`
- `apps/web/components/settings/mcp-task-agent-profile-default-settings.test.tsx`
- `apps/web/src/locales/*/settings.json`
- `docs/public/tasks-and-workflows.md`

## Dependencies

- Task 02 fixes the exact behavior that this copy describes.

## Parallelism

Sequential. Copy and contract assertions must describe the final resolution
behavior from Task 02.

## Inputs

- Spec: **What**, **API surface**, **Scenarios**, **Out of scope**
- Plan: **MCP contract text**, **Task Actions setting**, **Public documentation**
- Mobile exemplar:
  `apps/web/e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts`

## Output contract

Report changed terminology, public docs type, locale updates, files changed,
exact tests run, results, blockers, risks, and synchronized task/plan status.

## Results

Updated the setting copy and MCP tool descriptions to explain verified
creating-session inheritance, effective model/mode/options, precedence, cost
risk, external fallback, and the cases that suppress runtime copying. The
stored enum remains `current_task`, while the visible label is now
**Creating session profile**. English, pseudo, Portuguese, and Simplified
Chinese catalogs are synchronized, and the public task/workflow guide matches
the behavior.

Files changed:

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/server_test.go`
- `apps/backend/internal/mcp/server/handlers_test.go`
- `apps/web/components/settings/mcp-task-agent-profile-default-settings.test.tsx`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `apps/web/src/locales/pt-pt/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `docs/public/tasks-and-workflows.md`

Verification:

```text
pnpm --filter @kandev/web test -- components/settings/mcp-task-agent-profile-default-settings.test.tsx  PASS (4 tests)
pnpm --filter @kandev/web typecheck  PASS
pnpm --filter @kandev/web i18n:check  PASS (existing locale parity/orphan advisories only)
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs  PASS
rtk go test ./internal/mcp/server -count=1  PASS
```
