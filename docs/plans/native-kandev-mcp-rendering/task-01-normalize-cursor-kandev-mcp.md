---
id: "01-normalize-cursor-kandev-mcp"
title: "Normalize Cursor Kandev MCP calls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.1
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
---

# Task 01: Normalize Cursor Kandev MCP Calls

## Summary

Normalize new Cursor MCP calls at the ACP dialect boundary and make the shared
frontend path replay the already-persisted Cursor envelope. Use strict
Red-Green-Refactor so the captured wire and stored-message shapes fail before
the correction and pass without changing native renderer behavior.

## In scope

- Cursor and Grok dialect recognition of `providerIdentifier`, `toolName`, and
  `args`.
- Provider-qualified canonical generic identity and unwrapped arguments.
- Historic `kandev: <tool>_kandev` title parsing and `raw_input.args`
  extraction.
- Shared renderer selection, rich-output standalone grouping, and rendered-card
  regression coverage.
- Existing Claude and Codex compatibility.

## Out of scope

- Rich-output schema, chart, metrics, file block, settings, or copy changes.
- Persistence migration or message rewrite.
- Layout, responsive composition, touch behavior, or new E2E scenarios.
- Native rendering of foreign or unregistered tools.

## Acceptance

- A captured Cursor-style `other` frame for Cursor or Grok normalizes to
  `kandev/show_rich_output_kandev`, stores the version 1 object as generic
  input, and preserves its terminal output.
- The exact historic fixture resolves the registered renderer, remains a
  standalone transcript item, and renders HTML containing
  `data-testid="rich-output"` and the fixture presentation title
  `Build health`.
- Malformed Cursor envelopes, `kandev: Edit`, `mcp__github__...`, foreign
  providers, and unregistered Kandev tools keep the generic path; existing
  Claude and Codex cases continue to pass.

## TDD order

1. Add the exact Cursor adapter fixture and run the focused Go test to observe
   broad `other` identity and wrapped arguments.
2. Add exact frontend fixtures for argument unwrapping, stem extraction,
   registry matching, standalone grouping, and rendered HTML; run focused
   Vitest to observe the expected failures.
3. Add the minimum Cursor dialect parser and wire it through `newACPDialect`.
4. Add the minimum guarded title prefix and `args` unwrap to the shared
   frontend parser.
5. Refactor only after all focused regressions pass, then rerun every changed
   test.

## ASCII UI preview

Excerpt of [UI-01 in the plan](plan.md#ui-01-completed-native-result-in-transcript),
covering `AC-AGENTS-AGENT-RICH-OUTPUT-001.1`:

```text
Before: [Tool activity] > kandev: show_rich_output_kandev > raw JSON
After:  [Standalone native card] Build health
```

Desktop and phone use the same existing inline card composition.

## Verification

```bash
(cd apps/backend && go test ./internal/agentctl/server/adapter/transport/acp -run 'TestCursorMCP')
(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/parse.test.ts components/task/chat/messages/kandev-tool-message.test.tsx components/task/chat/types.test.ts hooks/use-processed-messages-rich-output.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output.spec.ts)
make fmt
make typecheck
make test
make lint
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `pnpm run i18n:ratchet` from `apps/web` only if implementation introduces
or changes user-facing copy; no copy change is planned.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_grok.go`
- `apps/web/components/task/chat/messages/kandev/parse.ts`
- `apps/web/components/task/chat/messages/kandev/parse.test.ts`
- `apps/web/components/task/chat/types.test.ts`
- `apps/web/components/task/chat/messages/kandev-tool-message.test.tsx`
- `apps/web/hooks/use-processed-messages-rich-output.test.ts`
- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/specs/agents/system-design/agent-rich-output.md`
- `docs/specs/ui/system-design/kandev-mcp-tool-results.md`
- `docs/plans/native-kandev-mcp-rendering/plan.md`
- `docs/plans/native-kandev-mcp-rendering/task-01-normalize-cursor-kandev-mcp.md`

## Dependencies

None.

## Risks

- Cursor's provider envelope has no separate MCP marker, so recognition must
  require all three typed fields and preserve provider identity.
- Historic title compatibility must not become a general trust signal for
  arbitrary `kandev:` text.

## Parallelism

`sequential`

The adapter and frontend compatibility paths define one serialized contract and
must be validated together. No subagents are authorized.

## Inputs

- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/specs/ui/system-design/kandev-mcp-tool-results.md`
- Existing Codex MCP normalization and frontend compatibility tests

## Results

Implemented strict Cursor-style MCP normalization for Cursor and Grok, plus
frontend compatibility for persisted `kandev: <tool>_kandev` titles and
`raw_input.args`. Added positive and fail-closed adapter, parser, registry,
grouping, and rendered-card regressions.

Verification results:

- Focused ACP adapter Go tests pass.
- Focused frontend tests pass (five files, 72 tests).
- `make fmt`, `make typecheck`, and `make lint` pass.
- Desktop and mobile rich-output E2E suites pass.
- Specification metadata, specification lint, and `git diff --check` pass.
- `make test` completed with two unrelated environment failures in
  `TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc` and
  `TestHandleLSPStream_MissingBinaryWithoutAutoInstallClosesWithBinaryNotFound`;
  both reproduce in isolation.
