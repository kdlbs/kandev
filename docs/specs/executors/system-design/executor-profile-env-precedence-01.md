---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001
created: 2026-08-17
owners:
  - tbd
---
# Executor-Profile Environment Precedence System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

Environment values split into two kinds that today share one home:

- **Machine-scoped**: where inference goes, where tools live. `ANTHROPIC_BASE_URL` is the
  clearest case. It describes the host, not the model.
- **Agent-scoped**: `CLAUDE_CONFIG_DIR`, `ANTHROPIC_DEFAULT_SONNET_MODEL`, `MCP_TIMEOUT`.

Machine-scoped values live on the agent profile, so a profile named "Opus" means "Opus, and
always talk to localhost:3456". That is true on the local host and false on an SSH executor.
Today the only workaround is duplicating the agent profile per executor: N agents x M
executors.

A user who sets `ANTHROPIC_BASE_URL` on both an agent profile and an executor profile does not
get a choice. The task fails during environment resolution.

## Current behaviour (verified)

Verified 2026-08-17 by reading the implementation, not by inference.

Environment values reach an agent as `Definition` values that keep their source identity until a
single composition boundary
(`apps/backend/internal/agent/runtime/environment/environment.go`). `Definition` carries `Key`,
`Literal`, `SecretID`, `Origin`, `WorkspaceID`.

`selectDefinitions` (`environment.go:95`) sorts by `Key`, then `Origin`, then `SecretID`, then
walks the sorted list. For a repeated key it compares `definitionIdentity` (`environment.go:149`),
which is `"secret:" + SecretID` for a secret-backed definition and
`"literal:" + sha256(Literal)` otherwise. Identical identities are merged and every origin is
recorded. Different identities are collected into a conflict set and `Resolve` returns a
`*ConflictError` naming the key and every participating origin, sorted:

```text
environment key "ANTHROPIC_BASE_URL" has conflicting definitions from agent profile, executor profile
```

There is deliberately no precedence. The resolver refuses to choose.

### The two assembly sites

Definitions are assembled at exactly two places, and both funnel into the same resolver:

1. **Primary launch / resume / execute.**
   `Executor.resolveLaunchEnvironment`
   (`apps/backend/internal/orchestrator/executor/task_environment.go:90`) builds sources from
   `req.Env` (origin `managed runtime`), the **executor** profile's `EnvVars` (origin
   `executor profile`, confirmed via `executorConfig.ProfileEnvVars` populated from
   `profile.EnvVars` at `executor_state.go:203`), and each attached repository's
   `SecretBindings` (origin `repository <name>`). It then runs `runtimeenv.Validate` as a
   shape-only preflight and sets `EnvironmentResolutionRequired = true`. Called from
   `executor_interaction.go:747`, `executor_resume.go:964`, `executor_execute.go:1100`.
2. **Restart recovery / workspace-only.**
   `Manager.prepareExecutionEnvironment`
   (`apps/backend/internal/agent/runtime/lifecycle/manager_execution.go:740`) assembles
   repository and executor-profile definitions directly.

Both reach `Manager.resolveStrictEnvironment`
(`apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go:31`), which appends the
remaining origins and calls `runtimeenv.Resolve`. Both get there through
`Manager.buildEnvForExecution` (`manager_startup.go:295`): site 1 sets
`EnvironmentResolutionRequired = true` at `task_environment.go:121`, site 2 at
`manager_execution.go:772`, and `buildEnvForExecution` dispatches on that flag at `:303`.

`manager_interaction.go:1023` is unrelated: it resolves a task **environment ID**, not
environment variables.

#### The legacy fill-missing branch is out of scope

`buildEnvForExecution` has a THIRD branch (`manager_startup.go:314-339`), taken when neither
`EnvironmentFinalized` nor `EnvironmentResolutionRequired` is set. It never calls the resolver: it
copies `req.Env`, merges agent-profile values through `mergeAgentProfileEnv` /
`mergeAgentProfileEnvFromInfo`, then fills agent-runtime defaults with a
`if _, exists := env[k]; !exists` guard.

This spec does not change that branch, and no tier logic applies there. It is a named exclusion
rather than a claim of unreachability: both assembly sites above set the flag, and
`manager_launch.go:1254` sets it back to `false` only AFTER `buildEnvForExecution` has already
resolved (alongside `EnvironmentFinalized = true`, so a re-entry takes the first branch, not this
one) — but a `LaunchRequest` that reaches `buildEnvForExecution` having passed through neither
assembly site would land here, and this spec does not attempt to prove none does.

Nothing is lost by excluding it. Executor-profile definitions are attached ONLY by the two
assembly sites, so this branch never has an executor-profile value to weigh against an
agent-profile one — the conflict this feature resolves cannot arise in it. AC-49 is scoped to
strict-resolution launches for exactly this reason. Making the legacy branch tier-aware would mean
teaching it to load executor profiles, which is a behaviour change well outside this card.

### The complete origin set

Every origin string produced today, with its source:

| Origin | Constant / source | Backing |
| --- | --- | --- |
| `managed runtime` | `managedRuntimeOrigin`, `environment_resolution.go:17` | literal |
| `managed credentials` | `managedCredentialOrigin`, `environment_resolution.go:18` | literal |
| `managed agent defaults` | `managedAgentDefaultsOrigin`, `environment_resolution.go:19` | literal |
| `agent profile` | `agentProfileOrigin`, `environment_resolution.go:20` | literal or secret |
| `executor profile` | `executorProfileOrigin`, `environment_resolution.go:21` | literal or secret |
| `repository <name>`, or `repository` when the name is blank | built at `environment_resolution.go:225` and `task_environment.go:77` | secret only |

`managed runtime` covers `KANDEV_INSTANCE_ID`, `KANDEV_TASK_ID`, `KANDEV_SESSION_ID`,
`KANDEV_AGENT_PROFILE_ID`, `KANDEV_EXECUTION_PROFILE_ID`, `AGENT_MODEL`,
`AGENTCTL_AUTO_APPROVE_PERMISSIONS`, `GOCACHE`, and everything in `req.Env`.

The orchestrator site hardcodes the literal strings `"managed runtime"` and
`"executor profile"` (`task_environment.go:63`, `:67`) instead of importing the lifecycle
constants. That is a live drift hazard for any rule keyed on origin.

### The two profile kinds are NOT validated the same way — verified

This is load-bearing for AC-6, AC-38, AC-38a and AC-40, and three earlier drafts of this spec got
it wrong, so it is stated once here as fact and every AC below refers to it rather than restating
it.

ALL THREE save-time validators live ONLY in the **agent**-profile controller,
`apps/backend/internal/agent/settings/controller/profile_crud.go`:

| Rule | Symbol | Applies to |
| --- | --- | --- |
| value-or-secret required | `validateEnvVarValue` (`:913`, "must set either value or secret_id") | agent profiles ONLY |
| reserved key rejected | `TASK_DESCRIPTION` / `KANDEV_*` check (`:904`, constants `:867-868`) | agent profiles ONLY |
| duplicate key rejected | `env_vars[i].key duplicates env_vars[j].key` (`:908`) | agent profiles ONLY |

A `grep` for those symbols across the whole backend returns those lines and nothing else.

The **executor**-profile save path is a different code path entirely — HTTP
(`internal/task/handlers/executor_profile_handlers.go`), WS, and MCP
(`internal/mcp/handlers/config_executor_handlers.go`) all reach
`task/service.CreateExecutorProfile` / `UpdateExecutorProfile`
(`internal/task/service/service_resources.go:1587` / `:1625`), which validates exactly two things:

- global-secret references (`validateGlobalProfileEnvRefs`, `service_resources.go:1665`), and
- the Sprites token requirement.

It enforces NO value-or-secret rule, NO reserved-key rule, and NO duplicate-key rule. The web
editor does not compensate: `rowsToEnvVars`
(`apps/web/components/settings/profile-edit/env-vars-card.tsx`) filters empty KEYS only, so
`{key: "K", value: "", secretID: ""}` is savable from the UI as well as from HTTP, WS and
agent-driven MCP.

**Consequences this spec must own, and does:** an executor profile can carry a blank-valued entry
(AC-38, AC-38b), a `KANDEV_*` key (AC-40), and two entries with the same key and different values
(AC-6a). None of these is reachable from an agent profile.

### The primary path strips two shell keys before assembly

`resolveLaunchEnvironment` calls `applyPreferredShellEnvWithStatus`, and when a preferred shell
applies it drops `SHELL` and `AGENTCTL_SHELL_COMMAND` from the executor profile's env vars
before `taskEnvironmentSources` runs (`withoutPreferredShellProfileEnvVars` /
`isPreferredShellEnvKey`, `task_environment.go:141-154`; pinned by
`TestResolveLaunchEnvironment_PreferredShellWinsOverProfileShell`,
`task_environment_test.go:93`).

This spec does not change it, and it is recorded only so nobody debugging tier precedence with
key `SHELL` loses an hour watching an executor-profile value vanish before it ever reaches the
resolver. It is on the PRIMARY path only; the restart-recovery path applies no such filter.

### `managed agent defaults` is already lower than `agent profile`

`appendAgentRuntimeDefaults` (`environment_resolution.go:75`) skips any key for which
`hasEnvironmentDefinition` is already true. Agent-profile definitions are appended first
(`environment_resolution.go:40` before `:43`), so an agent-runtime default never competes with an
agent-profile value. This is pinned by
`TestAppendAgentRuntimeDefaultsFillsOnlyMissingKeys`
(`environment_resolution_defaults_test.go:17`), which asserts `"profile-wins"`.

### `mergeEnvFillMissing` is a different layer

`mergeEnvFillMissing` (`apps/backend/internal/agent/runtime/lifecycle/profile_env.go:57`) fills
agent-profile values into an already-built map at agent launch. The conflict check runs first and
a conflicting pair never reaches it. This spec does not change that function.

### The governing ADR already declares the layering

ADR-2026-08-03 (`docs/decisions/2026-08-03-scope-and-merge-repository-secrets.md`) models the
environment as a tree and states, twice, that the agent profile sits **below** the task
environment:

> Agent profile environment: existing fill-missing defaults applied after the task environment is
> resolved; Global secrets only.

> The existing agent-profile contract remains a lower-priority default: it fills keys absent from
> the resolved task environment.

The strict resolver flattened the agent profile into a peer origin of the task environment. That
contradicts the ADR. This spec restores the declared layering rather than inventing a new tier.

The same ADR fixes the invariants this spec must not break:

- "the same key bound to different secret IDs blocks launch";
- "an executor literal and repository secret using the same key block launch, even if their
  current plaintext happens to match";
- "a repository key colliding with a managed runtime value blocks launch rather than replacing
  it";
- "repository order and task-repository position never choose a winner";
- alternative 6, "Compare decrypted values and deduplicate equal plaintext", was **rejected**.

## Decision summary

1. Environment origins are ranked into three tiers. A higher tier wins a key outright.
2. Within the top tier, origins remain peers. Disagreement there still blocks launch, unchanged.
3. Precedence applies only when **every** definition for that key is literal-backed. If any
   definition for the key is secret-backed, the conflict still blocks launch.
4. An override that actually takes effect is recorded and logged. Silence is not acceptable for a
   value that changes where an agent sends inference.
5. Nothing migrates. This is a new capability, not a data change.
6. The primary assembly site gains ONE behaviour change beyond precedence: it drops a profile entry
   whose value and secret are both empty, matching what the restart-recovery site already does.
   Without it, tier precedence would let a blank executor-profile value beat a real agent-profile
   value and launch the agent with the key set to the empty string. See AC-38.

## Precedence model

### Tiers

| Tier | Name | Members |
| --- | --- | --- |
| 1 | Authoritative | `managed runtime`, `managed credentials`, `executor profile`, any origin for which `IsRepositoryOrigin` reports true, **and any origin not recognised by this table** |
| 2 | Profile default | `agent profile` |
| 3 | Agent runtime default | `managed agent defaults` |

Tier 1 members are peers with each other. Tier 1 beats Tier 2 beats Tier 3.

Tier membership is decided by the origin string. Because the two assembly sites do not share
constants today, tier classification must be driven from one shared, exported table so the sites
cannot drift.

#### What counts as a repository origin — ONE normative rule

There is exactly one definition, and everything else in this spec refers to it rather than
restating it:

> An origin is a REPOSITORY ORIGIN **if and only if** its first whitespace-delimited field — the
> first element of `strings.Fields(origin)` — is exactly `repository`.

This is the NORMATIVE rule, and `IsRepositoryOrigin(origin string) bool` (AC-19c) is its single
implementation. Both the tier classifier and the metric-label normaliser (AC-25) call that one
function; neither re-derives the rule.

`OriginRepositoryPrefix` (the literal `"repository "`) is a CONSTRUCTION constant, not a matching
rule. The assembly sites use it to BUILD an origin (`OriginRepositoryPrefix + name`); nothing
classifies with `strings.HasPrefix` against it. The distinction is load-bearing because the two
are not equivalent: `task_environment.go:77` and `environment_resolution.go:225` emit the BARE
string `repository` when the repository name is blank, and a `HasPrefix(origin, "repository ")`
test rejects that string while the normative rule accepts it. Both readings happen to land on
Tier 1 and on the same label today, so this is a latent divergence rather than a live bug — but
two independent spellings of this rule is exactly the drift AC-19a exists to prevent, so the spec
keeps only one.

Worked consequences of the normative rule, so Build does not have to guess at the boundaries:

| Origin | Repository origin? | Why |
| --- | --- | --- |
| `repository` | yes | sole field is `repository` (the blank-name case, produced today) |
| `repository app` | yes | first field is `repository` |
| `repository  app` (two spaces) | yes | `strings.Fields` collapses runs of whitespace |
| `repository\tapp` (tab) | yes | `strings.Fields` splits on any whitespace, not only spaces |
| `repositoryapp` | no | first field is `repositoryapp` — unrecognised, so Tier 1 by AC-35 |
| `` (empty) | no | no fields at all — unrecognised, so Tier 1 by AC-35 |

### Where the constants and the tier table live

Both live in `apps/backend/internal/agent/runtime/environment` — the `environment` package, which
both assembly sites already import as `runtimeenv`. That package is where tier classification
actually runs, and both sites already depend on it, so this introduces no new edge in the import
graph. Concretely it gains exported constants `OriginManagedRuntime`, `OriginManagedCredentials`,
`OriginManagedAgentDefaults`, `OriginAgentProfile`, `OriginExecutorProfile`, and
`OriginRepositoryPrefix` (the literal `"repository "`, a construction constant per the normative
rule above), plus the tier table and its classifier. The unexported lifecycle constants at
`environment_resolution.go:17-21` and the hardcoded orchestrator strings at
`task_environment.go:63` and `:67` are replaced by references to them.

**The classifier's exported surface is named here, not left to the builder** (AC-19c). The spec
declares every other symbol it introduces, and the tier table is the one artefact AC-19a and AC-35
both hang off, so it gets the same treatment:

```go
// TierForOrigin classifies one origin string into its tier. It is the ONLY
// place the tier table is read. An origin the table does not name classifies
// as TierAuthoritative (AC-35).
func TierForOrigin(origin string) Tier

// IsRepositoryOrigin reports whether origin is a repository origin, per the
// single normative rule: strings.Fields(origin)[0] == "repository".
// TierForOrigin and NormalizeOriginLabel both call it; neither restates it.
func IsRepositoryOrigin(origin string) bool
```

THE TABLE ITSELF IS UNEXPORTED — an unexported package-level `map[string]Tier` reached only
through `TierForOrigin`. This is deliberate rather than incidental: an exported map is mutable by
any importer, so exporting it would let a caller silently re-rank precedence at run time and
defeat the single-source-of-truth guarantee AC-19a is asking for. A function cannot be mutated,
and it is also where the "unrecognised origins are Tier 1" default and the `IsRepositoryOrigin`
call live, neither of which a bare map lookup can express.

This is a placement decision about ownership, not a workaround for an import restriction. The
orchestrator already imports `runtime/lifecycle` directly (`executor.go`,
`executor_environment_reuse.go`, `executor_execute.go`), so exporting the constants from
`lifecycle` would also have compiled. `environment` is chosen because the resolver owns the tier
decision, and a table read by the resolver should not live in one of the resolver's callers.

### Why `repository` is in Tier 1 and not above `executor profile`

Repository bindings are secret-only by ADR construction ("Bindings contain no literal values"),
so a repository definition is always secret-backed. Rule 3 therefore blocks any repository
disagreement before tier rank is ever consulted. Placing repository in Tier 1 keeps the ADR's
"never silently replaced" guarantee absolute, without needing a repository-specific carve-out.

### Why `managed credentials` is in Tier 1

A managed credential is the value the install has been configured to authenticate with, resolved
from the credential manager for a key the agent runtime declares in `RequiredEnv`. It describes
the machine's identity, not the agent's persona, which is the same test that puts
`managed runtime` and `executor profile` in Tier 1.

State the consequence plainly, because AC-3 changes behaviour: a `managed credentials` value and a
differing `agent profile` literal for the same key block the launch today, and after this change
the managed credential wins and the agent-profile literal is discarded with an override record.
For an API-key-shaped key that changes which account is billed. That is the intended direction —
a per-profile literal should not silently redirect billing away from the install's configured
credential — and it is observable, logged (AC-23) and counted (AC-24) rather than silent. The
common case is unaffected either way: `appendRequiredCredentialDefinitions` skips any key whose
credential lookup returns empty, and an identical pair merges without an override (AC-15).

### Why unrecognised origins are Tier 1

Fail closed. A typo'd or newly added origin must never become a silent loser. Classifying it into
the peer group preserves today's conflict behaviour for anything this table does not name.

## The secret veto

For one key, after collecting all its definitions:

- If two or more definitions have different `definitionIdentity` values, and **any** definition
  for that key is secret-backed (`SecretID != ""`), the resolver returns `*ConflictError` naming
  every participating origin — defined precisely below as every CONTRIBUTING origin of every
  surviving identity, de-duplicated and sorted ascending. Tier rank is not consulted.
- The veto is symmetric. It applies whether the higher-tier definition, the lower-tier
  definition, or both are secret-backed.

Rationale: silently discarding a token source, or silently choosing between two token sources, is
exactly the failure ADR-2026-08-03 rejected. Rotation and identity semantics matter more than
convenience, and the values are not comparable without decrypting them, which the ADR also
rejected.

### "Every participating origin" means every CONTRIBUTING origin, on the veto path too

`ConflictError.Origins` SHALL hold **every contributing origin of every surviving identity**,
de-duplicated and sorted ascending — NOT one origin per surviving identity, and NOT the surviving
`Definition.Origin` field alone.

This is the same rule procedure step 6 states for the precedence path, and it is spelled out here
because the veto path needs it MORE, not less. Merging (step 3) runs before the veto (step 5), so
a merged identity reaching the veto already carries several origins: a `repository app` binding
and an `executor profile` entry that reference one `SecretID` share the identity `"secret:<id>"`
and collapse into a single surviving definition whose `Origin` field can only name one of them.
An implementation that enumerates `Definition.Origin` per surviving identity therefore drops
`executor profile` (or `repository app`, depending on sort order) from the error the user reads,
while every precedence test still passes.

The veto path is also the more production-reachable of the two: ADR-2026-08-03 makes every
repository binding secret-only, so real-world secret disagreements arrive here rather than at
step 6. This preserves today's behaviour rather than changing it — `selectDefinitions` already
folds the accumulated origin set of the prior identity into the conflict set before adding the
differing definition's origin, and already returns that set de-duplicated and sorted. AC-12a
observes it, and the existing
`TestResolveEnvironmentSources_ReportsEveryConflictingOrigin` asserts against the same field.
