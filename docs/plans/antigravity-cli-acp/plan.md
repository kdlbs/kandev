---
created: 2026-11-23
status: complete
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-004
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
  - REQ-AGENTS-ANTIGRAVITY-ACP-011
  - REQ-AGENTS-ANTIGRAVITY-ACP-013
  - REQ-AGENTS-ANTIGRAVITY-ACP-015
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
legacy_specs: []
---

# Implementation plan for Antigravity CLI ACP

## Overview

Use the verified `agy acp` command for the built-in profile on a host with a compatible Antigravity CLI. Route model discovery, utility inference, and task startup through the same selected runtime. Then make install and sign-in controls show which executable they set up.

## Evidence

Kandev's November 3 install action at `f5c03760265` provisions the pinned standalone ACP server and harness. Google added the CLI ACP command in 1.5.0 on November 4. [Vendor release notes](https://antigravity.google/docs/changelog?tab=cli#v1.5.0)

On November 23, the running Kandev build `4ebc1e2fdf4` reported `auth_required` from a refresh of `GET /api/v1/agent-models/antigravity-acp?refresh=true`. The response contained zero models. The installed CLI 1.5.15 returned 27 model and effort variants and completed an ACP smoke prompt. A separate standalone 1.4.5 process exited with `SIGILL`.

The smallest regression has both runtimes on the host, with a signed-in CLI and a standalone server that requests consent. The current profile returns the standalone auth result. The first backend regression must reproduce that choice. [Requirements](../../specs/agents/requirements/antigravity-acp-agent.md)

## Scope

- New local and worktree sessions and their host-local model and inference probes. The existing agent identity and profiles are the entry point.
- Resume of native conversations through their recorded runtime, including pre-upgrade standalone conversations.
- Runtime-aware install and sign-in guidance in Settings on desktop and phone.
- Installer scope covers the pinned standalone bundle. Google owns CLI installation, updates, and credentials. Remote and container launch use the executor's standalone runtime.

## Technical approach

1. Resolve a qualified CLI or a complete standalone bundle in `internal/agent/antigravityruntime`. Use the complete argv before profile flags and wrappers in `manager_launch.go`.
2. Use that selection for the command preview, host utility probes, inference, and task configuration. Carry the trusted variant to agentctl and bind it to the native conversation at creation.
3. Apply the runtime's wire identity and auth policy in both agentctl ACP paths. The vendor CLI provides its model and configuration options through ACP.
4. In Settings, use the backend's runtime selection to pick the login action. The CLI action reuses the agent login terminal. The standalone action reuses the consent dialog, including when a saved standalone session needs sign-in.

| Selected runtime | Command | Authentication | Verification |
| --- | --- | --- | --- |
| Compatible host CLI | `agy acp` | Cached CLI login. Explicit attempt runs `agy acp --login`. | CLI fixture and signed-in host smoke |
| Host without a compatible CLI | Validated standalone pair with platform args | Standalone `oauth-personal` and consent dialog | Existing installer and auth tests plus fallback fixture |
| Saved Antigravity conversation | Recorded variant. Older records select standalone. | Its original store | Resume fixture for each variant and missing-runtime case |
| Remote or container executor | Executor-local standalone argv | Executor's existing auth | Launch tests with an eligible CLI on the backend host |

See the [system design](../../specs/agents/system-design/antigravity-acp-agent.md) and [runtime decision](../../decisions/2026-11-23-antigravity-cli-acp-runtime.md).

## ASCII UI preview

### UI-01 Antigravity profile needs sign-in

Entry point is Settings > Agents > Antigravity. The agent list offers the same action.

```text
Current desktop
[Start model  Select a model...                     v]
  No auth - login required  [Open terminal] [Refresh]

Proposed desktop
[Start model  Select a model...                     v]
  Antigravity CLI
  Google sign-in required    [Sign in]       [Refresh]

Proposed phone
  Start model
  [Select a model...                       v]
  Antigravity CLI
  Google sign-in required
  [               Sign in                   ]
  [               Refresh                   ]
```

The profile owns vertical scrolling. The phone stacks visible actions with at least 44px hit targets. The active login surface fits the viewport and owns terminal scrolling. Reuse the existing agent terminal and the Antigravity consent dialog. The inset action pattern follows `mobile-menu-sheet.tsx`. The standalone state names its server and opens its consent dialog. The ready state enables the existing model picker. Copy in the preview is illustrative.

### UI-02 Antigravity list without a compatible runtime

```text
  Antigravity
  Install the Antigravity CLI
  [CLI setup guide]
  [Install standalone ACP server]
```

The phone keeps the same action order in a single column. The list shows the standalone install only on supported hosts. Views cover AC-AGENTS-ANTIGRAVITY-ACP-009.1, 010.7, and 015.4 under the same requirement prefix.

## Tests

| Criteria | Regression evidence |
| --- | --- |
| AC-AGENTS-ANTIGRAVITY-ACP-003.5, 003.14, 003.15 and 011.1 | `TestAntigravityLocalProbePrefersCLIWithStandaloneInstalled` and command, fallback, status, and wire fixtures from Task 01 |
| AC-AGENTS-ANTIGRAVITY-ACP-004.11 | `TestAntigravityResumeUsesSavedRuntime` covers old and new records, a worker restart, and a missing runtime |
| AC-AGENTS-ANTIGRAVITY-ACP-010.7 and 013.2 | `TestAntigravityCLIProbeUsesCachedAuth` and `TestAntigravityCLIPromptUsesWireOutcome` plus approved host smoke |
| AC-AGENTS-ANTIGRAVITY-ACP-009.1 and 015.4 | `TestAntigravityCLILoginUsesResolvedCommand`, web component tests, and the E2E cases below |

## E2E tests

- `chromium` runs `tests/settings/antigravity-runtime-setup.spec.ts`. Start with CLI ACP ready and select a model. Switch the fixture to signed out and verify that sign-in refreshes models. Cover the labeled standalone install and standalone recovery when the CLI is the host default.
- `mobile-chrome` runs `tests/settings/mobile-antigravity-runtime-setup.spec.ts`. Open the profile and tap sign-in. Verify the selected login surface, focus return, model refresh, touch hit areas, and viewport containment.

Use scoped fake ACP and login fixtures. The host smoke uses an existing local sign-in, and browser CI uses synthetic auth.

## Work orders

- [x] [Task 01 Use the CLI for local ACP](task-01-local-runtime.md)
- [x] [Task 02 Route Antigravity setup actions](task-02-setup.md)

## Verification results

The remediation suite passes across the backend and web paths. Focused Antigravity Go tests, lifecycle resume regressions, and changed-package Go lint pass; `make -C apps/backend build` succeeds. The web suite passes 29 tests, typecheck, lint, i18n checks, and production build. Desktop and mobile setup E2E each pass 2 tests. Public docs validation passes 63 checks across 56 pages; all 2,106 specification files and 464 decisions pass catalog validation. The signed-in host CLI probe returned 27 models and the ACP prompt smoke returned the requested `ACP_OK` response. `git diff --check` and the changed-Go-file gofmt check pass.

## Review remediation

The implementation review found gaps in utility command admission, CLI-only process setup and capability negotiation, saved-runtime resume propagation, task authentication, settings snapshot provenance, standalone task recovery, and command preview. Both work orders now include regressions for those paths, and the focused tests, builds, lint, documentation validation, and desktop and mobile flows pass.

## Risks

- CLI and standalone session IDs refer to different stores. A restarted session needs its committed runtime association before launch.
- A signed-out CLI or a CLI startup failure can look like an unavailable runtime. Surface the selected runtime's result.
- The native CLI prompt wire contract needs separate failure and resume qualification. The standalone journal is specific to its release.
- An externally updated CLI can change model IDs or support. Preserve the profile's exact selection and show unavailable models through the existing error path.
