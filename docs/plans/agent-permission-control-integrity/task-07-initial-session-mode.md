---
id: "07-initial-session-mode"
title: "Deliver the permission mode at session start"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.7
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.8
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.9
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.10
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.11
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Task 07: Deliver the permission mode at session start

## Summary

Kandev never configures the agent process with a permission mode. It creates the
session in the runtime's default mode and switches afterwards. Give the mode an
initial-delivery channel resolved before the process starts, in a per-session
configuration directory Kandev owns.

This is the work order that addresses the reported symptom directly: a mode that
reaches the agent's instruction layer without changing what the process
enforces.

## Scope

- Add an `InitialModeDelivery` declaration to `agents.Agent` with the three
  shapes in the design (`settings`, `session_meta`, absent), plus the
  mode-availability precondition. Declare it only for agents whose wire contract
  has been read; Claude ACP uses `settings`.
- Extend the per-instance session directory (`CommandBuilder.ExpandSessionDir`,
  `RuntimeConfig.SessionConfig.SessionDirTemplate`) to host executors, and
  export its path through the agent-declared configuration environment variable.
  Container executors keep their existing bind mount.
- Before launch, write only the Kandev-owned keys (the resolved effective mode)
  into that directory's settings file, preserving any other content.
- For container executors, declare the runtime sandbox environment so a
  permissive mode is not disabled for a root process identity.
- When the requested mode remains unavailable for the executor, record it as
  unavailable with the reason on the session instead of starting silently in
  another mode.
- Agents with no declared channel keep the post-creation switch and record the
  mode as best-effort.

## Exclusions

- No change to mode precedence. The effective mode resolved by
  `Manager.effectiveSessionMode` is the value delivered here.
- No `_meta` on `session/new` for Claude ACP: the bundled bridge overrides any
  caller-supplied `permissionMode` in the session request, so that channel does
  not work for it.
- No writes into the workspace's own agent configuration. The runtime strips an
  escalating default mode from repository-committed sources, and Kandev must not
  modify a checked-out repository to change a permission.
- No confirmation or attribution work; that is work order 02.

## Acceptance

1. A session started with a profile mode launches an agent process already
   configured with that mode, verified at the agent's configuration boundary
   rather than by a post-creation switch.
2. Delivery writes only into the per-session directory; a host-executor launch
   leaves the user's shared agent configuration byte-identical.
3. A mode the runtime disables for the executor's process identity is reported
   as unavailable with its reason, and the session still starts.

### What does not count as acceptance

`AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.4` applies to this work order. The
reporter measured the existing switch twice — once through the profile at
session start, once by toggling a live session — and in both runs Kandev logged
`set profile mode on ACP session` with `mode: bypassPermissions`, the agent
stated in its own output that the permissive mode was active, and the
state-changing commands were still refused. None of those three signals
observes enforcement.

This work order is accepted against an executed state-changing Git command
whose commit object resolves, in work order 06. A green unit test proving the
mode reached the configuration boundary is necessary and not sufficient.

It is also the only control left. The reporter measured all four — profile
mode, live mode switch, `cli_flags`, and `auto_approve` — and none produces an
unattended state-changing command today.

## Files likely touched

- `apps/backend/internal/agent/agents/agent.go`
- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/internal/agent/runtime/lifecycle/command.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_docker.go`
- New `apps/backend/internal/agent/runtime/lifecycle/initial_mode_test.go`

## TDD sequence

1. Add failing tests: a Claude ACP launch writes the resolved effective mode
   into the per-session configuration directory and exports its path; an
   existing unrelated key in that file survives the write; a host-executor
   launch does not touch the user home; an agent with no declared channel takes
   the post-creation path and records best-effort; an unavailable mode is
   recorded with its reason.
2. Run the focused command and confirm the expected failures.
3. Implement the declaration, the host-executor session directory, the settings
   write, and the sandbox declaration for container executors.
4. Re-run the focused command; all pass.

## Verification

```bash
cd "$(git rev-parse --show-toplevel)/apps/backend" && go test ./internal/agent/agents/... ./internal/agent/runtime/lifecycle/... -race -count=1
```

## Dependencies

None. Disjoint from work orders 01, 03, 04, and 05; complementary to 02, which
adds confirmation on top of the delivery this work order introduces.

## Risks

The reporter's measurement shows the permissive mode currently behaving worse
than the default mode: the default profile raised a prompt and the command ran
once answered, while the permissive profile refused with no prompt. If
delivering the mode at start does not change that, the remaining behavior is
provider-side and work order 06 is where that is recorded — do not compensate
for it here.

The reporter's user-level `~/.claude/settings.json` carries no `permissions`
block, so the scope this work order writes into is currently empty rather than
conflicting. Do not assume that holds on every install: merge into existing
content and write only the Kandev-owned keys.

Extending the per-session configuration directory to host executors changes
where a host-executor agent reads its configuration. That is the intended
isolation, but it also means a developer's existing local agent settings stop
applying inside a Kandev session. Confirm with the user before landing, and
cover the migration in the same change if existing behavior must be preserved
for some keys.

## Results

Done, with one deliberate deviation from the design.

Implemented:
- `agents.InitialModeDelivery` on `RuntimeConfig`, with the `settings_file`
  channel and an absent default. Claude ACP declares
  `CLAUDE_CONFIG_DIR` / `settings.json` / `permissions.defaultMode`, its four
  mode values, and `IS_SANDBOX=1` as the container sandbox declaration.
- `internal/agent/runtime/lifecycle/initialmode` materializes the per-session
  configuration directory under the Kandev root and returns its path.
- `Manager.applyInitialMode` resolves the effective mode before the process
  starts and exports the configuration directory into the launch environment.
  `Manager.launchSessionMode` mirrors `effectiveSessionMode`'s precedence, so a
  persisted session mode still wins over the profile mode.
- The sandbox declaration is added only for container runtimes (docker, remote
  docker, kubernetes), where the bridge would otherwise disable a permissive
  mode for the root process identity.
- An agent without a declared channel, or a mode its channel cannot express,
  produces a non-delivered outcome with a reason and a Warn. The post-creation
  `session/set_mode` stays in place for those and for later switches.

**Deviation: the directory is materialized by symlink, not by redirection
alone.** The design said to point the agent at a Kandev-owned directory. Doing
only that would have taken the agent's credentials with it — `.credentials.json`
lives in exactly that directory — so every host-executor session would have lost
its authentication. `Materialize` therefore links every source entry into the
session directory and owns only the settings file. A token the agent refreshes
reaches the real file through the link, and no secret is duplicated per session.

**Blast radius is bounded by an empty mode.** `applyInitialMode` returns
immediately when no mode is requested, so a profile that leaves `mode` empty
keeps its launch environment byte for byte. Only sessions that ask for a mode
are redirected.

Verification (2026-09-22):

```
go test ./internal/agent/agents/... ./internal/agent/runtime/lifecycle/... -race -count=1
```

all `ok`. Regression sweep over `./internal/orchestrator/...`,
`./internal/backendapp/...` and `./internal/agentctl/...` clean.

Not covered here: whether the delivered mode changes what the provider actually
enforces. That is work order 06, and it is measured against an executed
state-changing Git command, never against a displayed mode.
