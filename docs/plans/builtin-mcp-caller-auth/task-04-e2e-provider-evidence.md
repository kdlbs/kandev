---
id: "04-e2e-provider-evidence"
title: "E2E and provider compatibility evidence"
status: done
wave: 4
depends_on:
  - "03-backend-toggle-delivery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-001
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-002
  - REQ-AGENTS-BUILTIN-MCP-CALLER-AUTH-003
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-001.3
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-002.4
  - AC-AGENTS-BUILTIN-MCP-CALLER-AUTH-003.1
system_design:
  - ../../specs/agents/system-design/builtin-mcp-caller-auth.md
---

# Task 04: E2E and provider compatibility evidence

## Summary

Prove the end-to-end clarification flow under enforcement with the mock agent.
Record which real providers send the credential, and document the toggle for
operators. This evidence decides any later production promotion.

## In scope

- Run the clarification E2E specs under the E2E profile, where the toggle is on.
- For each installed provider in the plan's compatibility matrix, run an
  `/acp-debug` probe with enforcement on. Record whether the built-in server
  connects, in `docs/plans/builtin-mcp-caller-auth/compatibility-evidence.md`.
  Include `atomica-code` 1.0.102.
- Document `KANDEV_FEATURES_BUILTIN_MCP_CALLER_AUTH` in `docs/public/configuration.md`
  through `/docs-maintainer`.

## Out of scope

- Changing the production default.
- Fixing third-party agents that drop headers.

## Acceptance

- The clarification E2E specs pass with enforcement on.
- The evidence file lists every matrix row as verified, failed, or
  not verified, with the probe command and date.

## Verification

```bash
(cd apps/web && e2e/scripts/run-e2e.sh --host --shards 1 --project chromium -- e2e/tests/chat/clarification.spec.ts e2e/tests/chat/clarification-lifecycle-turn-shadow.spec.ts e2e/tests/chat/clarification-submit-failure.spec.ts)
(python3 scripts/lint-spec-files.py --all && python3 scripts/list-docs.py validate)
(git diff --check -- docs/plans/builtin-mcp-caller-auth docs/public/configuration.md)
```

## Files likely touched

- `docs/plans/builtin-mcp-caller-auth/compatibility-evidence.md`
- `docs/public/configuration.md`
- E2E specs only if a fixture needs the credential explicitly

## Dependencies

Task 03

## Risks

- Some providers are not installed locally. Record them as not verified rather
  than inferring support.

## Parallelism

`sequential`

## Inputs

- Plan: Provider compatibility matrix, E2E tests.
- `/acp-debug`, `/e2e`, `/docs-maintainer`.

## Results

- Clarification E2E specs: 36 passed under the E2E profile (enforcement on).
- Provider evidence: [compatibility-evidence.md](compatibility-evidence.md).
- Toggle documented in `docs/public/configuration.md`.
