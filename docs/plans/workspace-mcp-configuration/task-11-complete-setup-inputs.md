---
id: "11-complete-setup-inputs"
title: "Complete MCP definition setup inputs"
status: done
wave: 8
depends_on:
  - 10-review-remediation
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-001
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-002
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-003
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.2
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-002.11
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-003.1
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
system_design:
  - ../../specs/agents/system-design/workspace-mcp-resolution-delivery.md
---

# Task 11: Complete MCP definition setup inputs

## Summary

Repair findings 6/8/9: preserve argv, allow removing disabled direct selections,
and complete required non-secret/secret-reference setup. Keep the existing
settings controls, shared Save changes and workspace visibility boundary.

## In scope

Array argument editor; environment/header values and secret references;
marketplace requested-input controls and authoritative install validation;
honest unsupported-variable choices, all locales, docs and desktop/phone E2E.

## Out of scope

Inherited exclusions, add-then-use assignment, built-in management, usage links,
neutral connection styling, secret reveal in the browser, new package runtimes.

## Acceptance

1. Description-only and argument edits preserve exact argument boundaries,
   empty values and whitespace. Disabled unchecked entries remain unselectable;
   a disabled checked direct choice can be cleared and saved.
2. Required publisher inputs block incomplete installs. Only declared non-secret
   overrides and secret IDs cross the install API. Cached metadata stays intact;
   unsupported variable and argument-input shapes are visibly unavailable, not silently ignored. Raw JSON decode retains metadata used by discovery and install validation; fixed named flag/value pairs keep their boundaries.
3. Custom/edit forms preserve/rebind existing inputs through shared save/discard.
   Secret options belong to this workspace/global visibility context. Mobile
   reaches all controls and final actions without overflow or nested scrolling.

## ASCII UI preview

UI-05: Definition editor, desktop. Existing shared Save changes remains primary.

```text
Arguments
  [server.js                         ] [Remove]
  [Hello World                       ] [Remove]
  [Add argument]
Environment variables (or HTTP headers)
  [Name] [Non-secret value] [Remove]
  [Add value]
Secret bindings
  [API_KEY] [Workspace token v] [Remove]
  Bound values are delivered to the task executor.
  [Add secret binding]
```

Phone stacks name/value or name/secret controls in its existing settings
scroller; an argument stays one input plus 44px remove action. No new overlay.

UI-06: Marketplace review, desktop/phone share one bounded review scroller.

```text
Setup choice / runtime name
SETUP INPUTS
  X-Region      [eu-west]
  Authorization [Workspace token v]  [Required]
Complete required inputs before adding this server.
[Add MCP server] (disabled until complete)
```

Required state, separate secret references and scroll ownership are structural;
example names/spacing are illustrative. Unsupported choices keep another usable
choice reachable. Map to AC-001.2/002.3/002.11/006.4/006.5.

## Verification

From root; initial dependencies were refreshed with frozen lockfile after merge.

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig/registry ./internal/agent/settings/handlers -run 'MCP|Marketplace' -count=1)
(cd apps/web && corepack pnpm@9.15.9 exec vitest run components/mcp/ components/settings/workspaces/mcp-definition-form.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 corepack pnpm@9.15.9 run typecheck)
(cd apps && corepack pnpm@9.15.9 --filter @kandev/web lint)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
make build PNPM="corepack pnpm@9.15.9"
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium e2e/tests/settings/workspace-mcp-configuration.spec.ts e2e/tests/task/task-mcp-selection.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-workspace-mcp-configuration.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts)
git diff --check origin/main
```

Add actual editor/review flows for authenticated remote and npm env setup,
metadata-only save preserving arguments, disabled direct selection removal,
missing required values, secret options scoped to workspace, shared discard and
last-field/save reachability. Check 320px phone, desktop and translated copy.

## Files likely touched

`mcp-definition-form.tsx`, new argument/config/secret fields, marketplace UI;
pure `mcp-config-inputs.ts`; typed install API/handler/service input handling;
shared picker; locale catalogs; existing settings/task browser suites and public
automation/MCP guide. Use `useWorkspaceSecretOptions`, not component fetches.

## Dependencies

Task 10. Execution remains in primary; isolated reviewer does not implement.

## Risks

Arguments are not a shell string. Secret defaults from public metadata must
never become literal saved values. Arrays and maps need deep enough cloning to
avoid mutating the source definition or registry cache during editing.

## Parallelism

`sequential`

## Inputs

Independent findings 6/8/9; current setup design and shared metadata/control
patterns. Existing repository secret bindings supply responsive composition.

## Results

Argument, disabled-choice and required-input regressions reproduced failures,
then passed. Full Registry, settings-handler/store and domain suites passed,
including real PostgreSQL coverage. All 36 changed frontend test files passed
(416 tests); frontend lint, localized catalogs and typecheck passed. Web/native/
plugin-fixture builds passed. Current settings E2E passed seven desktop and five
phone cases with retries disabled, including exact argv/value/reference save,
repository metadata and unsupported long-package review containment. The latter
reproduced overflow, then passed after the dialog grid gained a zero minimum.
Final commit/review/publication/CI and merge confirmation belong to the parent.

### Current-main CI fixture integration

The first published-head full frontend shards exposed 36 failures across five
files: an obsolete workspace-tab adjacency assertion, missing MCP form-state
fields in the composition mock, and strict edit/draft transports without the
new legitimate MCP reads. The failures reproduced locally. Fixtures now return
empty definitions/selections only for their known workspace/scope owners;
unexpected request and draft-ownership assertions remain intact. All five
files/38 cases passed, the combined affected suite passed 41 files/454 cases,
and scoped lint and typecheck passed after extracting the MCP read fixture.
These changes affect test scaffolding only; rendered UI and runtime code are
unchanged, so the a57 screenshot captures remain valid for these surfaces.
