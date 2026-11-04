---
id: "01-recognize-copilot-identity"
title: "Recognize Copilot rich-output identity"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.1
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.13
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.17
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.18
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.6
  - AC-AGENTS-AGENT-RICH-OUTPUT-002.9
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
---

# Task 01: Recognize Copilot rich-output identity

## Summary

Repair the shared parser for the exact Copilot title shape from issue #4800.
Prove native rendering and standalone grouping for both live messages and replay.

## In scope

- Exact `kandev-` prefix recognition in `extractKandevStem`.
- Exact saved version 2 Callout fixture from the plan, with generic name `other`.
- Parser, scanner, registry, full renderer, and mixed-activity grouping tests.
- Desktop and phone browser fixtures for arrival without reload and history reload.

## Out of scope

- ACP dialect changes, metadata rewrites, report schema or copy changes.
- Layout, navigation, controls, feature flags, and issue #4788.

## Acceptance

- The exact saved fixture resolves `show_rich_output`, unwraps direct arguments,
  and renders the Callout title and text live and after reload, once each.
- The report retains its position between ordinary activity groups on desktop
  and phone. Existing pending/error surfaces remain intact.
- Foreign normalized identities and foreign raw providers cannot fall back to
  a Copilot title. Malformed and unregistered names cannot select rich output;
  existing Claude, Codex, Cursor/Grok, and bare-name cases remain green.

## TDD sequence

1. Add `recognizes the Copilot Kandev prefix` to `parse.test.ts`. Run it and
   observe `kandev-show_rich_output` instead of `show_rich_output`.
2. Add the scanner, registry, renderer, and grouping fixtures named in the plan.
   Confirm failures concern dispatch/grouping rather than invalid report data.
3. Add the exact prefix to the shared parser. Preserve all existing guards.
4. Add desktop and phone scenarios using the existing message seeding API.
   Assert arrival while mounted, then reload, plus neighboring activity groups.
5. Run all commands below. Record actual results and mark the work order and
   plan complete only after their acceptance conditions pass.

## ASCII UI preview

UI-01, excerpt from [the full preview](plan.md#ui-01-completed-report-in-task-chat):

```text
Before: [Tool activity > Copilot rich-output JSON]
After:  [Ordinary tools >]
        [Synthetic report: native Callout]
        [Ordinary tools >]
```

Desktop and phone share the inline composition and chronological order.
The phone wraps text inside the viewport and retains transcript scrolling.
Applies to AC-AGENTS-AGENT-RICH-OUTPUT-001.1, .13, .17, .18 and 002.6.

## Verification

Run from the repository root. In a fresh worktree, install dependencies first:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Then run:

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/kandev/parse.test.ts components/task/chat/types.test.ts components/task/chat/messages/kandev-tool-message.test.tsx components/task/chat/messages/kandev-tool-message-rich-output.test.tsx hooks/use-processed-messages-rich-output.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/kandev/parse.ts components/task/chat/messages/kandev/parse.test.ts components/task/chat/types.test.ts components/task/chat/messages/kandev-tool-message.test.tsx components/task/chat/messages/kandev-tool-message-rich-output.test.tsx hooks/use-processed-messages-rich-output.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/copilot-rich-output.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-copilot-rich-output.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/copilot-rich-output-identity
```

Run the managed E2E commands sequentially; they build current source. Record
browser checks as synthetic Copilot metadata coverage, not a real provider run.
Run the PR-documentation coverage preflight against the final changed paths
before delivery, using `.github/scripts/pr-docs.cjs` `validateCoverage`.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/parse.ts`
- `apps/web/components/task/chat/messages/kandev/parse.test.ts`
- `apps/web/components/task/chat/types.test.ts`
- `apps/web/components/task/chat/messages/kandev-tool-message.test.tsx`
- `apps/web/components/task/chat/messages/kandev-tool-message-rich-output.test.tsx`
- `apps/web/hooks/use-processed-messages-rich-output.test.ts`
- `apps/web/e2e/tests/chat/copilot-rich-output-helpers.ts` (new)
- `apps/web/e2e/tests/chat/copilot-rich-output.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-copilot-rich-output.spec.ts` (new)
- `docs/plans/copilot-rich-output-identity/plan.md`
- `docs/plans/copilot-rich-output-identity/task-01-recognize-copilot-identity.md`

## Dependencies

None. One sequential vertical slice; no delegation is authorized.

## Risks

A helper that supplies canonical tool metadata would hide the defect. Preserve
`generic.name: "other"`, Copilot title/content, direct `raw_input`, and the
string acknowledgement result. Do not silently rewrite the fixture.

## Parallelism

`sequential`

## Inputs

- [Issue #4800](https://github.com/kdlbs/kandev/issues/4800), synthetic fixture.
- [Requirements](../../specs/agents/requirements/agent-rich-output.md), criteria above.
- [Agent design](../../specs/agents/system-design/agent-rich-output.md), Provider identity.
- [Shared UI design](../../specs/ui/system-design/kandev-mcp-tool-results.md), Copilot ACP title compatibility.
- Existing Cursor fixture in `kandev-tool-message-rich-output.test.tsx`.
- Existing `rich-output-helpers.ts`, `seedSessionMessage`, and mobile rich-output tests.
- `.agents/skills/tdd/SKILL.md` and `.agents/skills/e2e/SKILL.md` for implementation.

## Results

Complete. The parser regression was observed before the production change, and
the final focused unit suite passes 91 tests across five files.
`NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck` and the targeted
ESLint command pass. The desktop and phone managed E2E commands each pass one
test after rebuilding the backend, frontend, and fixture plugin. The phone test
confirms viewport containment and no document horizontal overflow after replay.
These tests use synthetic Copilot metadata, not a live provider execution.
Specification validation, spec lint, `git diff --check`, and PR documentation
coverage pass.

Subsequent pull request fixup verification: the desktop and phone E2E scenarios
wait for the exact seeded report event before asserting rendered output, and
both pass with retries disabled. `pnpm run typecheck` and ESLint on the three
Copilot rich-output E2E files pass. This is test synchronization only; no
product behavior or specification contract changed. Two unrelated tests were
flaky in the initial CI run but pass in retry-free local reproductions; the
initial results were superseded by a successful updated-head run. After the
failed shards were rerun, the PR waiter reported 63 passed checks, 18 skipped,
and zero failed or pending checks. The three rerun shard reports contained 440
passes and four skips with no retries. The initial walkthrough timeout was
also superseded by the successful updated-head run.
