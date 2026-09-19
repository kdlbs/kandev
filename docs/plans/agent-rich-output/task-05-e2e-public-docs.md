---
id: "05-e2e-public-docs"
title: "E2E and public documentation"
status: done
wave: 5
depends_on: ["04-native-blocks-mobile"]
plan: "plan.md"
spec: "../../specs/agents/requirements/agent-rich-output.md"
---

# Task 05: E2E and public documentation

## Acceptance

1. Desktop and `mobile-chrome` Playwright tests exercise the real inline
   Kandev MCP call, native rendering, file interaction, responsive geometry,
   and reload persistence.
2. Public MCP reference documents availability, schema, limits, lifecycle,
   and plain-text-first guidance.
3. Every task result and plan verification result records exact final commands
   and outcomes.

## Verification

```sh
(
  set -e
  cd apps/web
  pnpm e2e:run --project chromium tests/chat/rich-output.spec.ts -- --retries=0
  pnpm e2e:run --project mobile-chrome tests/chat/mobile-rich-output.spec.ts -- --retries=0
  cd ../..
  node --test scripts/validate-public-docs.test.mjs
  node scripts/validate-public-docs.mjs
  git diff --check
)
```

## Files likely touched

- `apps/web/e2e/tests/chat/rich-output.spec.ts`
- `apps/web/e2e/tests/chat/mobile-rich-output.spec.ts`
- `docs/public/automation-and-mcp.md`
- `docs/plans/agent-rich-output/*.md`
- `docs/specs/agents/requirements/agent-rich-output.md`

## Dependencies

Task 04.

## Parallelism

Sequential. Browser proof depends on final backend and frontend artifacts.

## Inputs

- Spec scenarios and out-of-scope boundary.
- Mock-agent `e2e:mcp:kandev:<tool>({...})` directive.
- Existing chat and mobile file-viewer E2E patterns.

## Output contract

Report discovered test counts, desktop/mobile outcomes, generated artifacts,
cleanup evidence, docs validation, known risks, and synchronized task/plan
status.

## Results

- Desktop Chromium: 1 rich-output test passed. It exercises the real mock-agent
  MCP directive, all three blocks, reload persistence, explicit preview, and
  the existing desktop file tab.
- Mobile Chrome: 1 rich-output test passed. It proves zero document overflow,
  chart containment, 44px actions, explicit preview, and the native mobile
  viewer.
- `docs/public/automation-and-mcp.md` now documents scope, the version 1
  contract and limits, file lifecycle, Markdown-table guidance, and the
  portable MCP versus MCP Apps boundary.
- Focused E2E sleep lint passed. The public-doc test command passed 61 tests,
  and the page validator accepted all 41 published pages.
- `git diff --check` passed. The E2E runner left no live process or
  `e2e/test-results` directory; Git status contains only intended source,
  test, locale, spec, plan, decision, and public-doc changes.
