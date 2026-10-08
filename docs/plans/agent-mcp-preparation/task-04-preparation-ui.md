---
id: "04-preparation-ui"
title: "Profile and task preparation interface"
status: complete
wave: 2
depends_on: ["01-profile-contracts"]
plan: "plan.md"
requirements:
  - REQ-AGENTS-MCP-PREP-001
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
acceptance_criteria:
  - AC-AGENTS-MCP-PREP-001.1
  - AC-AGENTS-MCP-PREP-001.2
  - AC-AGENTS-MCP-PREP-001.5
  - AC-AGENTS-MCP-PREP-002.3
  - AC-AGENTS-MCP-PREP-003.1
  - AC-AGENTS-MCP-PREP-003.2
  - AC-AGENTS-MCP-PREP-003.3
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
---

# Task 04: Profile and task preparation interface

## Summary

Profile and task preparation interface according to the linked requirements and design.

## In scope

Own web profile draft/save/normalization/UI, discovery client, preparation payload/store/rendering, typed recovery client/task terminal navigation, locale catalogs and focused unit/browser tests. Do not regenerate Task01 contract or edit backend. Preserve existing profile toggle behavior. Stale refresh cannot mutate another profile; unavailable saved IDs stay visible. Task actions use agreed typed backend routes, never manufacture arbitrary commands. Desktop/phone support and all localization are required. Use existing managed E2E fixture patterns, one worker and no full-suite overlap.

## Out of scope

Other work-order ownership, commits, personal credential changes and unrelated refactors.

## Acceptance

- Satisfy every referenced acceptance criterion within this boundary.
- Add failing behavior tests first, implement, then pass targeted checks.
- Preserve existing dirty work and report exact validation evidence.

## ASCII UI preview

UI-01: Agent profile settings, selected mode (AC-AGENTS-MCP-PREP-001.1, 001.2, 001.5).

```text
Desktop: Settings > Agent profile
 MCP preparation                         [Refresh]
 [x] Import enabled native MCP servers
 Mode: ( ) Inherit enabled  (*) Selected only
 Atlassian
   [x] Atlassian MCP        Credentials available
 Harness
   [ ] Figma               No reusable credentials
 [x] Reuse existing credentials
                                      [Discard] [Save]

Phone: Agent profile
 MCP preparation
 [Refresh discovery                   ]
 Import native servers             [on]
 Mode [Selected only                 v]
 Atlassian
 [x] Atlassian MCP
     Credentials available
 Reuse credentials                 [on]
 [Discard]                     [Save]
```

Rows stack on phone with 44px targets and one page scroll owner. Loading,
empty and unavailable discovery have distinct feedback. Saved missing IDs
remain selectable unavailable rows; refresh errors never clear selection.

UI-02: Task workspace preparation (AC-AGENTS-MCP-PREP-002.3, 003.1-003.3).

```text
Desktop: Workspace preparation
 [ok] Prepare environment
 [ok] Discover agent MCP servers
 [ok] Apply profile selection
 [ok] Reuse existing credentials
 [ok] Approve Atlassian
 [!] Verify Atlassian: authentication required
      [Authenticate] [Retry connection]

Phone: Workspace preparation
 [!] Atlassian
     Authentication required
 [Authenticate                       ]
 [Retry connection                   ]
             | opens existing terminal navigation
 Native login terminal
 <provider browser consent flow>
```

Actions stay outside chat. Pending actions disable duplicate requests, failures
are localized, results survive reload. Structure and mobile accessibility are
required; exact wording/spacing is illustrative and uses existing primitives.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

```bash
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/settings/cursor-plugin-mcp.spec.ts e2e/tests/session/agent-mcp-preparation.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-cursor-plugin-mcp.spec.ts e2e/tests/session/mobile-agent-mcp-preparation.spec.ts)
```

## Files likely touched

- `apps/web/components/settings/profile-form-fields.tsx`
- `apps/web/components/settings/agent-profile-page-state.ts`
- `apps/web/components/session/prepare-progress.tsx`
- `apps/web/lib/ws/handlers/executor-prepare.ts`
- `apps/web/lib/state/slices/session-runtime/types.ts`
- `apps/web/lib/api/domains/user-shell-api.ts`
- `apps/web/src/locales/`

## Dependencies

01-profile-contracts

## Risks

See plan compatibility and concurrency risks; escalate contract changes to coordinator.

## Parallelism

parallel-safe after Task 01 freezes shared contracts; disjoint implementation ownership.

## Inputs

- [Requirements](../../specs/agents/requirements/agent-mcp-preparation.md).
- [System design](../../specs/agents/system-design/agent-mcp-preparation.md).
- Existing scoped AGENTS.md, TDD skill and neighboring test patterns.

## Results

Implementation completed on 2026-09-28. Focused web verification passed:

- Combined profile, preparation and recovery suite: 95 tests across 12 files passed with `pnpm exec vitest run hooks/domains/settings/use-agent-mcp-discovery.test.tsx components/settings/cursor-mcp-profile-selection.test.tsx components/settings/cursor-mcp-selection.test.ts lib/api/domains/agent-profile-normalize.test.ts components/settings/agent-profile-reconciliation.test.ts components/settings/agent-profile-dirty.test.ts components/session/prepare-progress-status.test.ts lib/state/slices/session-runtime/prepare-result.test.ts lib/ws/handlers/executor-prepare.test.ts components/task/use-passthrough-terminal.test.ts components/task/agent-mcp-prepare-actions.test.tsx lib/api/domains/session-mcp-actions.test.ts`.
- `pnpm run typecheck`: passed.
- `pnpm run i18n:check`: passed; all six locale catalogs are complete and the pseudo catalog is synchronized.
- `pnpm run i18n:ratchet`: passed; 2 added and 21 modified files are clean, with the guard allowlist intact.
- `pnpm exec eslint components/task/ws-reconnect.ts components/task/use-passthrough-terminal.test.ts components/task/agent-mcp-prepare-actions.tsx components/task/agent-mcp-prepare-actions.test.tsx lib/api/domains/session-api.ts lib/api/domains/session-mcp-actions.test.ts components/settings/cursor-mcp-profile-selection.tsx components/settings/cursor-mcp-selection.ts components/settings/cursor-mcp-profile-selection.test.tsx`: passed with no warnings.

Historical 2026-09-28 desktop Playwright command:

```bash
GOCACHE=/private/tmp/kandev-cursor-review-go-cache pnpm e2e:run --host --project chromium e2e/tests/settings/cursor-plugin-mcp.spec.ts e2e/tests/session/agent-mcp-preparation.spec.ts
```

The host backend and pseudo-locale Vite asset builds passed. All 4 Chromium tests
then failed before assertions because headless Chromium exited with
`bootstrap_check_in org.chromium.Chromium.MachPortRendezvousServer: Permission denied (1100)`
and SIGTRAP. The retained error contexts are:

- `apps/web/e2e/test-results/session-agent-mcp-preparat-15336-retries-in-the-same-session-chromium/error-context.md`
- `apps/web/e2e/test-results/settings-cursor-plugin-mcp-0e1de-Ds-even-after-refresh-fails-chromium/error-context.md`
- `apps/web/e2e/test-results/settings-cursor-plugin-mcp-de164-r-a-Cursor-strategy-profile-chromium/error-context.md`
- `apps/web/e2e/test-results/settings-cursor-plugin-mcp-ea0e8-nce-for-non-Cursor-profiles-chromium/error-context.md`

Historical 2026-09-28 mobile command:

```bash
GOCACHE=/private/tmp/kandev-cursor-review-go-cache pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-cursor-plugin-mcp.spec.ts e2e/tests/session/mobile-agent-mcp-preparation.spec.ts
```

The mobile attempt was stopped during its backend build, before Playwright
started, at the coordinator's direction. It uses the same Chromium binary that
failed to launch in the desktop run, so mobile E2E remains blocked and is not
reported as passed.


PR #4025 remediation on 2026-09-30 isolates terminal callbacks by socket
identity. A replaced socket's late open or close cannot overwrite/dispose the
active attachment or disconnect its successor.
`terminal-websocket-lifecycle.test.tsx` reproduces that handoff and also checks
that closing the active socket still disposes its attachment and disconnects.
Desktop and mobile authentication assertions select the visible terminal panel.
The affected fork-PR scenario now waits for the established chat-idle signal
before asserting the response, rather than racing agent startup.

From `apps/web`, focused verification passed:

```bash
pnpm exec vitest run components/task/terminal-websocket-lifecycle.test.tsx components/task/use-passthrough-terminal.test.ts components/session/prepare-progress-status.test.ts lib/ws/handlers/executor-prepare.test.ts components/task/agent-mcp-prepare-actions.test.tsx lib/state/slices/session-runtime/prepare-result.test.ts
pnpm run typecheck
pnpm run lint
```

The focused Vitest run passed 44 tests in six files. Managed Docker Chromium
provides browser verification despite the host's Mach-port restriction; fresh
pushed-head CI remains an external verification step. These fixes restore
existing terminal and preparation behavior, so public documentation is unchanged.


PR #4025 remediation verification on 2026-09-30: all 16 desktop scenarios
in the setup-script, GitHub URL task-creation and MCP-recovery files passed
under managed Docker Chromium. This supersedes the historical host browser
launch limitation for these scenarios. The task file's describe-level retry was
temporarily disabled for verification and restored afterward.

From `apps/web`:

```bash
pnpm e2e:run --docker -- tests/task/create-task-github-url.spec.ts tests/session/setup-script-progress.spec.ts tests/session/agent-mcp-preparation.spec.ts --retries=0
```


Latest-main integration on 2026-09-30 preserves profile-context model
discovery, the extracted capabilities UI and the existing Cursor MCP preferences
and selection fields. The permission-toggle extraction remains shared, and
profile HTTP validation follows main's extracted handler file. The focused
frontend suite expanded to 129 passing tests in 16 files, including model
capabilities, profile forms, MCP selection, preparation and terminal handoff.
The profile-form tests passed again after splitting the MCP assertions into
their own describe group to keep the merged test within the lint limit.


Latest-base browser verification on 2026-09-30 passed seven focused desktop
scenarios and all three mobile profile/recovery scenarios. The desktop task
file's describe-level retry was disabled for that run and restored afterward.
From `apps/web`:

```bash
pnpm e2e:run --docker -- tests/task/create-task-github-url.spec.ts tests/session/setup-script-progress.spec.ts tests/session/agent-mcp-preparation.spec.ts tests/settings/cursor-plugin-mcp.spec.ts --grep "opens the exact DB-backed|keeps setup script failure|two tasks from the same PR URL|starts a target-attached fork PR|Cursor plugin MCP" --retries=0
pnpm e2e:run --docker --no-build --project mobile-chrome -- tests/session/mobile-agent-mcp-preparation.spec.ts tests/settings/mobile-cursor-plugin-mcp.spec.ts --retries=0
```

The managed desktop run rebuilt backend binaries, web assets and plugin
fixtures before the mobile run reused them. These results supersede the
historical desktop/mobile browser launch limitation for these files.
Latest-base `pnpm run typecheck`, `pnpm run lint` and `pnpm run i18n:check` passed.
Harness tests (19), specification-linter tests (36), full harness/specification
lint and the documentation catalog (334 decisions, 1264 specs) also passed.
Remote checks and reviews still require fresh evidence after delivery.


The expanded frontend verification command from `apps/web` was:

```bash
pnpm exec vitest run components/settings/profile-form-fields.test.tsx components/settings/profile-model-options.test.tsx hooks/domains/settings/use-profile-model-capabilities.test.tsx components/settings/cursor-mcp-profile-selection.test.tsx components/settings/cursor-mcp-selection.test.ts hooks/domains/settings/use-agent-mcp-discovery.test.tsx components/settings/agent-profile-reconciliation.test.ts components/settings/agent-profile-dirty.test.ts lib/api/domains/agent-profile-normalize.test.ts components/task/terminal-websocket-lifecycle.test.tsx components/task/use-passthrough-terminal.test.ts components/task/agent-mcp-prepare-actions.test.tsx components/session/prepare-progress-status.test.ts lib/state/slices/session-runtime/prepare-result.test.ts lib/ws/handlers/executor-prepare.test.ts lib/api/domains/session-mcp-actions.test.ts
```
