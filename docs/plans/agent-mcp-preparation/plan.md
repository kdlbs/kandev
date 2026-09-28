---
created: 2026-09-28
status: complete
requirements:
  - REQ-AGENTS-MCP-PREP-001
  - REQ-AGENTS-MCP-PREP-002
  - REQ-AGENTS-MCP-PREP-003
system_design:
  - ../../specs/agents/system-design/agent-mcp-preparation.md
legacy_specs: []
---

# Implementation Plan: Agent MCP preparation

## Overview

Deliver profile-controlled MCP selection and automatic native preparation with
visible task progress and native authentication recovery. Establish persisted
contracts first, then implement disjoint credential, runtime and UI boundaries.
The user explicitly authorized this package and immediate worker implementation
in one turn. All work remains uncommitted for review.

## Scope

### In scope

Profile selection/discovery; Cursor native approval/readiness; durable task
preparation; authentication terminal and same-session retry; credential refresh
continuity; desktop/phone coverage and isolated real Cursor fixture proof.

### Out of scope

Other adapters, remote credential transfer, plugin installation, account merging,
blanket tool approval and provider business operations during verification.

## Technical approach

Task 01 expands AgentProfile storage, DTO/catalog, runtime resolver and generated
web contract with `mcp_selection_mode` and `mcp_selected_servers`. Task 02 adds
provenance to `mcpconfig/cursor_auth_bridge.go`. Task 03 owns discovery handlers,
Cursor command adapter and lifecycle preparation/recovery integration. Task 04
consumes these contracts in profile drafts/settings and task preparation/terminal
UI. Task 05 proves native fresh-task behavior with a local authenticated fixture.
Root owns integration review and documentation synchronization throughout.

| Provider / transport | Identity | Behavior | Evidence / fallback |
| --- | --- | --- | --- |
| Cursor ACP, local/worktree | Exact emitted MCP name | Selected imports approved and verified before conversation | Fake runner + native fixture; classified degraded readiness |
| Cursor terminal, local/worktree | Same | Same preparation with tool permissions retained | Native terminal fixture |
| Cursor remote/container/different HOME | Same | No host discovery or credential import | Gating regression tests |
| Other agents | Unsupported capability | Existing behavior retained | No discovery/UI controls |

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

## Tests

- AC-001.1/001.4: profile store/controller/contract and resolver tests, defaults,
  PATCH omission vs empty, duplicate and migration rebuild.
- AC-001.2/001.3/002.2/002.5: sanitized discovery and selection tests in
  `cursor_plugin_mcp_inventory_test.go`, native approval ownership/disables,
  cancellation and source-context tests.
- AC-002.1/002.3/002.4: injected native runner tests and
  `manager_launch_prepare_events_test.go`, completion ordering and persistence.
- AC-003.1-003.3: typed recovery endpoint validation, terminal deduplication,
  safe argv/quoting and same-session retry tests.
- AC-003.4: `cursor_auth_selection_test.go` and provenance tests for refresh,
  source change/removal and corrupt sidecar.

## E2E tests

`apps/web/e2e/tests/settings/cursor-plugin-mcp.spec.ts` and mobile counterpart:
selection persistence, refresh failures, unavailable saved IDs (AC-001.1/2/5).
New `tests/session/agent-mcp-preparation.spec.ts` and
`mobile-agent-mcp-preparation.spec.ts`: persisted degraded preparation,
authenticate terminal and retry same session (AC-002.3, 003.1-3).
Run chromium and mobile-chrome sequentially with managed fixtures/one worker.
Task 05 adds native ACP/terminal fixture proof (AC-002.1/2/4).

## Work orders

- [x] [Task 01: Persist profile selection](task-01-profile-contracts.md)
- [x] [Task 02: Preserve native credential refresh](task-02-credential-continuity.md)
- [x] [Task 03: Prepare and recover native MCP connections](task-03-runtime-preparation.md)
- [x] [Task 04: Profile and task preparation interface](task-04-preparation-ui.md)
- [x] [Task 05: Prove native task preparation](task-05-native-fixture.md)

## Verification results

Implementation and coordinator review are complete. The final targeted
promotion/passthrough ordering tests and lifecycle lint also pass. See [review findings](review-findings.md)
for resolved findings, exact failed broader-test names and validation limits.

The 95-test frontend suite, typecheck, localization/ratchet, scoped lint, backend
MCP/settings/terminal/gateway tests, focused lifecycle/recovery races, SQLguard,
store-conformance race tests and Windows MCP-config cross-compilation pass.
Both installed Cursor native smoke tests pass (73.772s total), proving fresh
ACP/terminal fixture tool access and saved-conversation recovery in a replacement
ACP child.

Browser E2E is not passed: desktop Chromium cannot launch under this macOS
sandbox (Mach port permission denial), and mobile was stopped before launch
because it uses the same binary. Broader backend suites also contain reported
path/cleanup/Git/socket fixture failures; no clean-baseline run establishes that
these failures predate the branch. These are explicit validation limits.

Design/document checks on 2026-09-28:

- `python3 scripts/list-docs.py validate`: passed (322 decisions, 1224 specs).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- Local `.github/scripts/pr-docs.cjs` `validateCoverage` against changed files
  and referenced package/agent documents: covered, no errors.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed.
- `node scripts/validate-public-docs.mjs`: 47 published pages passed.
- `git diff --check -- docs`: passed.

Documentation catalog/spec/public-page validation and local coverage validation
were repeated after the implementation updates and passed. These checks do not
substitute for native/runtime/browser acceptance evidence.

## Risks

Cursor CLI output/approval behavior is version-sensitive. CLI execution must be
bounded and stale work must not approve replacement definitions. Native refresh
writes may replace symlinks. Recovery must use the task environment and avoid
replaying prompts. Existing dirty changes belong to earlier work and are retained. Native login-terminal
recovery is initially POSIX-only; Windows returns unavailable instead of emitting
an unverified shell command. Native Windows execution remains untested here.
Automatic live TUI recovery remains unavailable until Kandev can retain an owned
Cursor CLI chat ID. Initial TUI preparation and POSIX authentication terminals
are supported; same-conversation automatic reload is ACP-only.
