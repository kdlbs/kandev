---
created: 2026-11-25
status: implemented
requirements:
  - REQ-AGENTS-INJECTED-MCP-APPROVAL-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-001
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-012
system_design:
  - ../../specs/agents/system-design/injected-mcp-approval.md
  - ../../specs/agents/system-design/antigravity-acp-agent.md
legacy_specs: []
---

# Antigravity approval and logo fix

## Overview

Recognize injected Kandev Core tools in Antigravity CLI ACP sessions. Add the CLI permission switch to profile settings with an off default. Use the provider-usage plugin's Antigravity arch for both themes.

Agents owns all three outcomes through its existing tool approval, launch settings, and provider identity contracts.

## Evidence

Antigravity CLI 1.5.15 ran the Neon High profile in Auto with Kandev auto-approval off. The following saved prompts offered Allow once and Deny.

| Time (UTC) | ACP tool notification name | Saved approval |
| --- | --- | --- |
| 19:14:35 | `mcp_kandev_set_task_title_kandev` | Approved through the web client |
| 19:16:08 | `mcp_kandev_list_agents_kandev` | Approved through the web client |

The records come from Kandev session `ff8f827c-bb80-49b3-9541-caeb215e5f82` and ACP session `f0054ad4-139b-4c41-900a-b43a416c5907`. Match each request by tool-call ID. The screenshot corresponds to a shell permission.

`ParseQualifiedMCPToolName` accepts the double-underscore form. Antigravity has no permission-name translation for the observed form. The retained log records tool notifications, but no `session/request_permission` frame. The `agy` executable is not installed here, and the disposable CLI handshake required Google sign-in before opening a session. Work order 01 records this limitation and tests the typed ACP request contract and notification correlation through the peer fixture.

Installed `agy acp --help` lists `--dangerously-skip-permissions`. The profile catalog lacks a curated entry for it. The live logo endpoint returned the embedded star for both variants. The reference SVG uses the rounded arch. [Google ACP flags](https://antigravity.google/docs/ide/extensions/acp#flags), [provider-usage icon source](https://github.com/kdlbs/kandev-plugin-provider-usage/blob/671dd5b87a95095ba8c99da93941449b7ef62c60/ui/bundle.js#L200)

## Scope

- Fix ordinary approvals for identifiable tools on the injected Kandev Core server.
- Add an opt-in profile flag for the Antigravity CLI.
- Replace both embedded SVGs. Check the existing picker on desktop and phone.

The existing session policies govern shell tools, ambiguous MCP identities, questions, and scoped execution. This delivery uses the existing settings storage, MCP endpoints, and tool authorization.

## Technical approach

1. Qualify Antigravity's programmatic permission identity. Resolve it against the current injected Core endpoint and its exported tools. Reuse the existing option selection and server authorization.
2. Add a disabled catalog entry with the existing `cli_flag` apply method. Store choices in `cli_flags` and use the effective CLI command in preview and launch.
3. Embed the reference arch with the existing light and dark fills. Exercise the logo API and composer picker.

| Session or request | Result | Verification |
| --- | --- | --- |
| Antigravity CLI ACP with an identifiable Core call | Offered allow-once, then offered allow-always | ACP request and response fixture |
| Shell, third-party, or ambiguous tool identity | Ordinary permission policy | Mixed-server ACP fixture |
| Native question, coordinator, or managed session | Applicable session policy | Existing policy tests |
| Existing qualified MCP identity | Current injected-server path | Existing ACP flow test |
| Antigravity CLI flag off or unset | Ordinary launch argv | Profile catalog, preview, and launch checks |
| Supported CLI flag on | One flag on eligible new CLI launches | Saved settings and effective argv checks |
| Standalone or a launch that requires per-tool review | Ordinary launch argv | Incompatible launch fixture |

## ASCII UI previews

### UI-01 Profile CLI flag

Open Settings > Agents > Antigravity > profile. The existing CLI Flags card contains the switch. Save applies the profile choice to future eligible launches. AC-AGENTS-ANTIGRAVITY-ACP-001.8.

```text
Agent CLI flags
  Skip Antigravity CLI tool prompts             [Off]
  --dangerously-skip-permissions

                                            [Save]
```

The phone settings page uses the same control in its scrolling field list. Text can wrap. The switch uses a 44 px touch target. The row stays in the profile page.

### UI-02 Composer provider icon

Open an Antigravity session. The theme-specific arch appears before the model label on desktop and phone. AC-AGENTS-ANTIGRAVITY-ACP-001.5.

```text
Desktop composer       [Antigravity arch] Gemini 4 Neon [v]
Phone composer         [Antigravity arch] Gemini 4 Neon [v]
```

The bracketed label represents the SVG. The existing desktop popover and phone picker sheet remain the entry points for model selection.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| Injected MCP 001.3 to 001.5, 001.7, 001.9 | `TestAntigravityInjectedKandevPermissionACPFlow` and existing injected permission policy tests |
| Antigravity ACP 012.1, 012.3 | `TestAntigravityHumanInputNeverAutoApproves` and existing coordinator policy tests |
| Antigravity ACP 001.8, 003.13 | Existing catalog, command preview, and local launch fixtures with on, off, and fallback cases |
| Antigravity ACP 001.5 | Extend `TestAntigravityACP_LogosNonEmpty` to check valid theme variants and arch geometry |

## E2E tests

Use the real profile catalog and logo endpoint. Stub runtime availability where needed. The cases require no Google sign-in.

| Requirement | Desktop Chromium | Mobile Chromium |
| --- | --- | --- |
| Antigravity ACP 001.8 | `settings/agent-profile-cli-flags.spec.ts` | `settings/mobile-antigravity-runtime-setup.spec.ts` |
| Antigravity ACP 001.5 | `chat/model-selector-provider-icon.spec.ts` | `chat/mobile-model-selector.spec.ts` |

## Work orders

- [x] [Task 01 Recognize injected MCP calls](task-01-injected-mcp-approval.md)
- [x] [Task 02 Expose the CLI permission flag](task-02-cli-permission-flag.md)
- [x] [Task 03 Use the Antigravity arch](task-03-antigravity-logo.md)

Run the orders in this sequence. Work order 01 establishes the narrow approval path. Work order 02 separately verifies the opt-in launch path. Work order 03 verifies the visual identity.

## Verification results

Design checks passed.

- `python3 scripts/list-docs.py validate` validated the catalog.
- `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- The PR documentation validator classified the current diff as documentation-only. A simulated implementation diff resolved all three work orders.
- `git diff --check` passed.

Implementation and focused verification passed:

- Work order 01's ACP approval tests passed in the previous implementation turn. Its fixture covers the typed request and notification-correlation contract. A raw `session/request_permission` capture remains unavailable because `agy` is not installed and the isolated handshake required Google sign-in; this limitation is documented above and in task 01.
- Work order 02 backend permission, command preview, launch-policy, and mock identity tests passed. `pnpm run typecheck`, `pnpm run i18n:check`, public-doc validation, and desktop/phone settings E2E passed.
- Work order 03 asset and endpoint tests passed. Desktop Chromium loaded the dark arch from the logo API; mobile Chromium loaded the light arch in the model picker.
- Both frontend E2E runs built the backend. The desktop run also built the Vite web assets. Prettier, `gofmt -l`, and `git diff --check` passed.
- Review follow-up fixes cover external plugin discovery manifests, bounded regular-file reads, qualified ACP aliases, duplicate switch entries, saved-profile launch eligibility, and exact flag token parsing. The ACP peer fixture uses the production Core catalog and exact endpoint, while legacy Core profiles are checked against their registered tools.
- The full affected backend package suite passed with `-race -trimpath -tags fts5`; changed-Go lint reported 0 issues. The local build, web typecheck, i18n check, asset helper tests, and desktop and phone E2E passed. The duplicate-profile UI regression also passed, checking the saved disabled choice and preserved unrelated flag. Four fresh desktop/phone settings and logo captures were validated. The old-head CI failures also pass their targeted local regressions; new-head CI remains authoritative after push.

## Risks

- The native alias uses a delimiter that can occur in server names. Work order 01 must establish server identity before it approves an alias.
- The native flag suppresses CLI permission requests. Keep it off by default and apply it only to eligible CLI sessions.
- The command and visual checks use fixtures for CI. The ACP fixture must come from the qualified wire shape.
