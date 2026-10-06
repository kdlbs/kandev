---
status: current
system: agents
created: 2026-09-27
requirements:
  - REQ-AGENTS-OPENCODE-V2-001
  - REQ-AGENTS-OPENCODE-V2-002
owners:
  - Kandev
---

# OpenCode V2 Adoption System Design

## Purpose and boundaries

The agent system owns runtime selection, installation, and native session restoration.
Keep the existing `opencode-acp` identity and ACP adapter for both OpenCode distributions.
This design extends [runtime updates](runtime-updates-01.md) and defines the OpenCode exception to
[default activation](runtime-default-activation.md).
Implementation is pending in the [plan package](../../../plans/opencode-v2-adoption/plan.md).

| Requirement | Design sections |
| --- | --- |
| REQ-AGENTS-OPENCODE-V2-001 | Runtime definitions, Persistence, Bootstrap, Activation, Settings |
| REQ-AGENTS-OPENCODE-V2-002 | Session continuity, Failure boundaries, Verification |

## Evidence and settled choices

The user confirmed v2 for new installations and an explicit migration for existing v1 users.
Migration uses Kandev's managed installation and leaves an independently installed CLI unchanged.
The update dialog belongs to agent Settings; its runtime choice is install-wide, not per profile.

The existing store already saves package and version. Its readers assume one trusted package per agent.
`Store.Get` filters selections by package, and `Store.ReconcileDefaults` deletes selections after a package change.
Changing only the OpenCode package constant would therefore force adoption and lose prior selections.
`runExactCandidate` also prefers a global native update when `opencode` is on PATH.
These are explicit integration changes, not a new ACP protocol implementation.

The earlier isolated experiment loaded a 1.18.5 conversation in 2.0.18 and recalled its previous codeword.
A temporary Kandev adapter test completed a v2 turn. Neither result proves full application migration.
The exact candidates for permanent compatibility tests are managed v1 1.18.32 and v2 2.0.18;
include the observed native v1 1.18.5 fixture where practical.

## Runtime definitions and command resolution

Maintain two trusted OpenCode definitions in `internal/agent/agents`:

| Family | Package | Initial reviewed pin | ACP arguments |
| --- | --- | --- | --- |
| v1 | `opencode-ai` | 1.18.32 | `acp --print-logs --log-level ERROR` |
| v2 | `@opencode/cli` | 2.0.18 | `acp --print-logs --log-level error` |

Keep both package pins in `managed_npm_runtime_versions.json` and its maintenance workflow.
Do not resolve `latest` during launch. Version catalogues must reject versions outside their family's major.
A future major requires a separate reviewed adoption policy; package identity alone does not permit it.

Add a resolved runtime value carrying trusted package, effective version, source, and command arguments.
Resolve it before building commands; do not mutate the registry's singleton agent per request.
Pass this value through the current `CommandOptions` and runtime-selection seams as needed.
Other agents retain their existing fixed-package resolution.

All OpenCode consumers must use this resolution: registry probes, host utilities, lifecycle profile resolution,
standalone sessions, executor preflight/install, container and SSH commands, retry/cache repair,
Settings preview and activation, and Kandev-created interactive CLI commands.
For OpenCode the interactive CLI and ACP runtime come from the same distribution.
Construct interactive commands without `acp` and retain their existing resume/model/prompt arguments.
Do not apply this rule to other providers with separate interactive and ACP packages.
Custom user commands remain explicit overrides and the UI must not claim to migrate them.

Managed selection overrides native preference, including executor `native_binary` metadata.
Stage managed candidates through `CacheUpdateCommand`, never `NativeUpdateCommand`.
Use an explicit managed ACP candidate command, never `RefreshCommand` if it can rediscover PATH.
Installation helpers for managed OpenCode must use the resolved exact package.
Discovery and availability checks must recognize the selected managed runtime even when `opencode` is absent from PATH.
`OpenCodeACP.IsInstalled` currently checks native presence; reconcile that check with managed readiness so a successful
managed installation remains selectable. An unprepared fresh runtime may show an install action; discovery alone must not
perform an unrequested global install. Installation uses the normal Kandev install/launch flow and the resolved managed command.
Before a managed Settings install command reaches the shell, prepare its internal npm project-prefix marker as a private
temporary directory on the backend host. The displayed command may retain the readable marker; execution must not rely
on creating or resolving that path beneath the user's home directory.
Missing artifacts or executor failures must not fall back to another family or a native binary.
Remote launchers use the same family and exact version, but installation occurs in the target executor.
A successful host update does not certify remote installation success.

## Persistence

Add one OpenCode-specific JSON record in the existing install-wide `settings` table:
`managed_runtime.opencode.selection`. No profile column or database table is required.

Proposed record fields:

- `schema_version`: 1.
- `family`: `v1` or `v2`.
- `source`: `native` or `managed`.
- `package`: the family allowlisted package.
- `selected_version`: exact operator version, absent when following the family default.
- `applied_default_version`: the reviewed family default last reconciled.
- `revision`: monotonically increasing integer for stale-preview detection.

For native source, `selected_version` is absent. A bounded local version check determines actual CLI arguments;
observed native version is a status projection, not a promise that Kandev controls that executable.
If a user independently updates native v1 to v2, dispatch using its observed supported major and retain the
same session/home. Report the actual version and offer managed v2 adoption. Do not replace the binary.
An unknown major or failed detection blocks that launch with a diagnostic; do not guess arguments.
Managed v2 never returns to native preference when PATH changes.

A successful migration writes `family=v2`, `source=managed`, package, exact `selected_version`,
current family default, and incremented revision in one settings Save.
This includes an explicit selection when the candidate equals the default.
No separate migrated Boolean can disagree with the executable selection.

The new record is authoritative once written. Legacy `managed_runtime.active.opencode-acp` and
`managed_runtime.default.opencode-acp` rows are import inputs only; remove them after the authoritative write.
A crash during cleanup retries cleanup without importing again.
Exclude OpenCode from generic `ReconcileDefaults` after adding its bootstrap resolver.
Other agents retain current behavior.

Within the adopted family, a reviewed default change clears only `selected_version` and updates
`applied_default_version` in the same record. Family and source remain sticky.
An explicit Use Kandev default action validates the adopted family's default and clears only its version override.
A v2 default update cannot adopt v1 users; a v1 default update cannot return managed v2 users to v1.
Store errors stop startup before consumers or readiness. Invalid records fail visibly rather than choosing v2.
Serialize runtime updates through the existing maintenance coordinator; reject a stale revision before mutation.

## Bootstrap and upgrade compatibility

Bootstrap runs before generic reconciliation and any OpenCode probe or launch.
It performs local reads and, only for discovered native installations, a bounded `--version` check.
It does not install packages, start ACP, or contact npm.

Use this precedence when no authoritative record exists:

1. A native executable that the old resolver would have preferred retains native source.
   Validate its supported major for command dispatch. Preserve legacy managed information until the new record is saved.
2. Otherwise import a valid legacy package/version selection and its family; retain its exact version on this first import.
3. Otherwise an old OpenCode default marker selects managed v1. Use its valid v1 version for the import;
   absent valid version evidence, use the reviewed v1 pin.
4. Otherwise existing OpenCode session/configuration evidence selects managed v1 conservatively.
   Reuse existing repository queries and known data paths; do not inspect conversation contents or credentials.
5. Only an installation with no legacy evidence and no existing native OpenCode selects managed v2 at its reviewed default.

A registry-created profile alone is not evidence of prior use. Old generation markers can exist even if an agent
was never used; conservatively preserving v1 on those older Kandev installations is intentional.
Malformed persisted selections or read failures are errors, not proof of a fresh installation.
Retry after interruption must produce the same result and must not delete the old state before the new record is durable.
No supported downgrade to an older Kandev binary is promised by this one-way settings migration.

## Activation and API contract

Extend existing `/api/v1/agent-update/:agentName` preview and submission contracts.
Proposed optional fields are `target_family` and `expected_runtime_revision`.
Omitting family means an ordinary update in the currently adopted family.
A v1-to-v2 request must explicitly name `v2`; clients cannot supply package names or commands.
Reject other family transitions and a cross-family `use_default` request.
Existing other-agent clients remain valid.

Preview/status/job projections include current family/source, observed and effective version,
trusted target package/version, revision, migration availability, and a structural operation `migrate`.
Use the reviewed v2 pin as the initial migration target; ordinary version browsing remains within a family.
List only trusted stable versions with the existing catalogue limits.
Return typed eligibility/error states, not translated backend sentences as branching keys.

Activation sequence:

1. Require the existing agent-update authorization and claim agent maintenance.
   Compare preview revision and re-evaluate eligibility on the server.
2. Refuse migration while any Kandev-owned OpenCode execution or utility process can still use the shared data.
   Coordinate the check with launch admission; new OpenCode launches cannot race through activation.
   Reuse existing maintenance plumbing and extend lifecycle admission where it does not yet share that guard.
   Do not terminate a task to make migration possible. Unreachable executor liveness blocks activation.
3. Stage the exact managed v2 candidate, using current bounded npm recovery.
4. Probe ACP in a disposable working directory with isolated HOME and XDG paths.
   Do not load the user's session, config, project plugins, or auth during this compatibility probe.
   Protocol initialization and advertised capabilities must succeed without a provider prompt.
5. Save the complete authoritative record atomically. Only this step changes future launch selection.
6. Invalidate old utility instances/capabilities under the same guard, publish the new runtime selection,
   and refresh real-profile capabilities through normal discovery. Release maintenance and finish the job.

The isolated probe verifies transport compatibility, not the user's credential or model access.
Do not publish its empty model catalogue as the user's real capabilities.
If post-activation discovery fails, report v2 as selected with discovery failure and offer retry.
Do not report a failed pre-activation migration or automatically restore v1 after this boundary.
A process interruption before Save preserves v1; interruption after Save reconstructs v2 on restart.
Closing the dialog does not cancel a submitted job or imply rollback.

## Settings and mobile composition

Use `agent-runtime-update-control.tsx`, the existing hooks/API client, and `AgentRuntimeUpdateSurface`.
Entry remains Settings > Agents > OpenCode update control, including the profile context where it is displayed.
V1 ordinary updates remain available. A separate Upgrade to v2 choice previews managed migration.
Show current and target version before submission. The adjacent Settings info icon explains shared profile scope and the unchanged standalone CLI on hover/focus or in a tap-open information sheet. Keep the external-process migration warning visible. The command preview is collapsed by default and expands in place.
No automatic submit or preselected cross-family ordinary Update action is allowed.

Desktop uses the existing Dialog. Phone uses its inset bottom Drawer with a fixed title and action footer,
maximum 92dvh, one scrolling body, safe-area padding, and at least 44px action hitboxes.
This short, infrequent choice fits a drawer. Reuse the shipped update surface and the mobile picker pattern;
do not introduce stacked dialogs or a new Settings page.
Both compositions share selection, progress, permission, and retry state.
Keep ordinary desktop controls at 28px; scope larger targets to phone/coarse pointers.
Focus returns to the update trigger after dismissal. Reopening reads authoritative status.

States: resolving target, ready, active-session blocked, installing, checking ACP, saving,
success, pre-activation failure with v1 retained, and post-activation discovery failure with v2 retained.
Long commands/logs stay contained. Disable duplicate submission during a job.
All copy uses the existing translation system and six supported locales.
The [plan previews](../../../plans/opencode-v2-adoption/plan.md#ascii-ui-preview) define structural review views.

## Session continuity and failure boundaries

Keep agent/profile/task/session IDs, native session ID, working directory, executor, and HOME/XDG storage identity.
Migration does not copy or rewrite user configuration or session databases.
Use advertised ACP resume/load support through the existing adapter.
For saved OpenCode sessions, a failed restoration must not take the lifecycle's generic new-session fallback.
Return an actionable resume error on the same Kandev session. Explicit user creation of another session remains available.

V2 first accesses existing data during a normal post-activation launch.
Do not promise that reselecting v1 reverses upstream data changes. Cross-family downgrade is outside this package.
Kandev does not supervise independent standalone OpenCode processes; the migration notice explains that
users must stop external v1 processes sharing the same data before switching.
The Kandev execution guard prevents owned-process overlap, not arbitrary OS process coexistence.

Keep supported v1 configuration intact. Verify local and HTTP MCP with Kandev's injected configuration;
unsupported MCP transports produce a clear compatibility error rather than a silent omission.
V1 plugins may require user migration; do not rewrite or disable them automatically.
Reconcile new v2 capability shapes at the ACP boundary while preserving v1 tests and other providers.

## Security and observability

Reuse the existing admin update permission, trusted package allowlist, exact version validation,
filtered installer environment, managed npm project prefix, and bounded sanitized job logs.
Settings may display the managed-prefix marker, but install execution resolves the trusted argv and
prepares its prefix on the execution host before serializing that argv for the shell. Never execute
the display string or parse arbitrary shell text to recover command arguments.
Temporary probe paths are owned by the job and cleaned on every exit.
Log source/target family, versions, activation boundary, and failure phase without credentials or conversation data.
No new metric or feature flag is required.

## Verification and sources

Store and composition tests prove bootstrap ordering, crash boundaries, persisted adoption, and per-family reconciliation.
Command tests cover every managed consumer and native v1/v2 dispatch.
Controller and HTTP tests cover authorization, trusted inputs, stale previews, and launch-versus-activation races.
Desktop/phone tests prove the Settings action and durable selection after restart.
Real ACP tests use disposable state: create and prompt in v1, stop it, activate v2, resume the exact session,
and prove history recall. Add a Kandev lifecycle test to establish the persisted task/session mapping.
Record unavailable platform/runtime evidence explicitly; command-contract tests do not prove remote binaries work.

External references, consulted 2026-09-27:

- [OpenCode ACP](https://opencode.ai/v2/docs/cli/acp/): stdio ACP and saved-session operations.
- [OpenCode v1 migration](https://opencode.ai/v2/docs/migrate-v1): shared configuration and plugin compatibility limits.
- [Paseo helper recovery](https://github.com/getpaseo/paseo/pull/5488): research context, not the implementation contract.

## Related decisions

- [Persist OpenCode adoption independently of version defaults](../../../decisions/2026-09-27-opencode-runtime-adoption.md).
