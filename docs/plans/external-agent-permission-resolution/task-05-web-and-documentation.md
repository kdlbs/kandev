---
id: "05-web-and-documentation"
title: "Web compatibility and documentation"
status: complete
wave: 4
depends_on: ["03-authorized-permission-service"]
plan: "plan.md"
spec: "../../specs/agents/external-permission-resolution.md"
---

# Task 05: Web compatibility and documentation

## Acceptance

- The existing web permission hook sends task/session/request/pending identity while preserving all
  rendered states and option selection; its focused unit test and existing permission E2E pass.
- Settings lists both external permission tools with localized descriptions and correct catalog
  count, without layout or mobile interaction changes.
- Public/internal docs explain examples, PAT/task authorization, live-versus-audit authority,
  immutable options, stale/replay behavior, privacy/redaction, agentctl actions, and WebSocket
  request identity. Public-doc and i18n validators pass.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/task/chat/messages/use-permission-handlers.test.ts lib/settings/external-mcp-tools.test.ts && pnpm --filter @kandev/web lint && cd web && pnpm run typecheck && pnpm run i18n:check
cd ../.. && node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
make build-backend build-web-e2e
cd apps/web && pnpm e2e:raw --project=chromium tests/chat/permission-approval.spec.ts
```

## Files likely touched

- `apps/web/components/task/chat/messages/use-permission-handlers.ts`
- `apps/web/components/task/chat/messages/use-permission-handlers.test.ts`
- `apps/web/lib/settings/external-mcp-tools.ts`
- `apps/web/lib/settings/external-mcp-tools.test.ts`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/pseudo/settings.json`
- `apps/web/src/locales/pt-pt/settings.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `docs/public/automation-and-mcp.md`
- `docs/public/agents-and-profiles.md`
- `docs/backend_agentctl_connectivity.md`
- `docs/WEBSOCKET_API.md`

## Dependencies

Task 03.

## Parallelism

Parallel-safe with task 04 after task 03 because this task owns only web/docs files. User
authorization is still required for parallel agents.

## Inputs

- Spec: web compatibility, public API examples, permissions, failure modes, and out-of-scope rules.
- Docs guidance: `automation-and-mcp.md` is the primary how-to/explanation; `agents-and-profiles.md`
  is the approval-policy explanation/reference.
- Mobile guidance: copy/data-only change in an unchanged Settings surface; focused rendered check is
  required, new mobile E2E is not.

## Risks

- Cached permission messages created before the new field must render safely but cannot be allowed
  to bypass strict resolution.
- The external tool catalog already has documented drift; adding two entries must not conceal
  unrelated missing tools or assert a false exact count.

## Output contract

Report web payload compatibility, catalog/i18n and public-doc type, focused rendered/E2E evidence,
exact commands/results, files changed, blockers/risks, then update task/plan status.

## Results

Completed 2026-08-11.

- The web permission handler now includes the full task/session/request/pending/option tuple and
  safely refuses legacy cached requests that lack `request_id`; focused hook and catalog tests pass
  (16 tests).
- Both external MCP tools are listed with localized descriptions in English, pseudo, Portuguese,
  and Simplified Chinese. This is a data/copy-only change with no layout or mobile interaction
  changes.
- Public and internal docs cover the live-authority boundary, PAT/task authorization, immutable
  option identity, stale/replay errors, redaction, agentctl actions, and WebSocket request identity.
- `pnpm --filter @kandev/web lint`, `pnpm run typecheck`, and `pnpm run i18n:check` passed. The i18n
  check reported only its existing advisory locale-parity and orphan-key notices.
- `node --test scripts/validate-public-docs.test.mjs` passed (1 test) and
  `node scripts/validate-public-docs.mjs` validated 41 pages.
- `make build-backend` and `make build-web-e2e build-e2e-plugin-package` passed.
- `pnpm e2e:raw --project=chromium tests/chat/permission-approval.spec.ts` passed (3 tests) after
  installing only the repository-pinned Playwright Chromium binary.
