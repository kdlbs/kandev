---
id: "02-setup-validation"
title: "Setup, protocol and documentation"
status: done
wave: 2
depends_on: ["01-native-runtime"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MINIMAX-001
  - REQ-AGENTS-MINIMAX-002
  - REQ-AGENTS-MINIMAX-003
acceptance_criteria:
  - AC-AGENTS-MINIMAX-001.2
  - AC-AGENTS-MINIMAX-001.3
  - AC-AGENTS-MINIMAX-002.1
  - AC-AGENTS-MINIMAX-002.2
  - AC-AGENTS-MINIMAX-002.3
  - AC-AGENTS-MINIMAX-003.1
  - AC-AGENTS-MINIMAX-003.2
system_design:
  - ../../specs/agents/system-design/minimax-code.md
---

# Task 2: Setup, protocol and documentation

## Summary

Localized install/login/passthrough guidance, desktop/phone setup and selected-model task launch checks, native protocol probes and public setup documentation.

## In scope

Localized install/login/passthrough guidance, desktop/phone setup and selected-model task launch checks, native protocol probes and public setup documentation.

## Out of scope

New transports, Office routing and cross-executor token copying.

## Acceptance

- The scoped native behavior satisfies all linked criteria.
- Task-defined tests pass with no fabricated live subscription evidence.
- Existing credential and permission ownership remains intact.

## ASCII UI preview

UI-01/UI-02 from [the plan](plan.md#ascii-ui-preview) apply unchanged.

```text
MiniMax [Login required] [Login]
Profile start model: [MiniMax-M3 - thinking] [Save]
```

## Verification

```bash
(cd apps/web && pnpm run i18n:check && pnpm run typecheck)
(cd apps/web && pnpm exec vitest run components/settings/agent-login-dialog.test.tsx components/settings/profile-form-fields.test.tsx components/settings/installed-agent-card.test.tsx hooks/domains/settings/use-dynamic-models.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/minimax-agent.spec.ts)
(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/settings/mobile-minimax-agent.spec.ts)
node scripts/validate-public-docs.mjs
node --test scripts/validate-public-docs.test.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/settings/agent-login-dialog.tsx`
- `apps/web/components/settings/agent-login-dialog.test.tsx`
- `apps/web/components/settings/pty-terminal-dialog.tsx`
- `apps/web/components/settings/install-agent-card.tsx`
- `apps/web/components/settings/installed-agent-card.tsx`
- `apps/web/components/settings/profile-permission-toggles.tsx`
- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/e2e/helpers/minimax-acp-fixture.mjs`
- `apps/web/src/locales/*/agents.json`
- `apps/web/e2e/tests/settings/*minimax*`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 01.

## Risks

CLI/ACP syntax and credential-location differences; live OAuth unavailable.

## Parallelism

`sequential`

## Inputs

[Requirement](../../specs/agents/requirements/minimax-code.md),
[system design](../../specs/agents/system-design/minimax-code.md), existing native
ACP agents, shared protocol tests and current official upstream source.

## Results

Passed: desktop and phone native login/model profile E2E flows; login refresh ordering/failure and single-probe ownership unit tests; frontend typecheck, changed-file ESLint and complete i18n checks; public documentation validator and its 62 tests; specification/catalog/harness linters and documentation coverage preflight.

Published 0.5.10 probes used disposable HOME/data/workspace directories. Unauthenticated session/new returned auth_required. An explicit local BYOK fixture verified model selection/rejection, text streaming, MCP initialize/tools/list/tools/call, active cancellation and session load after cancellation. No live subscription/OAuth consent test was possible. Native permission-mode selection succeeded, but no request_permission was emitted for these tool calls; shared ACP handler tests provide permission regression evidence.

Desktop/phone screenshots were captured from the deterministic ACP peer, not from a live model account. Phone testing first exposed overflowing long command content; the quick terminal and bounded preview fixed it. Closing MiniMax login from an agent card now refreshes native models before rescanning; the profile auth panel retains ownership of its existing capability refresh and performs one probe.

Review remediation extends the real installation job through an isolated npm fixture and exercises filesystem discovery with E2E availability bypass disabled. The desktop/phone flows check Portuguese install and passthrough copy, then launch an actual native ACP task and assert the saved model reaches the fixture's prompt. Full login commands wrap on phones. Technical command/path values use interpolation so pseudo localization preserves their bytes. The MiniMax docs explicitly state the existing POSIX login-terminal boundary and native Windows manual-login environment requirements.
