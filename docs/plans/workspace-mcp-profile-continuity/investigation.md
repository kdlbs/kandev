# Issue 5044 restart investigation

## Finding

The reported loss of seven canvas tools and the title tool reproduces on v0.98.0.
It occurs when workspace access creates a basic agentctl helper before the same conversation resumes.
Historical workspace promotion does not deliver the complete resolved MCP profile to that helper.
Starting the resume before workspace access avoids the loss.

The same message-resume and workflow-reuse sequences pass on baseline `5d6adcbca18231a5aa3726ca9ee642bfa255cd4a`.
Its existing package binding path delivers the full profile before the provider process starts.
The proposed additional profile endpoint is cancelled.
Permanent regression coverage of this existing path is implemented in
`apps/backend/internal/backendapp/restart_mcp_catalog_test.go`.

## Setup and limits

Investigation date: 2026-11-04.
Historical source: tag `v0.98.0`, commit `2d93502c256`.
Current source: `5d6adcbca18`.

Each version ran native backend, agentctl, and mock-agent binaries against its own disposable SQLite database and local worktree repository.
No user instance, database, provider credentials, or conversation was used.
The backend's real orchestrator, recovery, workspace preparation, MCP resolver, policy reader, lifecycle, and HTTP/SSE transports were exercised.
The mock provider used the `claude-acp` identity and retained its saved native conversation ID across restarts.

The isolated profile replaced external ACP and service calls with mocks.
Canvas availability was explicitly enabled, as required to test the gated tools.
Agent survival was disabled for replacement cases and enabled for the retained-process control.
These are normal production code paths with disposable test inputs, not a real Claude discovery test.
No fixture forced a nil built-in package manifest.

The reproduction does not establish the behavior of real Claude ToolSearch, Claude's `/model` command, or the reporter's original processes.
It did not capture outgoing generated prompt text during the native-process runs.
Task 02 implements the startup-guidance agreement assertion in permanent composition coverage.

## Reproduction procedure

1. Create a local Git repository with one commit and register it in an isolated Kandev workspace.
2. Create a task through `POST /api/v1/tasks` with `start_agent: true`, the worktree executor, the mock Claude ACP profile, and the default In Progress step.
   Use `auto_title: true` for the title-owner cases, so the normal title path creates the pending owner.
3. Run `e2e:mcp:kandev:list_canvases_kandev({})` and wait for `WAITING_FOR_INPUT`.
   Record session/native IDs, execution identity, attachment inventory, and initialized MCP `tools/list` names.
4. Stop only the owned backend with SIGTERM, then start a new native backend against the same disposable database.
5. For the helper-first case, send `workspace.tree.get` before `message.add`.
   For the direct-resume control, send `message.add` without workspace access first.
6. Wait for a new completed turn in the same task session, then initialize a separate MCP client and record `tools/list` again.
7. For workflow reuse, restart again, access the workspace, and move the task from Review to In Progress with `POST /api/v1/tasks/{id}/move`.
   Use entry instructions to request the same MCP call.
8. For the current retained-process control, enable agent survival, then verify the execution, native process, binding, and conversation across restart.

The MCP probe completed `initialize`, `notifications/initialized`, and `tools/list` over HTTP using protocol version `2025-03-26`.
The provider also attached through the real SSE client and called `list_canvases_kandev`.

## Results

| Version | Sequence after backend restart | Canvas tools delivered | Eligible title tool delivered | Same conversation |
| --- | --- | --- | --- | --- |
| v0.98.0 | Workspace first, message resume | 0 of 7 | Not eligible in this case | Yes |
| v0.98.0 | Message resume first | 7 of 7 | Not eligible in this case | Yes |
| v0.98.0 | Workspace first, message resume, pending title owner | 0 of 7 | No | Yes |
| v0.98.0 | Workspace first, workflow reuse, pending title owner | 0 of 7 | No | Yes |
| 5d6adcbca18 | Workspace first, message resume | 7 of 7 | Not eligible in this case | Yes |
| 5d6adcbca18 | Message resume first | 7 of 7 | Not eligible in this case | Yes |
| 5d6adcbca18 | Workspace first, message resume, pending title owner | 7 of 7 | Yes | Yes |
| 5d6adcbca18 | Workspace first, workflow reuse, pending title owner | 7 of 7 | Yes | Yes |
| 5d6adcbca18 | Workspace first, retained process | 7 of 7 | Not eligible in this case | Yes |

The title-owner metadata still pointed to the same session after each title-case restart.
Title loss on v0.98.0 was therefore not explained by title settlement or an ownership change.

The old helper-first catalog fell from 45 to 38 tools, or from 46 to 38 for the pending title owner.
The current core catalog stayed at 41 tools, or 42 for the pending title owner.
Current package splitting moves other tools to separate endpoints, so totals are not directly comparable across versions.
This local repository has fewer provider-specific tools than the issue reporter's context.
The evidence is the missing names and groups, not a universal tool count.

The latest historical workflow tool-call output contained:

```text
tool 'list_canvases_kandev' not found: tool not found
```

The equivalent current workflow call returned:

```json
{"canvases": []}
```

The current retained case kept execution `b72b7904-cb45-4730-ae00-062433dea6ef`.
ACP trace records show both prompts on native process `1047573` with the same connection and no intervening session load.
This distinguishes retained-process adoption from replacing the process while preserving only its conversation ID.

### Fixture identities

| Version and case | Kandev session | Native conversation |
| --- | --- | --- |
| v0.98.0, ordinary title | `6506f17d-a7ec-45bd-be3d-40c54382fe95` | `mock-session-1048934-1` |
| v0.98.0, pending title owner | `11707ad9-6b9d-4fba-a2d0-96eed33f37d9` | `mock-session-1053126-1` |
| 5d6adcbca18, ordinary title | `319d5c5f-57e9-4fd8-b0ec-1e84dae31891` | `mock-session-1038311-1` |
| 5d6adcbca18, pending title owner | `9f656845-979b-4daa-9f4c-e8ab0c31111f` | `mock-session-1053206-1` |

## Source trace

1. `workspace.tree.get` reaches `GetOrEnsureExecution` through the workspace file handler.
2. `prepareExecutionCreateRequest` creates a workspace-only helper from `WorkspaceInfo.McpMode`.
   It cannot yet use the later launch request's full task/session profile.
3. `Executor.applyResumeMCPSettings` resolves the complete profile.
   `buildLifecycleLaunchRequest` carries `McpProfile` into lifecycle resume/launch.
4. v0.98.0 `Manager.promoteWorkspaceExecution` configures the reused helper with providers but omits that full profile.
   The helper's basic tool catalog survives into the resumed process.
5. Current production wiring installs the built-in policy reader.
   `PrepareTaskSessionMCPProfile` supplies a package-bearing profile for ordinary task starts.
6. Current promotion calls `prepareBuiltinWorkspaceExecution` before command configuration.
   The existing `PUT /api/v1/mcp/builtin-binding` operation binds the complete profile under the process-start lock.
   The persisted profile includes `canvas-authoring`, `task-title` where eligible, and `user-question`.

Commit `a1013e89fc460b39b17003e5022d756bf480e3d2`, from [PR #4762](https://github.com/kdlbs/kandev/pull/4762), added this preparation path.
The old/current runtime comparison and source trace support its role in correcting the reproduced failure.
An isolated-commit bisect was not performed.

Canvas tools are built-in capability registrations.
This sequence does not require the issue's proposed plugin-registration race.
The manually manifest-free lifecycle failure on current main is insufficient evidence for a new monolithic endpoint in ordinary task starts.

## Local artifacts and commands

Current evidence root: `/tmp/kandev-iso-48429-9pbxwG`.
Historical source root: `/tmp/kandev-issue5044-v098-z7z786gd`.
Historical evidence root: `/tmp/kandev-issue5044-v098-z7z786gd/runtime`.

The current root contains temporary `repro.mjs`, `resume.mjs`, and `workflow.mjs` drivers.
Each evidence root contains initial/resumed case JSON, `workflow-resumed.json`, backend logs, ACP logs, and `acp-trace.jsonl`.
The current root also contains `repro-summary.json` with the matrix above.
These temporary artifacts are local evidence, not committed tests or portable test dependencies.

Representative commands actually run from the repository root:

```bash
scripts/dev-isolated --timeout 90
node /tmp/kandev-iso-48429-9pbxwG/repro.mjs init
node /tmp/kandev-iso-48429-9pbxwG/repro.mjs create title-helper-first
node /tmp/kandev-iso-48429-9pbxwG/resume.mjs title-helper-first
node /tmp/kandev-iso-48429-9pbxwG/workflow.mjs title-helper-first
```

For the historical runs, the same drivers used these environment overrides:

```bash
REPRO_ROOT=/tmp/kandev-issue5044-v098-z7z786gd/runtime \
REPRO_PORT=48439 REPRO_AGENTCTL_PORT=50439 \
REPRO_BINARY=/tmp/kandev-issue5044-v098-z7z786gd/apps/backend/bin/kandev \
node /tmp/kandev-iso-48429-9pbxwG/resume.mjs title-helper-first
```

The old helper-first diagnostic commands returned exit code 2 when the missing-tool assertions fired.
Current counterparts returned exit code 0.
Historical direct-resume returned exit code 0.
All assertions preserved the native conversation.

The six existing component checks also passed in backendapp, executor, and lifecycle:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -p 1 ./internal/backendapp ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle -run '^(TestBuiltinMCPManagementRoutesAreWired|TestBuiltinFreshResumeAndModelSwitchResolveCurrentPolicy|TestExistingWorkspacePreparesBuiltinPackagesBeforeStart|TestBuiltinPromotionBindsWorkspaceBeforeCommand|TestBuiltinPromotionVerifiesRetainedBindingWithoutReplacingIt|TestStartupMCPReadyBeforeRecoveryAndLaunch)$' -count=1)
```

### Teardown

Both owned instances were stopped with the scoped helper:

```bash
scripts/kandev-kill --pidfile /tmp/kandev-dev-isolated-48429.pid --yes
scripts/kandev-kill --pidfile /tmp/kandev-dev-isolated-48439.pid --yes
```

The instance and control port ranges have no remaining listeners.
A process scan found no process with either disposable home.
Local logs and databases remain available for review.

## Reporter evidence

The user authorized an information request.
The [posted comment](https://github.com/kdlbs/kandev/issues/5044#issuecomment-6637018148) initially requested adapter details and restart evidence.
It was updated after the reproduction to report the historical cause and the successful current binding path.
It now asks the reporter to retry a build containing PR #4762.
If Claude still loses tools, it requests Kandev/Claude ACP versions, redacted restart/resume logs with a UTC time, and clarification of `/model`.
The live comment was fetched and its full body verified after both posting and updating.

A current-main reproduction with a correct helper inventory would move the investigation to provider attachment/discovery.
No issue closure, additional endpoint, or live repair follows from the mock-provider result alone.
