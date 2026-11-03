---
id: "06-native-usage"
title: "Report native usage honestly"
status: in_progress
wave: 3
depends_on:
  - 02-subscription-policy
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-014
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-014.1
  - AC-AGENTS-ANTIGRAVITY-ACP-014.2
  - AC-AGENTS-ANTIGRAVITY-ACP-014.3
  - AC-AGENTS-ANTIGRAVITY-ACP-014.4
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 06: Report native usage honestly

## Summary

Keep Antigravity's native token data and account report visible through current task surfaces.
Mark the personal path as subscription usage and distinguish estimated dollar values from additional charges.

## In scope

- Change Antigravity's default billing metadata for the supported personal path without promising effective billing for legacy API-key/remote methods.
- Preserve typed prompt token fields, context occupancy separation, usage persistence, reload, and current deduplication semantics.
- Cover absent/partial fields, repeated context notifications, resumed turns, and child-agent inclusion limits.
- Keep absent provider dollars absent. Use existing pricing estimates only when a model has known pricing.
- Add concise translated subscription cost guidance to existing usage details.
- Pass native `/usage` through current command handling, rendering the native Markdown report without a synthetic token delta or model turn.
- Assert no Google allowance client or Office quota poller is registered.

## Out of scope

New Google HTTP quota APIs, Office allowance bars, inferred reset windows, historical SQLite token import, and a new billing engine.

## Acceptance

1. Supplied native tokens survive existing aggregation and reload without invented fields or duplicate deltas; context and subagent scope remain clear.
2. Subscription metadata and visible estimates do not imply a measured Google charge; unavailable provider dollars/prices remain unavailable.
3. `/usage` renders the native account report in normal task chat without Office or a separate client, and does not fabricate model usage.

## ASCII UI preview

Usage excerpt of [UI-03](plan.md#ui-03-task-human-question-and-usage-details), covering 014.1-014.4.

```text
Existing usage details, desktop and phone
+-------------------------------------------------------+
| Tokens: native values when supplied                   |
| Cost: Estimated, or unavailable without known pricing |
| Estimate is not an additional Google subscription bill|
| Use /usage for the native account allowance report.   |
+-------------------------------------------------------+
```

On phone, use the existing usage sheet/popover behavior with wrapped disclosure and touch targets.
No new quota dashboard or fabricated numeric example is introduced.

## Verification

Run from the repository root after Task 01's pnpm bootstrap.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/agents -run 'TestAntigravity' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'TestAntigravity|TestConvert.*Usage|Test.*UsageUpdate|Test.*Prompt.*Usage' -count=1)
(cd apps/backend && go test -trimpath ./internal/backendapp -run 'TestAntigravityDoesNotRegisterQuotaClient' -count=1)
(cd apps/web && pnpm exec vitest run components/task/chat/conversation-usage-display.test.tsx)
(cd apps/web && pnpm exec eslint components/task/chat/conversation-usage-display.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/conversation-usage.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-conversation-usage.spec.ts)
git diff --check
```

Named Go tests: `TestAntigravityUsage`, `TestAntigravityNativeUsageCommand`, and `TestAntigravityDoesNotRegisterQuotaClient`.
Web component case: `Antigravity subscription cost disclosure`.
Preserve prior usage display E2E scenarios while adding Antigravity cases and reload assertions.
State explicitly whether live native usage excludes subagents. Missing source data is not a zero measurement.
Run changed-Go lint against all changed/new Go files.

## Files likely touched

- `apps/backend/internal/agent/agents/antigravity_acp.go`, `antigravity_acp_test.go`.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/meta_convert.go`, `adapter_updates.go`, `adapter_prompt.go` only where fixture tests identify missing conversion.
- `apps/backend/internal/agentctl/server/adapter/transport/acp/antigravity_usage_test.go` and sanitized synthetic fixtures.
- `apps/backend/internal/backendapp/usage_adapter_test.go` (negative registration regression).
- Existing task usage persistence/aggregation only if new tests reveal lost fields.
- `apps/web/components/task/chat/conversation-usage-display.tsx` and test.
- `apps/web/e2e/tests/chat/conversation-usage.spec.ts`, `mobile-antigravity-usage.spec.ts`.
- Seven locale catalogs; generate Traditional Chinese through the repository tool.

## Dependencies

Task 02 provides the native personal subscription command path and compatibility identity.
Task 03 is needed for live consent, but not deterministic usage fixtures.

## Risks

- Provider usage can be optional and main-trajectory-only. Do not overstate task totals.
- Catalog identifiers can lack pricing. Do not invent dollar cost or equate estimated cost with a bill.
- `/usage` is native rendered text, not a stable machine-readable quota API.

## Parallelism

`sequential`

## Inputs

- Design section Usage and billing.
- Existing typed usage extraction, context conversion, conversation usage UI, and `conversation-usage.spec.ts`.
- Native 1.4.5 `_to_acp_usage` and `/usage` source observations, with live evidence still pending.

## Results

Implemented.

- Antigravity billing metadata now reports subscription usage; no Google allowance client is registered in the Office usage adapter.
- Added `TestAntigravityUsage` covering native token fields, omitted fields, context occupancy separation, repeated context updates, resumed turns, and absent provider dollar cost.
- Added `TestAntigravityNativeUsageCommand` proving `/usage` is forwarded as one normal prompt, renders the native Markdown, and emits no synthetic usage.
- Added the localized subscription cost disclosure to desktop and phone usage details, gated to Antigravity sessions, with `Antigravity subscription cost disclosure` coverage.
- Added `TestAntigravityDoesNotRegisterQuotaClient`.

Verification: targeted backend tests, web component tests, typecheck, i18n check, and changed-Go lint passed.

Live native token scope and `/usage` content remain Task 07 evidence. Native usage is main-trajectory only; absent child-agent tokens are not a zero measurement.

Antigravity usage E2E cases remain open; deterministic backend and component coverage is complete.
