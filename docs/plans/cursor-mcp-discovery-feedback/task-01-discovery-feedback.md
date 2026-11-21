---
id: "01-discovery-feedback"
title: "Explain Cursor MCP discovery failures"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-002
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-002.8
  - AC-AGENTS-MCP-PREP-002.9
  - AC-AGENTS-MCP-PREP-002.10
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 01: Explain Cursor MCP discovery failures

## Summary

Use the recorded provider and failure code to label Cursor discovery and
explain failed discovery in the task transcript. The current renderer omits
details when a failed discovery row has no server ID.

## In scope

- Use the existing provider and kind fields to select the Cursor label in
  `agentMcpStepLabel`. Apply the existing suffix when the row has a server ID.
- In the failed row, render a small `CursorMcpDiscoveryDetails` component with
  the shared `Collapsible`, standard `Button`, and `task:details` label. Use
  an exact lookup of translation keys and one shared message for skipped
  Marketplace imports. Use the existing row dispatch for per-server recovery.
- Translate new keys in English, Portuguese, Japanese, Korean and Simplified
  Chinese. Generate both Traditional Chinese catalogs and the pseudo-locale.
- Update **Choose and prepare local Cursor MCP servers** in the public agent
  guide with the row label, disclosure, and effect of a Marketplace timeout.

## Acceptance

1. Cursor discovery rows use the provider-specific label and show the server
   suffix when present. Other providers use the generic label. The mixed
   result keeps both the discovery error and successful verification visible.
2. A failed Cursor discovery row without an ID exposes a collapsed **Details**
   button. Keyboard activation and phone taps open and close it. The button
   reports its expanded state and keeps focus.
3. Live and hydrated rows explain the recorded code through the same
   translations. The timeout explains the limit and skipped imports. Missing
   or unknown codes use the general message. The text wraps on phones.

## ASCII UI preview

**UI-01** · Task conversation, expanded environment setup. The
[full preview](plan.md#ascii-ui-preview) includes the current and collapsed
desktop states. The work order covers AC-AGENTS-MCP-PREP-002.8,
AC-AGENTS-MCP-PREP-002.9 and AC-AGENTS-MCP-PREP-002.10.

```text
Desktop, expanded
  [x] Discover Cursor MCP servers
      [Details ^]
      Checking enabled Cursor Marketplace plugins timed out (5-second limit).
      Kandev skipped Marketplace plugin imports for this setup attempt.
      Other configured MCP servers can still connect.
  [ok] Verify connection: kandev-diagnostics

Phone, expanded
  [x] Discover Cursor MCP servers
      [ Details ^               ]
      Checking enabled Cursor
      Marketplace plugins timed
      out (5-second limit).
      Kandev skipped Marketplace
      plugin imports for this
      setup attempt. Other
      configured MCP servers
      can still connect.
  [ok] Verify connection:
       kandev-diagnostics
```

Use the existing conversation scroller for this region. The desktop trigger
measures 28px at the standard root font size. The phone and coarse-pointer
trigger measures at least 44px. On phones, use the row's content width and
wrap translations within it. Compare the rendered layout with the preview.

## Regression scenarios

- In `agent-mcp-preparation.spec.ts`, add **explains a Cursor MCP discovery
  timeout during setup and after reload**. Send a live failed discovery step
  without a server ID through `routeGatewayNotifications`. Open **Details** by
  keyboard and check the timeout and its effect. In the same case, check a
  credential failure and an unknown reason. Seed the completed result with
  the timeout and successful verification, then reload and check both rows.
  Check the 44px target in a narrow fine-pointer viewport.
- In `mobile-agent-mcp-preparation.spec.ts`, add **opens Cursor MCP discovery
  timeout details on phone**. Seed the same result. Tap to open and close the
  explanation. Check the 44px target and wrapping against UI-01.
- Update the successful discovery label assertions in existing desktop and
  phone MCP recovery tests. Rerun both full focused spec files.

## Verification

Run from the repository root. For a fresh worktree, run
`pnpm --dir apps install --frozen-lockfile` first. Add the browser regression
and run the red check before production edits. Expect the Cursor label
assertion to fail on current source.

```bash
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/session/agent-mcp-preparation.spec.ts -- --grep 'explains a Cursor MCP discovery timeout during setup and after reload' --retries=0
```

After translating the source catalogs, generate Traditional Chinese and pseudo.

```bash
pnpm --dir apps/web run i18n:zh-hant
pnpm --dir apps/web run i18n:pseudo
```

Run the desktop and phone Playwright commands in order. The phone run uses the
fresh desktop build from the same checkout.

```bash
(cd apps/web && pnpm exec vitest run components/session/prepare-progress.test.tsx components/task/agent-mcp-prepare-actions.test.tsx)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/session/prepare-progress.tsx components/session/cursor-mcp-discovery-details.tsx)
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
pnpm --dir apps/web run i18n:ratchet
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/session/agent-mcp-preparation.spec.ts -- --retries=0
scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-agent-mcp-preparation.spec.ts -- --retries=0
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the repository coverage validator for the final working diff.

```bash
node <<'JS'
const { execFileSync } = require("node:child_process");
const { readFileSync } = require("node:fs");
const { validateCoverage } = require("./.github/scripts/pr-docs.cjs");
const paths = [
  "docs/specs/agents/requirements/agent-mcp-preparation.md",
  "docs/specs/agents/system-design/agent-mcp-preparation.md",
  "docs/plans/cursor-mcp-discovery-feedback/plan.md",
  "docs/plans/cursor-mcp-discovery-feedback/task-01-discovery-feedback.md",
];
const changedFiles = [
  ...execFileSync("git", ["diff", "--name-only", "-z", "HEAD"], { encoding: "utf8" }).split("\0"),
  ...execFileSync("git", ["ls-files", "--others", "--exclude-standard", "-z"], { encoding: "utf8" }).split("\0"),
].filter(Boolean);
const fileContents = Object.fromEntries(paths.map((path) => [path, readFileSync(path, "utf8")]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== "covered") process.exitCode = 1;
JS
```

## Files likely touched

- `apps/web/components/session/prepare-progress.tsx`
- `apps/web/components/session/cursor-mcp-discovery-details.tsx` (new)
- `apps/web/lib/prepare/cursor-mcp-discovery-reason.ts` and its test
- `apps/web/src/locales/{en,pt-pt,ja,ko,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
- `apps/web/e2e/helpers/agent-mcp-preparation.ts`
- `apps/web/e2e/helpers/ws-capture.ts`
- `apps/web/e2e/tests/session/agent-mcp-preparation.spec.ts`
- `apps/web/e2e/tests/session/mobile-agent-mcp-preparation.spec.ts`
- `docs/public/agents-and-profiles.md`
- This work order and its parent plan for execution results.

## Dependencies

None.

## Risks

- Use the same session ID and attempt markers for the live updates and saved
  result. Wait for the gateway before sending the first update.
- Treat `unavailable` as general discovery feedback. This code can also come
  from an error before the Marketplace lookup.
- Match and translate reason codes exactly. Otherwise an unknown code can
  select the wrong copy or expose an internal string.

## Parallelism

`sequential`

## Inputs

- [Agent MCP preparation requirements](../../specs/agents/requirements/agent-mcp-preparation.md#req-agents-mcp-prep-002-seamless-launch-preparation)
- [Agent MCP preparation design](../../specs/agents/system-design/agent-mcp-preparation.md#cursor-discovery-feedback)
- [Existing prepare event mapper](../../../apps/web/lib/ws/handlers/executor-prepare.ts)
- [Saved prepare-result mapper](../../../apps/web/lib/state/slices/session-runtime/prepare-result.ts)
- [MCP preparation fixture](../../../apps/web/e2e/helpers/agent-mcp-preparation.ts)
- [Gateway notification helper](../../../apps/web/e2e/helpers/ws-capture.ts)
- [GitHub issue #5407](https://github.com/kdlbs/kandev/issues/5407)

## Results

Implemented Cursor-specific discovery labels and the inline failure disclosure.
Exact reason lookup covers the known inventory codes and sends missing,
`unavailable`, and unknown codes to the general message. Live progress, saved
results, and server recovery share the existing preparation row.

Verification passed:

- The new Chromium regression failed before implementation at the expected
  provider-label assertion, then passed after the fix.
- `pnpm exec vitest run components/session/prepare-progress.test.tsx components/task/agent-mcp-prepare-actions.test.tsx lib/prepare/cursor-mcp-discovery-reason.test.ts` passed (38 tests).
- Focused Oxlint, `pnpm run typecheck`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- The complete Chromium preparation spec passed (3 tests), followed by the
  complete Pixel 5 preparation spec (3 tests).
- Public-doc tests and validation, specification catalog validation and lint,
  repository coverage validation, and `git diff --check` passed.
