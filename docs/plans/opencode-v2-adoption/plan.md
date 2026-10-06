---
created: 2026-09-27
status: completed
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
  - REQ-AGENTS-OPENCODE-V2-002
system_design:
  - ../../specs/agents/system-design/opencode-v2-adoption.md
legacy_specs: []
---

# Implementation Plan: OpenCode V2 Adoption

## Overview

Deliver v2 for fresh installations and an explicit, persisted migration for existing v1 users.
Keep one OpenCode agent identity, ACP integration, and existing conversations.
Implement runtime resolution first, the migration interaction second, and application session continuity third.
Ship the package together; completing an early work order does not establish release readiness.

Inputs are the [requirements](../../specs/agents/requirements/opencode-v2-adoption.md),
[design](../../specs/agents/system-design/opencode-v2-adoption.md), and
[adoption decision](../../decisions/2026-09-27-opencode-runtime-adoption.md).
Confirmed choices are managed v2, unchanged standalone CLI, install-wide selection, and success-only DB activation.
No material product question remains. Upstream compatibility still requires the prescribed executable evidence.

## Scope

### In scope

- Preserve legacy v1 selection before startup reconciliation; default genuinely fresh installs to v2.
- Resolve package, exact version, source, and command consistently across Kandev OpenCode consumers.
- Extend the existing update dialog and API with explicit migration, permission checks, progress, and failure recovery.
- Preserve saved-session identity and fail restoration without creating another conversation.
- Desktop/phone tests, targeted real ACP compatibility tests, and public documentation.

### Out of scope

- Native HTTP integration, a second OpenCode agent type, or per-profile runtime selection.
- Global CLI replacement, plugin conversion, automatic cross-family downgrade, or data rollback tooling.
- Changing other agents' runtime policies, introducing a generic distribution marketplace, or adding a feature flag.
- Persistent Kandev task creation, subagent delegation, and committing this design package.

## Technical approach

### Runtime state and commands

Add the single authoritative OpenCode record described in the design to the existing settings store.
Import old selections/default markers before `backendapp.reconcileManagedRuntimeDefaults` processes consumers.
Remove OpenCode from generic reset after its family-aware bootstrap is wired.
No new database table is required. The atomic record is the activation boundary and startup recovery source.

Extend resolution at `agents.OpenCodeACP`, `managedruntime`, registry, host utility, and lifecycle boundaries.
Keep both trusted packages in the pin catalogue. Initial test targets are 1.18.32 and 2.0.18.
Add family-major checks to pin maintenance and catalogue selection so `latest` cannot silently introduce v3.
Managed v2 wins over native preference; native source observes the installed major for compatible arguments.
Interactive OpenCode launches use its selected distribution without the ACP subcommand.

### Migration transaction and Settings

Extend DTOs/HTTP/client types with optional family and revision inputs and structural migration state.
Existing same-family requests remain valid. Use the current update job and maintenance coordinator.
Add launch admission coordination where needed to prevent a new v1 execution between liveness check and activation.
Stage managed v2, validate with disposable HOME/XDG/workspace, save atomically, then invalidate/refresh real capabilities.
Keep the previous catalogue until commit; never publish a clean probe's empty models as the user's model list.

Extend `AgentRuntimeUpdateSurface` and shared control state. The update action remains attached to the agent;
there is no new profile setting or schema. Existing admin permissions apply on both UI and server.

### Session continuity and configuration

Retain native session ID, working directory, and executor home when launching v2.
Make OpenCode restore failure bypass generic new-session fallback in lifecycle `session.go`.
Use ACP capability negotiation for resume/load and configuration changes.
Test actual Kandev MCP injection, permissions, models, and saved-session mapping, not only raw initialization.
Preserve supported v1 files and disclose plugin/unsupported-transport limits.

### Compatibility matrix

| Provider / source | Transport and identity | Intended behavior | Evidence required | Unsupported case |
| --- | --- | --- | --- | --- |
| Managed OpenCode v1 | ACP, same agent/session IDs | Retained before opt-in; exact v1 commands | Store, command, adapter tests | Invalid version fails visibly |
| Native OpenCode v1/v2 | ACP, observed supported major | Existing PATH source retained; manual external upgrade dispatches correct arguments | Native version fixtures plus isolated real resume | Unknown major fails; no guessed flags |
| Managed OpenCode v2 | ACP, same agent/session IDs | Fresh default or explicit migration, no PATH override | Commands, API/UI restart, real continuation | No alternate package fallback |
| OpenCode interactive CLI | PTY, existing resume flags | Selected package, no `acp` argument | Passthrough command tests | Custom commands remain user overrides |
| SSH/container OpenCode | ACP, executor-local home | Same managed selection installed in executor | Preflight/install contracts and available executor integration fixtures | Surface remote failure; host probe is not remote certification |
| Other managed providers | Existing ACP/PTY split | No selection or native-preference changes | Existing shared runtime suites | Preserve existing provider behavior |

## ASCII UI preview

### Dialog simplification (2026-09-28)

The user requested less update explanation in the dialog. This revision supersedes the expanded explanatory text and command block in UI-01/UI-02 below.
Reuse `SettingsInfo`: hover/focus reveals details on desktop, and tapping opens its accessible information sheet on touch devices.
Keep the version change, runtime choice, migration warning, and primary action visible. Hide the redundant header description visually but retain its accessible description.
The command is an initially collapsed native details disclosure. Existing translations are reused.

```text
Desktop dialog                  Phone update drawer
Update OpenCode                 Update OpenCode
Upgrade to v2 (i)               Upgrade to v2       (i)
1.18.32 -> 2.0.18                1.18.32 -> 2.0.18
Version summary                 Version summary
[Update v1] [Upgrade to v2]      [Update v1] [Upgrade to v2]
Stop external v1 processes      Stop external v1 processes
> Command that will run         > Command that will run
[Cancel] [Upgrade to v2]         [Upgrade to v2] [Cancel]
```

The information disclosure holds shared profile scope, unchanged standalone CLI, model refresh behavior, and future-launch semantics.
The desktop test checks hover/keyboard disclosure; the phone test checks tap, a 44px target, closing the information sheet back to the update drawer, and expanding the command.
No update request is sent by either disclosure. Runtime behavior and activation semantics are unchanged.



UI-01: Desktop, Settings > Agents > OpenCode update control, explicit migration selected.

```text
+----------------------------------------------------+
| Update OpenCode                                [X] |
| Current: v1 1.18.32     Target: v2 2.0.18            |
| [Update v1] [Upgrade to v2]                         |
|                                                    |
| Uses managed OpenCode v2 for all OpenCode profiles. |
| Your standalone CLI stays unchanged.               |
| Stop external v1 processes that share session data. |
|                                                    |
| [Details: selected command and compatibility]       |
|----------------------------------------------------|
| [Cancel]                           [Upgrade to v2] |
+----------------------------------------------------+
```

UI-02: Phone, same entry/action, temporary inset bottom drawer.

```text
          Settings > Agents
+----------------------------------+
| OpenCode                  [Update]|
+----------------------------------+

  +------------------------------+
  | Update OpenCode          [X] | fixed header
  | Current: v1 1.18.32          |
  | Target: v2 2.0.18            |
  | [Update v1] [Upgrade to v2]  |
  | All OpenCode profiles       | one scrolling body
  | Standalone CLI unchanged    |
  | Stop external v1 processes  |
  | [Details]                   |
  |------------------------------|
  | [       Upgrade to v2      ] | fixed footer
  | [          Cancel         ] | safe-area clearance
  +------------------------------+
```

UI-03: Shared state region inside UI-01/UI-02. The action is never implicitly submitted.

```text
Resolving:  Checking available version...      [Upgrade disabled]
Blocked:    OpenCode sessions are active.      [Check again]
Running:    Installing / Checking ACP / Saving [Action disabled]
Failed:     Upgrade failed. Still using v1.    [Retry upgrade]
Succeeded:  OpenCode v2 is selected.            [Done]
Discovery:  V2 selected. Models unavailable.   [Retry discovery]
```

Structural requirements: visible shared scope, distinct opt-in, one body scroll owner, and reachable fixed actions.
Spacing and copy are illustrative and must use existing primitives and translations.
Phone max-height is 92dvh; actual touch targets are at least 44px. Desktop ordinary controls retain 28px sizing.
Dismissal returns focus; reopening reads persisted state. Long command/log detail cannot widen the page.
These views cover AC-AGENTS-OPENCODE-V2-001.3, .5, .6, .8, .9, and .10.

## Tests

Paths are relative to the repository root; the entries below name implementation and compatibility evidence.

| Acceptance criteria | File and proposed test / evidence |
| --- | --- |
| 001.1, .2, .7, .9 | `apps/backend/internal/agent/managedruntime/opencode_selection_test.go`: `TestBootstrapOpenCodeFreshInstallSelectsManagedV2`, legacy import/marker cases, and family-default reconciliation |
| 001.1, .2, .7 | `apps/backend/internal/backendapp/managed_runtime_defaults_test.go`: `TestOpenCodeFilesystemEvidenceUsesOnlyConfigurationAndSessionDatabase` and startup ordering/failure cases |
| 001.4, .11 | `apps/backend/internal/agent/agents/opencode_acp_test.go`: native-major detection, selected-family commands, managed source priority, and unsupported-major handling |
| 001.4, .7, .9 | `apps/backend/internal/agent/runtime/lifecycle/managed_runtime_command_test.go` and `internal/agent/hostutility/managed_runtime_test.go`: `TestOpenCodeResolvedRuntime`, table-driven host/remote/cache/utility commands |
| 001.3, .5, .6, .9, .10 | `apps/backend/internal/agent/settings/controller/opencode_migration_test.go`: `TestOpenCodeMigrationActivation`, `TestOpenCodeMigrationFailureBoundaries`, `TestOpenCodeMigrationLaunchRace`; exercise both orderings of launch and maintenance admission |
| 001.3, .9, .10 | `apps/backend/internal/agent/settings/controller/opencode_migration_test.go` and existing HTTP handler tests: trusted family/version, permissions, revision rejection, duplicate jobs, default action |
| 001.3, .8, .10 | `apps/web/lib/api/domains/agent-update-api.test.ts`, `components/settings/agent-runtime-update-control.test.tsx`, `lib/agent-runtime-update.test.ts`, and settings hook tests: explicit migration requests, shared states, and ordinary update regression |
| 001.9 | `apps/backend/internal/agent/managedruntime/opencode_selection_persistence_test.go`: `TestOpenCodeSelectionPersistsAcrossStoreReopen`; activated family, package, exact version, and revision survive a SQLite reopen |
| 002.2, .3 | `apps/backend/internal/agent/runtime/lifecycle/session_load_failure_test.go`: `TestInitializeSession_LoadFailureDoesNotCreateReplacement`; saved OpenCode ID is retained and load failure does not send `agent.session.new` |
| 002.1, .2, .4; 001.4 | `apps/backend/internal/agentctl/server/adapter/e2e/opencode_v2_migration_test.go`: `TestOpenCodeACP_V1ToV2Resume`, `TestOpenCodeACP_V2Compatibility`; real binaries, MCP, models, permissions, shutdown |
| 001.1, .2, .7 | `scripts/update-agent-runtime-pins.test.mjs`: both packages remain pinned, reject a mismatched major, other packages unchanged |

Acceptance suffixes above belong to `AC-AGENTS-OPENCODE-V2-`.
Use deterministic fake ACP fixtures for application lifecycle failure paths and isolated real binaries for protocol evidence.
No user credentials, home, config, or sessions may be used as test fixtures.

## Browser and compatibility tests

| File / project | Required scenario and acceptance criteria |
| --- | --- |
| `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`, chromium | Existing v1 requires an explicit migration choice; failed probe leaves the reopened preview on v1; retry submits the same v2 family, version, and revision. Existing update success/failure regressions remain covered. 001.2, .3, .5, .10 |
| `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`, mobile-chrome | Migration is an explicit touch-safe drawer action; shared scope and external-process guidance stay visible, the footer remains reachable, and the page has no horizontal overflow. 001.3, .8, .10 |
| Backend startup, migration, persistence, and lifecycle tests | Fresh/legacy selection, blocked admission and failure boundaries, SQLite reopen durability, and no replacement after OpenCode restore failure. 001.1, .2, .5-.7, .9; 002.2, .3 |
| `apps/backend/internal/agentctl/server/adapter/e2e/opencode_v2_migration_test.go`, explicit real-test opt-in | Exact managed v1-to-v2 binaries use an isolated home/database and local provider; v2 resumes the v1 native ID and history, with v2 model/config checks and process shutdown. 001.4; 002.1, .4 |

Browser tests can use deterministic runtime fixtures. The separate real-binary test is mandatory for claims about upstream history compatibility.
The E2E runner rebuilds frontend/backend; never reuse stale binaries with `--no-build` after code changes.

## Work orders

- [x] [Task 01: Persist runtime family and resolve commands](task-01-runtime-selection.md)
- [x] [Task 02: Activate migration through the update dialog](task-02-migration-dialog.md)
- [x] [Task 03: Preserve sessions and document compatibility](task-03-session-continuity.md)

Dependency order: 01 -> 02 -> 03, sequential in the primary session.
Each work order uses TDD and contains its exact commands. No generic QA or review work order is added.

## Existing package reconciliation

The completed `managed-runtime-default-activation` package records the original generation rule.
The `managed-runtime-recovery` package records exact candidate activation and responsive version selection.
Their historical results remain unchanged. This package owns the OpenCode exception and updates the living
requirements/designs and existing shared tests rather than rewriting completed work-order results.
Public docs describe managed v2 for fresh OpenCode installs, retained v1 selections, the explicit migration
action, and the unchanged standalone CLI.

## Verification results

Implementation verification on 2026-09-28:

- OpenCode-focused Go tests passed across the touched runtime, settings controller, lifecycle, registry, host utility, and backend startup packages. The complete lifecycle package passed with a short task-owned temporary directory.
- SQLite selection-reopen test passed; controller migration boundary tests passed.
- The explicitly enabled real ACP suite passed both v1-to-v2 history resumption and v2 compatibility tests with versions 1.18.32 and 2.0.18.
- Desktop runtime-update E2E passed 17/17; mobile runtime-update E2E passed 6/6. The E2E build completed both the backend and Vite production assets.
- Focused frontend tests passed 59/59; web typecheck passed. Targeted ESLint completed with zero errors and 11 warnings.
- Runtime pin tests passed 9/9. The i18n checker, ratchet, and Traditional Chinese generator passed.
- Public documentation validation passed for 47 pages; its node tests passed 62/62. Specification validation passed for 317 decisions and 1212 specifications; spec lint passed.
- `git diff --check` passed.
- A combined race run passed the host utility, managed-runtime, and Settings controller packages, then reported three unrelated Devin, Goose, and Muse installer test failures. Their subprocesses failed with `cat: write error: No space left on device` on the shared `/tmp` filesystem. The OpenCode-focused agent tests pass when filtered by name.

Review follow-up verification on 2026-09-28:

- `go build ./...` passed from `apps/backend` with task-owned `TMPDIR` and `GOTMPDIR`.
- Race tests passed for `internal/agent/hostutility`, `internal/agent/managedruntime`, `internal/agent/settings/controller`, `internal/backendapp` OpenCode/default-startup tests, `internal/agentctl/server/utility` probe tests, and OpenCode tests in `internal/agent/agents`.
- The earlier full `internal/agent/agents` race run failed three unrelated installer tests when subprocess writes hit shared `/tmp` (`cat: write error: No space left on device`). The full package suite was not rerun afterward; no package-wide pass is claimed.
- The targeted host-utility and mobile browser tests passed. Docker/SSH managed-runtime recovery was not verified locally: the container run encountered `ENOSPC` in the shared `/tmp` and an `runc` temp-file failure. All six container shards passed in the PR CI run after the review fixes.
- The later PR snapshot at `82cf819d078`, attempt 2 of run `36400881920`, had 57 checks passed, 10 skipped, one failed E2E shard, and two dependent E2E aggregates pending. The mobile clarification send target was covered by the update-available toast; the same shard reported one stale deletion preview as flaky after its retry passed.
- After those failures, the mobile clarification test passed locally with retries disabled after waiting for the transient toast. The E2E delete helper now refreshes only on that exact stale-preview 409; its regression passed, and the focused deletion E2E passed with retries disabled. Four isolated repetitions before the helper change passed on the exact previous head, so the CI timing race did not reproduce locally. The later merge with current `main` replaced the temporary wait with main's update-toast placement fix and explicit non-overlap assertion; the merged mobile spec passed with retries disabled.
- The dialog simplification passed the full desktop runtime-update spec (17/17) and mobile runtime-update spec (6/6) with retries disabled. Focused Settings/API-client tests passed 13/13; web typecheck, targeted ESLint, and the production E2E build passed.
- The managed npm install prefix fix passed `go test -race ./internal/agent/settings/controller`; the OpenCode fresh managed v2, managed v1, and native v1 install cases verify the prepared prefix and selected cache behavior. The live Sprites environment was not available for post-fix verification.
- `python3 scripts/list-docs.py validate` validated 321 decisions and 1220 specifications; `python3 scripts/lint-spec-files.py --all` passed. `git diff --check` passed.
- At the time of the initial fixup record, CI for remote head `e56f4dd3126` was still pending. The `82cf819d078` counts above are historical and do not verify that head or the later merge commit.
- The verification-record update passed `node scripts/validate-public-docs.mjs` (47 pages) and `TMPDIR=/root/.cache/kandev-go-tmp node --test scripts/validate-public-docs.test.mjs` (62/62).
- `git diff --check` passed.

Initial design-package validation on 2026-09-27:

- `python3 scripts/list-docs.py validate`: passed; 317 decisions and 1212 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed; 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs` local `validateCoverage`: actual documentation-only diff is exempt;
  prospective source-change evaluation is covered with all three work-order chains valid.
- Local Markdown target check: passed for the new requirement, design, decision, and plan package.
- `git diff --check -- docs/plans/opencode-v2-adoption docs/specs docs/decisions`: passed.
- `git status --short -- docs/plans/opencode-v2-adoption`: confirmed the new untracked package.

The implementation and review fixes are tracked in PR #4014; public README guidance and these verification limits are recorded with the work orders.

## Risks

- Existing profile plugins and unsupported MCP transports can prevent real discovery after activation.
- A clean candidate probe cannot prove authenticated model access or all remote platform artifacts.
- Owned-process activation requires admission coordination, not a racy liveness snapshot.
- External v1 processes share storage outside Kandev's process guard; the UI must state the boundary.
- Conservative legacy detection preserves v1 on older installations that have never used it.
- Same-family default changes remain automatic under the existing reviewed-default policy.
