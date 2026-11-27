---
created: 2026-09-25
status: implemented
requirements:
  - REQ-AGENTS-HOST-CLI-001
  - REQ-AGENTS-HOST-CLI-002
  - REQ-AGENTS-HOST-CLI-003
  - REQ-AGENTS-HOST-CLI-004
system_design:
  - ../../specs/agents/system-design/host-cli-model-discovery.md
legacy_specs: []
---

# Implementation Plan: Host CLI Model Discovery

## Overview

Claude and Codex profiles get a model list sourced from the installed vendor
CLI where the vendor documents one, a custom model ID entry where it does not,
and the installed CLI version on the Agents settings card. Discovery re-runs
when the backend loads agents, when the Agents settings page loads, and after
the existing agent install action replaces a CLI. The order is: trusted
host-CLI metadata and process adapters first, then discovery, the merged model
API, and the refresh triggers, then the frontend, then E2E and public docs.

Installing or upgrading a vendor CLI stays the existing agent install action
(`EnqueueInstall` / `POST /api/v1/agent-install/:agentName`, UI at
`/settings/agents/browse`). This package adds no update mechanism.

## Scope

### In scope

- `hostcli` package: spec, version parsing, Codex app-server model listing.
- `HostCLIAgent` on Claude, Codex, and the E2E mock agent.
- CLI version on discovery; merged model list with `discovery` metadata.
- Refresh triggers: agent load, settings page load, install completion,
  explicit profile refresh.
- Agents card version badge; profile model selector custom entry and source
  note; six-locale copy.
- Playwright coverage on desktop and phone; public docs.

### Out of scope

- Any new CLI install or update API, job, or button.
- Pointing the ACP bridge at the host CLI (see the ADR).
- Agent types other than Claude and Codex.
- Model ID validation against vendor APIs.

## Technical approach

### Backend: host CLI metadata and adapters (`internal/agent/hostcli`)

- `spec.go`: `Spec`, `ModelSource`, `Model`.
- `version.go`: `ParseVersion`, `DetectVersion` with a five-second timeout.
- `codex_models.go`: `ListCodexModels`; JSON-RPC framing, reader started
  before the writes, typed errors `ErrNotInstalled`, `ErrNotLoggedIn`,
  `ErrTimeout`.
- `runner.go`: `Runner`/`Process` seam and `ExecRunner`.
- `agents.HostCLIAgent` in `internal/agent/agents/agent.go`; `HostCLI()` on
  `ClaudeACP`, `CodexACP`, `MockAgent`; mock-agent gains `--version` and
  `app-server` entry points in `cmd/mock-agent/host_cli.go`.

### Backend: discovery, models, and refresh

- `discovery.Availability` gains `CLIVersion`/`CLIVersionError`;
  `discovery/host_cli.go` owns the resolver, its per-path cache, and
  `ApplyHostCLI`, used by the sweep and the E2E synthetic path.
- `dto.AgentDiscoveryDTO` gains the two fields; `dto.ModelDiscoveryDTO` and
  `discovery` are added to `DynamicModelsResponse` and `ModelConfigDTO`;
  entries carry `source`.
- `controller/host_cli_models.go`: cache, `WarmHostCLIModels` (stale only),
  `refreshHostCLIModels`, `hostCLIInstallSucceeded`, `hostCLIModelProjection`.
- Triggers: `backendapp/main.go` after `SetHostUtility`; the
  `/agents/discovery` handler; the install `JobStore` `onSuccess` callback;
  `FetchDynamicModels(refresh=true)`.

### Frontend

- Types: `AgentDiscovery` CLI fields, `ModelDiscovery`, `ModelEntry.source`,
  `discovery` on `ModelConfig` and `DynamicModelsResponse`.
- `lib/agent-host-cli.ts`: version label and discovery-note helpers.
- `components/settings/installed-agent-card.tsx`: version line.
- `components/settings/model-discovery-note.tsx`: source, version, failure.
- `ModelConfigSelector`/`ModelConfigSelectorContent`: `allowCustomModel` and
  the custom row; `ModelPicker` passes discovery and renders an absent stored
  model as a custom entry.
- Copy in `en`, `ja`, `pt-pt`, `zh-cn`; `zh-hk`/`zh-tw` via
  `pnpm run i18n:zh-hant`; pseudo via `pnpm run i18n:pseudo`.

## ASCII UI preview

Structural requirements: the version line sits under the detected path; the
custom row is the last row of the model list; the source note sits under the
selector. No new action is added to the agent card. Spacing and glyphs are
illustrative.

### UI-01: Agents settings card (desktop, detected host CLI)

```text
+----------------------------------------------------------------------------+
| [logo] Claude  [MCP] [Configured]                    (o) [v] [+ New profile]
| Detected at /usr/bin/claude                                                 |
| Claude Code 2.1.220                                                         |
+----------------------------------------------------------------------------+
   (o) existing managed runtime update trigger   [v] existing collapse control

Version unknown variant:
| Claude Code version unknown: version command timed out after 5s             |
```

Phone: the header wraps onto two rows and the version line wraps under the
path; the existing controls keep their 44 px hit area.

### UI-02: Profile model selector with discovery (Codex and Claude)

```text
Start model
+------------------------------------------+
| gpt-6-astra                           v  |
+------------------------------------------+
  Models from Codex CLI 0.155.1                          [refresh]

Open popover:
+------------------------------------------+
| [ filter or type a model ID ]            |
| Model                                    |
| ( ) GPT-6-Astra          Frontier ...    |
| ( ) GPT-6-Sol            Workhorse ...   |
| ( ) opus[1m]  (from the Claude runtime)  |
| (+) Use "claude-opus-5-5" as model ID    |   <- while typed text has no exact match
+------------------------------------------+

Claude note variant:
  Claude Code 2.1.220 does not publish a model list. Showing the Claude
  runtime's models. Type a model ID to use another model.
Failure variant:
  Model discovery failed: the command-line tool is not signed in.   [refresh]
```

Phone: the same popover, viewport-contained, 44 px rows (existing behavior).

## Tests

| Criterion | Evidence |
| --- | --- |
| AC-001.1, AC-001.2, AC-001.4 | `hostcli/hostcli_test.go` (`TestParseVersion`, `TestDetectVersion`), `discovery/host_cli_test.go` (`TestDetectCarriesHostCLIVersion`, `TestDetectReportsHostCLIVersionFailure`) |
| AC-001.3 | `discovery/host_cli_test.go` (`TestHostCLIVersionCacheAndInvalidation`) |
| AC-002.1 | `hostcli/codex_models_test.go` (`TestListCodexModels`), `controller/host_cli_models_test.go` (`TestFetchDynamicModelsMergesHostCLIModels`) |
| AC-002.2 | `controller/host_cli_models_test.go` (`TestFetchDynamicModelsSkipsAgentsWithoutModelSource`) |
| AC-002.3, AC-004.1, AC-004.2 | `apps/web/lib/agent-host-cli.test.ts`, `apps/web/components/model-config-selector.test.tsx` |
| AC-002.4 | `controller/host_cli_models_test.go` (`TestHostCLIModelsCacheAndRefresh`) |
| AC-002.5 | `hostcli/codex_models_test.go` (`TestListCodexModelsFailures`), `controller/host_cli_models_test.go` (`TestFetchDynamicModelsFallsBackWhenCLIFails`) |
| AC-002.6 | `controller/host_cli_models_test.go` (`TestFetchDynamicModelsLeavesOtherAgentsUnchanged`) |
| AC-003.1, AC-003.2 | `controller/host_cli_models_test.go` (`TestWarmHostCLIModelsOnlyVisitsModelSourceAgents`, freshness assertions) |
| AC-003.3 | `controller/host_cli_models_test.go` (`TestHostCLIInstallSucceededRediscoversModels`) |
| AC-003.4 | `controller/host_cli_models_test.go` (`TestHostCLIModelsCacheAndRefresh`) |
| AC-003.5 | No new route: `handlers` route table unchanged apart from reads |
| AC-004.3 | `controller/host_cli_models_test.go` (failure cases assert the bridge list survives) |

## E2E tests

| Flow | Criteria | File / project |
| --- | --- | --- |
| Card shows the mock CLI version; profile selector shows the discovery note, accepts a custom model ID, and persists it | AC-001.1, AC-002.1, AC-002.3, AC-004.1 | `e2e/tests/settings/agent-host-cli.spec.ts` / `chromium` |
| Phone variant of the same selector flow | AC-002.3 | `e2e/tests/settings/mobile-agent-host-cli.spec.ts` / `mobile-chrome` |

## Work orders

- [x] [Task 01: Host CLI metadata and process adapters](task-01-hostcli-package.md)
- [x] [Task 02: CLI version discovery and merged model API](task-02-discovery-and-models.md)
- [x] [Task 03: Discovery refresh triggers](task-03-refresh-triggers.md)
- [x] [Task 04: Settings and profile UI](task-04-frontend.md)
- [x] [Task 05: E2E coverage and public docs](task-05-e2e-and-docs.md)

## Verification results

The original contributor reported full backend, frontend and E2E success.
Those claims describe the original head and are historical, not verification
of the current integration. The original hostcli tests use deterministic
runners and protocol fixtures; they do not invoke installed Claude or Codex.

Scoped remediation preserves current main's profile authorization, launch
settings, runtime maintenance and mobile settings composition. It fixes
process reaping, initialization failures, pagination, bounded output,
vendor-versus-bridge executable identity, cache invalidation races, catalogue
projection and localized failures, including the Korean catalogue.

- Focused hostcli regression proof: first reproduced missing Wait and ignored
  initialization errors, then passed the hostcli package with deterministic
  protocol fixtures in a nonroot, credential-free sandbox with no network,
  a read-only root, no capabilities and bounded memory, CPU and PIDs.
- Go formatting and whitespace checks are static local checks.
- Prettier formatting ran in the same isolation boundary.
- Broad backend, frontend, i18n, desktop and mobile E2E suites and commit-hook
  equivalents are delegated to required hosted CI. They remain pending until
  successful exact-head results are collected in the external review receipt.
- No installed host CLI, user profile, app-server or agent session was invoked.

## Risks

- The Claude bridge coerces an unknown full model ID to the nearest alias or
  rejects it at session start; the custom entry cannot promise acceptance.
- `codex app-server` needs a logged-in CLI to list models; logged-out hosts
  fall back to the bridge list.
- Version detection adds one short subprocess per detected host CLI to each
  uncached discovery sweep.
