---
status: building
created: 2026-08-17
owner: tbd
---

# Executor-Profile Environment Precedence

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

## Resolution procedure

The order of these steps is part of the contract. A different order produces different answers for
mixed secret and multi-origin cases.

For each environment key, in a deterministic TOTAL sort order over named columns.

The sort that exists today at `environment.go:97` compares `Key`, then `Origin`, then `SecretID`.
That is not a total order, and the gap is load-bearing rather than theoretical: two definitions
agreeing on all three but differing in `WorkspaceID` share one `definitionIdentity` (identity is
`"secret:" + SecretID` for a secret), so they MERGE, and which one survives is decided by input
order — while `WorkspaceID` is exactly what `resolveEnvironmentDefinition` branches on to choose
between a global reveal and `RevealForWorkspace`. Input order could therefore change the revealed
value, which AC-30 forbids.

The sort therefore gains two further NAMED columns and becomes, all ascending:

    Key, then Origin, then SecretID, then WorkspaceID, then Literal

Adding columns can only break ties that were previously left to input order, so no definition set
whose outcome is already deterministic changes (AC-16, AC-29a).

For each key, in that order:

1. **Skip blanks.** Discard any definition whose `Key` is empty or whitespace-only.
2. **Group** the remaining definitions by `Key`.
3. **Merge identical identities.** Definitions sharing one `definitionIdentity` collapse to a
   single surviving definition, recording every contributing origin, exactly as today. Merging
   happens **before** any tier or secret evaluation. The merged group RETAINS THE FULL SET of
   contributing origins; step 6 classifies the group by that set, not by the single surviving
   `Definition.Origin` field.
4. **If one identity survives**, that is the value. No conflict, no override record.
5. **If more than one identity survives, apply the secret veto first.** If any surviving
   definition for that key has a non-empty `SecretID`, return `*ConflictError` whose `Origins`
   holds **every CONTRIBUTING origin of every surviving identity** (per "The secret veto"),
   de-duplicated and sorted ascending. Tier rank is not consulted.
6. **Otherwise compare tiers.** Classify each surviving IDENTITY — not each surviving
   `Definition` — into a tier by `TierForOrigin`. **An identity's tier is the STRONGEST
   (numerically smallest) tier among ALL of its contributing origins.**
   - If the highest occupied tier holds more than one surviving identity, return
     `*ConflictError` whose `Origins` holds every CONTRIBUTING origin of every surviving identity,
     including origins from lower tiers, de-duplicated and sorted ascending.
   - Otherwise the single identity in the highest occupied tier wins, every identity in a lower
     tier is discarded, and one override record is emitted. Both origin lists in that record are
     DE-DUPLICATED SETS (see "The override record"): `WinningOrigins` holds each distinct
     contributing origin of the winning identity once, and `LosingOrigins` holds each distinct
     origin appearing anywhere among the discarded identities once — one entry per ORIGIN, never
     one per discarded identity.
7. **Reveal** secrets only for surviving winners, after the whole key set is conflict-free,
   preserving today's ordering in `Resolve`.

Step 5 preceding step 6 is deliberate: a secret and a literal that disagree must block even when
they sit in different tiers.

Steps 5 and 6 use ONE enumeration rule between them — every contributing origin of every surviving
identity, de-duplicated and sorted. Stating it once per step rather than only on the precedence
path is deliberate: merging happens at step 3, before either, so BOTH paths can face a surviving
identity whose single `Definition.Origin` field under-reports the origins that produced it.

Step 6's "including origins from lower tiers" is deliberate: when the top tier is itself
ambiguous, the error should name every origin the user has to look at, matching today's behaviour
of reporting the complete participating set.

**Step 6 classifies the IDENTITY by its strongest contributing origin, and that word "strongest"
is load-bearing.** Classifying by the surviving `Definition.Origin` instead would silently
discard a Tier 1 value in the most common real configuration this feature exists to serve.
Worked case: key `AGENT_MODEL` carries `executor profile` literal X, `agent profile` literal X
(the SAME literal — precisely the duplicate-the-value workaround the Why section says users
employ today), and `managed runtime` literal Y from `profileInfo.Model`
(`environment_resolution.go:65`). Step 3 merges the X pair into one identity. Under the
five-column sort the surviving `Definition` is the `agent profile` one, because
`"agent profile" < "executor profile"` — so classifying by that single field would put identity X
in Tier 2, leave Y as the sole Tier 1 identity, and hand the key to Y while silently dropping an
`executor profile` literal that disagrees with it. AC-5 requires that pair to conflict. Under the
strongest-contributing-origin rule, identity X is Tier 1 (via `executor profile`), the top tier
holds two identities, and the key conflicts — which is what AC-5 says and what AC-5a pins.

## The override record

The record is a declared Go type, not a shape left to the builder. It lives in the `environment`
package beside the tier table, and it carries identity and rank only — never a value.

```go
// Tier ranks an origin. LOWER IS STRONGER: Tier 1 beats Tier 2 beats Tier 3.
type Tier int

const (
    TierAuthoritative       Tier = 1
    TierProfileDefault      Tier = 2
    TierAgentRuntimeDefault Tier = 3
)

// OriginTier pairs an origin string with the tier it classified into.
type OriginTier struct {
    Origin string
    Tier   Tier
}

// OverrideRecord describes one environment key whose value was chosen by tier
// precedence instead of blocking the launch. It contains no literal value, no
// SecretID, and no decrypted secret.
type OverrideRecord struct {
    Key            string       // the environment key that was overridden
    WinningOrigins []string     // SET: >= 1 entry, each distinct origin ONCE,
                                // sorted ascending; MAY span tiers
    WinningTier    Tier         // the STRONGEST tier among WinningOrigins — the tier the
                                // winning identity was classified into by procedure step 6
    LosingOrigins  []OriginTier // SET: >= 1 entry, each distinct origin ONCE,
                                // sorted by Origin ascending
}
```

`WinningOrigins` is a SLICE, not a string. A merged winner can legitimately have more than one
contributing origin — that is exactly the case AC-8a describes, where two Tier 1 definitions share
one `definitionIdentity`, merge, and then beat a Tier 2 literal.

### BOTH ORIGIN LISTS ARE SETS, NOT BAGS

This is a contract, not an implementation detail, because it decides observable test assertions
and observable counter values:

> **Every origin string appears AT MOST ONCE in `WinningOrigins`, and at most once across all of
> `LosingOrigins`.** Both lists are de-duplicated by origin string, then sorted. Neither list ever
> contains two entries carrying the same `Origin`.

Concretely, per list:

- **`WinningOrigins`** holds each DISTINCT origin that contributed to the winning identity. Two
  definitions that share both an origin and a `definitionIdentity` contribute ONE entry, not two.
- **`LosingOrigins`** holds each DISTINCT origin appearing anywhere among the discarded
  identities, paired with that origin's own tier. One entry per ORIGIN — **never one per discarded
  identity.** So when a single losing origin contributes SEVERAL differing identities, which is
  exactly what AC-6's stronger-tier carve-out produces, that origin still yields ONE
  `LosingOrigins` entry and therefore ONE counter increment (AC-8d).

Because `Tier` is functionally determined by `Origin` through `TierForOrigin`, de-duplicating by
origin can never discard distinct tier information: two entries sharing an `Origin` are always
identical `OriginTier` values.

**The two lists are de-duplicated INDEPENDENTLY, and one origin MAY appear in both.** The rule
above bounds each list on its own; it does not make them disjoint, and THE SYSTEM SHALL NOT
subtract `WinningOrigins` from `LosingOrigins`. Worked case: `executor profile` = X,
`agent profile` = X, `agent profile` = Y. Identity X is contributed by both origins and classifies
Tier 1; identity Y is contributed by `agent profile` alone and classifies Tier 2; X wins. Then
`WinningOrigins` is `[agent profile, executor profile]` and `LosingOrigins` is
`[{agent profile, TierProfileDefault}]` — the same origin on both sides, because it genuinely both
contributed to the winner and had a distinct value discarded. The metric label pair is
`winning_origin=agent profile+executor profile;losing_origin=agent profile`, which is correct
rather than a defect to normalise away.
This is resolver-level only, for the same reason AC-6's ambiguous case is: reaching it needs one
origin contributing two differing identities, and the only origin that can do that is
`executor profile` (AC-6a), which is Tier 1 and therefore never a loser. It is pinned because the
rule is otherwise silent on the interaction and a builder could reasonably "clean up" the
duplicate, changing an observable counter value with no AC to catch it.

Three reasons this is the SET reading and not the bag reading, in order of weight:

1. **It preserves today's behaviour.** `selectDefinitions` already accumulates contributing
   origins in a `map[string]struct{}` (`origins[key][origin] = struct{}{}`) and already emits
   `ConflictError.Origins` de-duplicated and sorted. The bag reading would be a silent behaviour
   CHANGE to merge-origin reporting on a path this feature was not asked to touch.
2. **It is what the counter means.** AC-24 counts "per-ORIGIN, not per input definition". Under the
   bag reading, one key overriding one origin could increment the same
   `winning_origin=…;losing_origin=…` label pair twice, which makes the metric a count of internal
   identity groupings rather than of overrides.
3. **It makes AC-28's ordering total.** With origins unique, `Origin` alone totally orders
   `LosingOrigins`; under the bag reading it does not, and AC-27's deep-equality requirement would
   rest on an undefined order between indistinguishable entries.

**The winning origins may span tiers, and `WinningTier` is the strongest of them.** A merge groups
definitions by `definitionIdentity`, which is the hash of the literal (or the secret ID) and says
nothing about origin, so one identity can be contributed by an `executor profile` (Tier 1) and an
`agent profile` (Tier 2) that happen to carry the same value. `WinningTier` therefore records the
tier the identity was CLASSIFIED into by procedure step 6 — the numerically smallest tier among
`WinningOrigins` — not a property shared by every member. Consumers that need the per-origin
detail read `WinningOrigins`; `WinningTier` answers only "which tier won this key".

Note the remaining asymmetry with `LosingOrigins`, which pairs each origin with its OWN tier via
`OriginTier`. That is deliberate and is not affected by the set rule above: losers are enumerated
per origin because AC-24 fans the counter out per losing origin, whereas the winner is a single
classified identity with a single rank.

### Signatures

```go
// CHANGED: gains a third return value.
func Resolve(ctx context.Context, definitions []Definition, reveal RevealFunc) (map[string]string, []OverrideRecord, error)

// UNCHANGED.
func Validate(definitions []Definition) error
```

`Resolve` gains a return value. That is a deliberate, named change to the function signature; the
exported FIELDS of `Definition`, `ConflictError` and `SecretError` are untouched. There are only
two call sites to update: `Manager.resolveStrictEnvironment`
(`environment_resolution.go:51`, the sole production resolve site) and `resolveEnvironmentSources`
(`task_environment.go:40`, which today is reached only from tests).

**What each call site does with the new value.** `Manager.resolveStrictEnvironment` consumes the
records: it emits the AC-23 log and the AC-24 counter increments. `resolveEnvironmentSources`
DISCARDS them (`resolved, _, err := ...`) and SHALL NOT log or count. That is not laziness about a
test helper — it is AC-46's rule applied consistently: emission is tied to an actual launch
resolution, and this function is reached from no production path (5 test callers, 0 production).
A test helper that incremented the shared expvar map would make the counter a count of test runs.
If a future change gives it a production caller, that caller must take over the emission and this
clause must be revisited.

**On any error, `Resolve` returns NO override records.** When the third return value is non-nil the
second SHALL be `nil` and the first SHALL be `nil`, matching today's `return nil, err` on every
failure path. This is all-or-nothing, not partial (AC-20a).

The case is reachable and is not hypothetical. Records are computed for every key before any
secret is revealed, because step 7 reveals only after the whole key set is conflict-free. So a set
where key `A` is resolved by tier precedence while key `B` conflicts, or where `A` overrides and
any secret then fails to reveal, has already produced a record for `A` at the moment the error is
returned. Handing those records back would let `Manager.resolveStrictEnvironment` log
`environment override applied` and increment `environment_override_applied_total` for a launch
that failed and applied nothing — corrupting the counter AC-24 defines and contradicting AC-46's
"tied to an actual resolution". A launch that does not start has not overridden anything.

`Validate` keeps its `error`-only signature and emits no records, no log, and no counter
increment. This is safe rather than merely convenient: the preflight set is assembled by
`Executor.taskEnvironmentSources` from `managed runtime`, `executor profile` and
`repository <name>` definitions only — every one of them Tier 1 — because agent-profile
definitions are not appended until `resolveStrictEnvironment` runs at the lifecycle site. A
Tier-1-only set can never occupy two tiers, so it can never produce an override, so there is
nothing for `Validate` to return. If a future change adds a Tier 2 or Tier 3 definition to the
preflight set, this AC must be revisited (AC-19b).

## Acceptance criteria

Observable behaviour. Each criterion is pass/fail against the resolver's return value, the emitted
override records, or the resulting environment map.

### Precedence

- **AC-1** WHEN a key has one literal definition from origin `executor profile` and one literal
  definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key
  to the `executor profile` value and SHALL NOT return an error.
- **AC-2** WHEN a key has one literal definition from origin `managed runtime` and one literal
  definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key
  to the `managed runtime` value and SHALL NOT return an error.
- **AC-3** WHEN a key has one literal definition from origin `managed credentials` and one literal
  definition from origin `agent profile` with a different value, THE SYSTEM SHALL resolve that key
  to the `managed credentials` value and SHALL NOT return an error.
- **AC-4** WHEN the resolver receives one literal definition from origin `agent profile` and one
  literal definition from origin `managed agent defaults` for the same key with different values,
  THE SYSTEM SHALL resolve that key to the `agent profile` value and SHALL NOT return an error.
  This is a resolver-level criterion. In the assembled launch path the pair cannot occur, because
  `appendAgentRuntimeDefaults` drops the agent-runtime default before resolution (AC-17). Tier 3
  exists so the resolver's table is total and the assembly-time skip is a redundant optimisation
  rather than the only thing preventing a conflict.
- **AC-4a** WHEN a definition is dropped at assembly time by `appendAgentRuntimeDefaults`, THE
  SYSTEM SHALL NOT emit an override record for that key. Assembly-time filtering is not an
  override; only a resolver tier decision is.
- **AC-5** WHEN a key has literal definitions from two different Tier 1 origins with different
  values, THE SYSTEM SHALL return a `*ConflictError` whose `Origins` lists every participating
  Tier 1 origin sorted ascending, and SHALL NOT resolve the key.
- **AC-5a** WHEN a key has an `executor profile` literal and an `agent profile` literal carrying
  THE SAME value, plus a `managed runtime` literal carrying a DIFFERENT value, THE SYSTEM SHALL
  return a `*ConflictError` naming all three origins sorted ascending, and SHALL NOT resolve the
  key. The merged `executor profile`/`agent profile` identity classifies as Tier 1 via its
  strongest contributing origin (procedure step 6), so the top tier holds two identities and AC-5
  applies. THE SYSTEM SHALL NOT classify that merged identity by the surviving `Definition.Origin`
  field alone, which would rank it Tier 2 and silently discard the `executor profile` value.
  This case is production-reachable and is the reason step 6 says "strongest": `AGENT_MODEL` is
  emitted as `managed runtime` from `profileInfo.Model` (`environment_resolution.go:65`) and is
  not a reserved key (`profile_crud.go:867-868` reserve only `TASK_DESCRIPTION` and `KANDEV_*`),
  so a user who duplicated it onto both profiles — the workaround this feature replaces — hits it.
- **AC-5b** WHEN a key has an `executor profile` literal and an `agent profile` literal carrying
  THE SAME value, plus a differing literal from a Tier 3 origin, THE SYSTEM SHALL resolve the key
  to the merged value and emit ONE `OverrideRecord` whose `WinningOrigins` holds BOTH
  `agent profile` and `executor profile` sorted ascending and whose `WinningTier` is
  `TierAuthoritative`. This pins that `WinningOrigins` may span tiers while `WinningTier` records
  only the strongest. This is a resolver-level criterion; in the assembled launch path
  `appendAgentRuntimeDefaults` drops the Tier 3 definition first (AC-17).
- **AC-6** WHEN a key has two definitions carrying the same `Origin` string and different
  `definitionIdentity` values, AND no definition for that key sits in a STRONGER tier than that
  origin, THE SYSTEM SHALL return a `*ConflictError`. Where a stronger tier does hold exactly one
  surviving identity, procedure step 6 governs instead: the stronger tier wins and the ambiguous
  weaker-tier origins are discarded into `LosingOrigins`, because a set of definitions that is
  being thrown away wholesale need not be self-consistent first. That discarded origin appears in
  `LosingOrigins` EXACTLY ONCE even though it contributed two identities, because both origin
  lists are de-duplicated sets ("The override record"); AC-8d observes this case directly.
  Same-origin ambiguity WITHIN the winning tier still conflicts, which is the case that matters:
  two differing `executor profile` definitions, or two differing `managed runtime` definitions,
  block the launch exactly as today (AC-5).
  REACHABILITY IS PER-ORIGIN, AND ONE ORIGIN REACHES THIS IN PRODUCTION. An earlier draft claimed
  the assembled launch path "cannot produce two same-origin definitions that disagree, because
  duplicate keys within one profile are rejected at save time (`profile_crud.go:908`)". That is
  true for `agent profile` and false for `executor profile`: the duplicate-key check is
  agent-profile-only (see "The two profile kinds are NOT validated the same way"), so one executor
  profile may legitimately hold two entries with the same key and different values. Per origin:
  - `agent profile` — NOT reachable; duplicate keys rejected at save time (`profile_crud.go:908`).
  - `managed agent defaults` — NOT reachable; built from a map.
  - `managed runtime` — reachable in principle, since `req.Env` and `appendStandardDefinitions`
    both emit this origin, though a collision there is a managed-value bug rather than user input.
  - `executor profile` — REACHABLE from ordinary user input, with no validation preventing it.
  The behaviour is nonetheless unchanged from today: every one of those origins is Tier 1, so with
  no stronger tier present the key returns `*ConflictError` exactly as it does now. AC-6a requires
  the test for the reachable case.
- **AC-6a** WHEN one executor profile holds TWO entries with the same key and different non-empty
  values, and no definition for that key sits in a stronger tier, THE SYSTEM SHALL return a
  `*ConflictError` naming `executor profile`, and SHALL NOT resolve the key by picking either
  value.
  This is the ASSEMBLED-PATH case AC-6 says is reachable, and it SHALL be tested through
  `Executor.taskEnvironmentSources` rather than only against the resolver, because the claim being
  pinned is that a real executor profile can produce the pair. `executor profile` is Tier 1, so
  this is AC-5's same-tier behaviour and is unchanged from today; the AC exists so the reachable
  case has a test rather than resting on a false save-time guarantee.
- **AC-7** WHEN a key is defined only by origin `agent profile` and no executor profile is
  selected for the session, THE SYSTEM SHALL resolve that key to the `agent profile` value.
- **AC-8** WHEN a key is defined only by origin `agent profile` and the selected executor profile
  declares no environment variables, THE SYSTEM SHALL resolve that key to the `agent profile`
  value.
- **AC-8a** WHEN a key has two Tier 1 definitions that share one `definitionIdentity` and one
  differing literal Tier 2 definition, THE SYSTEM SHALL merge the Tier 1 pair first, then apply
  tier precedence, resolving the key to the merged Tier 1 value and emitting a single
  `OverrideRecord` whose `WinningOrigins` holds BOTH contributing Tier 1 origins sorted ascending,
  whose `WinningTier` is `TierAuthoritative`, and whose `LosingOrigins` holds the one Tier 2
  origin. Merging precedes tier comparison (procedure step 3 before step 6). This is the case that
  makes `WinningOrigins` a slice rather than a string.
- **AC-8b** WHEN a key has two differing literal Tier 1 definitions and one differing literal
  Tier 2 definition, THE SYSTEM SHALL return a `*ConflictError` whose `Origins` names all three
  origins, sorted ascending. An ambiguous top tier is not resolved by falling through to a lower
  tier.
- **AC-8c** WHEN a key has one literal Tier 1 definition and differing literal definitions in both
  Tier 2 and Tier 3, THE SYSTEM SHALL resolve the key to the Tier 1 value and emit ONE
  `OverrideRecord` whose `LosingOrigins` holds both the Tier 2 and the Tier 3 origin, each paired
  with its own tier, sorted by `Origin` ascending. Per AC-24 this one record produces TWO counter
  increments, one per losing origin.
- **AC-8d** WHEN a key has ONE literal Tier 1 definition and TWO literal definitions that carry
  THE SAME lower-tier `Origin` string as each other but DIFFERENT `definitionIdentity` values,
  THE SYSTEM SHALL resolve the key to the Tier 1 value and emit ONE `OverrideRecord` whose
  `LosingOrigins` has `len(LosingOrigins) == 1` — a single entry for that origin paired with its
  own tier — and which produces EXACTLY ONE counter increment.
  This is the losing-side de-duplication pin, and it is the case AC-6's stronger-tier carve-out
  creates: one origin contributing two discarded identities is ONE loser, not two. A test asserting
  `len(LosingOrigins) == 2` or two increments is asserting the rejected bag reading.
  This one IS genuinely resolver-level, and unlike AC-6 the reason survives the per-origin check in
  AC-6a. The ambiguous origin here is a LOSING origin, so it necessarily sits in Tier 2 or Tier 3 —
  a Tier 1 origin can never lose, and an ambiguity inside Tier 1 conflicts instead (AC-5, AC-6a).
  The only Tier 2 origin is `agent profile`, whose duplicate keys ARE rejected at save time
  (`profile_crud.go:908`), and the only Tier 3 origin is `managed agent defaults`, which is built
  from a map. `executor profile`, the one origin that can produce disagreeing duplicates, is Tier 1
  and therefore cannot appear in `LosingOrigins` at all.
- **AC-8e** WHEN a key has ONE Tier 1 value contributed by TWO definitions that share BOTH an
  `Origin` string and a `definitionIdentity`, plus a differing literal definition in a lower tier,
  THE SYSTEM SHALL emit ONE `OverrideRecord` whose `WinningOrigins` has
  `len(WinningOrigins) == 1` — that origin listed once, not twice.
  This is the winning-side de-duplication pin. It matters beyond symmetry: today's
  `selectDefinitions` already accumulates contributing origins into a `map[string]struct{}`, so the
  bag reading would be a silent behaviour change to existing merge-origin reporting. Contrast with
  AC-8a, where two DISTINCT origins contribute one identity and both are listed.

### Secret veto

- **AC-9** IF a key has definitions with different `definitionIdentity` values and at least one of
  them has a non-empty `SecretID`, THEN THE SYSTEM SHALL return a `*ConflictError` and SHALL NOT
  apply tier precedence, even when the definitions sit in different tiers.
- **AC-10** WHEN a key has a secret-backed `executor profile` definition and a literal
  `agent profile` definition, THE SYSTEM SHALL return a `*ConflictError`.
- **AC-11** WHEN a key has a literal `executor profile` definition and a secret-backed
  `agent profile` definition, THE SYSTEM SHALL return a `*ConflictError`.
- **AC-12** WHEN a key has a secret-backed `repository <name>` definition and any differing
  definition from any other origin, THE SYSTEM SHALL return a `*ConflictError`, preserving
  ADR-2026-08-03.
- **AC-12a** WHEN a key has a secret-backed `repository app` definition and a secret-backed
  `executor profile` definition that carry THE SAME `SecretID`, plus a differing literal
  `agent profile` definition, THE SYSTEM SHALL return a `*ConflictError` whose `Origins` names ALL
  THREE origins — `agent profile`, `executor profile`, `repository app` — de-duplicated and sorted
  ascending, and SHALL NOT resolve the key.
  The two secret-backed definitions share the identity `"secret:<id>"` and MERGE at procedure
  step 3, so a single surviving `Definition` carries one `Origin` field between them. THE SYSTEM
  SHALL NOT enumerate `Definition.Origin` per surviving identity, which would report only two
  origins and hide from the user one of the two places the secret is bound.
  This is the veto-path counterpart of AC-5a, and it is the MORE production-reachable of the two:
  ADR-2026-08-03 makes every repository binding secret-only, so real secret disagreements arrive
  at step 5 rather than step 6. It preserves today's behaviour — `selectDefinitions` already folds
  the merged identity's accumulated origin set into the conflict set — and
  `TestResolveEnvironmentSources_ReportsEveryConflictingOrigin` already asserts against this field.
- **AC-13** WHEN a `*ConflictError` is returned, its message and `Origins` SHALL contain no secret
  value, no `SecretID`, and no literal value.

### No regression for currently-succeeding tasks

- **AC-14** WHEN a key has exactly one definition, THE SYSTEM SHALL resolve it to that
  definition's value, unchanged from today.
- **AC-15** WHEN a key has multiple definitions that all share one `definitionIdentity`, THE
  SYSTEM SHALL merge them, resolve the key to that shared value, record every origin as it does
  today, and SHALL NOT emit an override record. Nothing was overridden.
- **AC-16** WHEN a definition set produced no error before this change AND its result was
  DETERMINISTIC under the current three-column sort — that is, its outcome did not depend on input
  slice order — THE SYSTEM SHALL produce the same environment map after this change.
  The determinism qualifier is not a hedge; it is required, and AC-29a states the same bound for
  the sort. Two changes land together, and only one of them is precedence:
  - PRECEDENCE re-routes only sets that previously returned `*ConflictError`, so it cannot alter
    any set that already succeeded.
  - THE FIVE-COLUMN SORT (AC-29) additionally settles ties the three-column sort left to input
    order. One such tie already succeeds today: a same-`Key`/`Origin`/`SecretID` pair differing
    only in `WorkspaceID` merges (identity is `"secret:" + SecretID`), and whichever definition
    happens to come first supplies the `WorkspaceID` that `resolveEnvironmentDefinition` branches
    on to choose `RevealForWorkspace` over a global reveal (`environment_resolution.go:188-195`).
    Its revealed VALUE is therefore input-order-dependent today, and AC-29b now pins it. For such
    a set the post-change map may differ from one of its pre-change orderings, deliberately.
  A differential test against the current `selectDefinitions` as oracle MUST therefore restrict
  its generator to conflict-free sets that are order-INDEPENDENT today, or assert per-ordering
  agreement only for sets with no `WorkspaceID` tie. Generating the tie class and demanding
  equality would be testing a promise this AC does not make.
- **AC-17** WHEN `appendAgentRuntimeDefaults` runs, it SHALL continue to skip any key already
  defined by an earlier definition, preserving
  `TestAppendAgentRuntimeDefaultsFillsOnlyMissingKeys`.

### Preflight consistency

- **AC-18** THE shape-only preflight at `task_environment.go:118` SHALL apply the same tier and
  secret-veto rules as the final resolve, so it never rejects a definition set that the final
  resolve would accept.
- **AC-19** WHILE the preflight definition set contains no `agent profile` definitions, its
  accept/reject verdict SHALL be identical to today's for every input.
- **AC-19a** BOTH assembly sites SHALL derive their origin strings from one shared set of exported
  constants declared in the `environment` package, and the tier table SHALL be defined once, in
  that same package. The orchestrator site currently hardcodes `"managed runtime"` and
  `"executor profile"` (`task_environment.go:63`, `:67`) while the lifecycle site uses unexported
  constants (`environment_resolution.go:17-21`); a rule keyed on origin cannot depend on two
  independent spellings staying in sync.
- **AC-19c** THE `environment` package SHALL export exactly two classification functions,
  `TierForOrigin(origin string) Tier` and `IsRepositoryOrigin(origin string) bool`, and SHALL NOT
  export the tier table itself. `TierForOrigin` SHALL be the only reader of that table.
  `IsRepositoryOrigin` SHALL be the only implementation of the normative repository-origin rule
  ("What counts as a repository origin"), and BOTH `TierForOrigin` and `NormalizeOriginLabel`
  (AC-25) SHALL call it rather than re-deriving it — one spelling, per AC-19a.
  A test SHALL assert `IsRepositoryOrigin` against the worked boundary table in that section:
  true for `repository`, `repository app`, `repository  app` (two spaces) and `repository\tapp`;
  false for `repositoryapp` and the empty string. A test SHALL also assert that
  `TierForOrigin` returns `TierAuthoritative` for an origin the table does not name (AC-35).
  Keeping the table unexported is observable at compile time and is the point: an exported map
  could be mutated by any importer, which would defeat the single-source-of-truth guarantee AC-19a
  asks for.
- **AC-19b** `Validate` SHALL keep the signature `func Validate([]Definition) error` and SHALL
  emit no override record, no override log, and no counter increment. A test SHALL assert that a
  definition set which WOULD produce an override when passed to `Resolve` produces no observable
  override side effect when passed to `Validate`.

### Observability

- **AC-20** WHEN tier precedence resolves a key that would previously have produced a
  `*ConflictError`, THE SYSTEM SHALL produce exactly one `OverrideRecord` for that key, populated
  as declared in "The override record": `Key`, `WinningOrigins` (every DISTINCT origin that
  contributed to the merged winning identity, each listed ONCE, sorted ascending, at least one),
  `WinningTier`, and `LosingOrigins` (every DISTINCT origin appearing among the discarded
  identities, each listed ONCE with its tier, sorted by `Origin` ascending, at least one).
  BOTH lists are DE-DUPLICATED SETS: no origin string appears twice in either. One entry per
  ORIGIN, never one per contributing or discarded identity. AC-8d and AC-8e observe the two
  de-duplication cases directly.
- **AC-20a** WHEN `Resolve` returns a non-nil error, it SHALL return a nil map and a nil
  `[]OverrideRecord`, even if tier precedence had already resolved other keys before the failing
  key was reached, AND the caller SHALL emit no override log record and SHALL NOT increment the
  counter for that resolve. A test SHALL cover both error paths: (a) one key resolved by tier
  precedence plus a second key that conflicts, and (b) one key resolved by tier precedence plus a
  secret that fails to reveal, which is the more reachable case because step 7 reveals only after
  every key is conflict-free. Nothing was applied, so nothing is reported.
- **AC-21** An `OverrideRecord` SHALL NOT contain any literal value, any `SecretID`, or any
  decrypted secret. The type has no field capable of carrying one.
- **AC-22** THE resolver SHALL return override records to its caller rather than logging them
  itself. The `environment` package holds no logger and must stay a pure function of its inputs.
  Records reach the caller as `Resolve`'s new third return value.
- **AC-23** WHEN a launch applies one or more overrides, `Manager.resolveStrictEnvironment` SHALL
  emit one structured log record at Info level per `OverrideRecord`, with the message
  `environment override applied` and these fields:

  | Field | Type | Source |
  | --- | --- | --- |
  | `env_key` | string | `OverrideRecord.Key` |
  | `winning_origins` | []string | `WinningOrigins`, in record order |
  | `winning_tier` | int | `WinningTier` |
  | `losing_origins` | []string | `LosingOrigins[].Origin`, in record order |
  | `losing_tiers` | []int | `LosingOrigins[].Tier`, index-aligned with `losing_origins` |
  | `task_id` | string | `LaunchRequest.TaskID` |
  | `session_id` | string | `LaunchRequest.SessionID` |

  Origins are logged UNNORMALISED, so a reader sees which repository was involved. Normalisation
  (AC-25) applies to the metric label only, where cardinality matters.
- **AC-24** THE SYSTEM SHALL increment an expvar counter map named
  `environment_override_applied_total` once per entry in `LosingOrigins`, so the number of
  increments for one record equals `len(LosingOrigins)` exactly. Counting is POST-MERGE and
  strictly per-ORIGIN, never per input definition and never per discarded identity: ALL discarded
  definitions sharing one origin collapse into a single `LosingOrigins` entry and are counted
  ONCE, **whether or not they share a `definitionIdentity`**. Two differing `agent profile`
  literals discarded together are ONE increment, not two (AC-8d).
  The earlier wording conditioned that collapse on sharing "one origin AND one
  `definitionIdentity`", which read as one entry per identity and contradicted this AC's own
  per-ORIGIN rule; the condition is the ORIGIN alone, because `LosingOrigins` is a de-duplicated
  set (AC-20, "The override record"). One key overriding one origin increments its label pair
  exactly once, so the counter measures overrides rather than internal identity groupings.
  The record and the log record remain one per key (AC-20, AC-23); only the counter fans out per
  losing origin, because one label pair cannot represent several losers.
- **AC-24a** THE counter SHALL live in a new neutral package
  `apps/backend/internal/agent/runtime/envmetrics`, following the existing convention in
  `apps/backend/internal/workflow/signalmetrics`: an `expvar.NewMap` at package scope, incremented
  through an exported `RecordOverrideApplied(winningOrigin, losingOrigin string)` helper, with the
  label key built as `k1=v1;k2=v2` so a downstream Prometheus translation splits on the same
  delimiters. The label key names SHALL be exactly `winning_origin` and `losing_origin`, giving
  map keys of the form `winning_origin=executor profile;losing_origin=agent profile`.
  `RecordOverrideApplied` SHALL receive values that are ALREADY NORMALISED per AC-25 and SHALL
  perform no normalisation of its own. `envmetrics` stays a neutral recorder with no dependency on
  `environment`, exactly as `signalmetrics` is neutral today.
- **AC-24b** THE counter SHALL be incremented by the lifecycle caller, in the same place as the
  AC-23 log, and SHALL NOT be incremented inside `selectDefinitions`, `Resolve`, or `Validate`.
  The `environment` package must not import `envmetrics`; keeping the resolver pure is what makes
  AC-27's repeatability testable.
- **AC-25** THE normalised origin used as a metric label SHALL be derived as follows, and this
  normalisation applies to BOTH label values:
  - an origin for which `IsRepositoryOrigin` reports true collapses to the single value
    `repository`, so repository names cannot make metric cardinality unbounded;
  - every other origin, INCLUDING an origin the tier table does not recognise, is used verbatim,
    with no case folding and no whitespace substitution. Origins are produced by code constants,
    so the value set is bounded without further processing.
  - WHEN `WinningOrigins` holds more than one origin, the `winning_origin` label value SHALL be
    each origin normalised, de-duplicated, sorted ascending, and joined with `+` (for example
    `executor profile+managed runtime`).
    The join's de-duplication is a POST-NORMALISATION step and is separate from the set rule on
    `WinningOrigins` itself: AC-20 already guarantees no origin appears twice in the field, but two
    DISTINCT repository origins (`repository app`, `repository web`) both normalise to
    `repository`, so the collapse can still produce a duplicate that the join must remove. That
    specific pair is resolver-level only, exactly as AC-4 and AC-6 are: repository definitions are
    secret-only by ADR construction, and the veto (AC-9) bars any secret-backed definition from
    reaching `WinningOrigins` beside a differing definition. The join is de-duplicated anyway so
    the label rule is total and cannot depend on that reachability argument staying true.

  THE normalisation SHALL be owned by the `environment` package and exposed as two exported
  functions beside the tier table: `NormalizeOriginLabel(origin string) string` for a single
  origin, and `JoinOriginLabels(origins []string) string` for the collapse-dedupe-sort-join over
  `WinningOrigins`. `NormalizeOriginLabel` SHALL make its repository decision by calling
  `IsRepositoryOrigin` (AC-19c), never by re-deriving the rule. The lifecycle caller calls these
  two and passes the results to `RecordOverrideApplied`; because AC-24 fans the counter out per
  losing origin, the `losing_origin` value is always a single origin and `NormalizeOriginLabel`
  alone produces it. `environment` owns this because the repository rule is the same knowledge the
  tier table already consults (AC-19a): two independent spellings of "what counts as a repository
  origin" is precisely the drift AC-19a exists to prevent, which is why exactly one predicate
  exists and both callers share it.
- **AC-26** WHEN no override is applied during a launch, THE SYSTEM SHALL emit no override log
  record and SHALL NOT increment the counter.
  THE RETURN CONVENTION IS PINNED: when no override is applied, `Resolve` SHALL return `nil` for
  `[]OverrideRecord`, never a non-nil empty slice. `nil` and `[]OverrideRecord{}` are not
  `reflect.DeepEqual`, so leaving the choice open would make the AC-27 and AC-30 determinism
  tests fail or pass on an irrelevant detail. Callers SHALL nonetheless treat an empty slice and
  `nil` identically — `len(records) == 0` is the correct caller-side test, so a future change to
  the convention cannot silently start emitting logs.

### Determinism and ordering

- **AC-27** WHEN the same definition set is resolved twice, THE SYSTEM SHALL return an identical
  environment map and a deeply identical override-record sequence. "Deeply identical" includes the
  contents and order of `WinningOrigins` and `LosingOrigins` inside every record, not just the
  sequence of records — a test comparing records SHALL compare their nested slices element by
  element.
- **AC-28** THE SYSTEM SHALL order override records by `OverrideRecord.Key` ascending. Because
  AC-20 emits exactly one record per key, `Key` is unique across the sequence and is therefore a
  total order on its own; no secondary record-level sort key exists or is needed. Ordering WITHIN a
  record is specified separately and is what AC-27 depends on:
  - `WinningOrigins` SHALL be sorted by origin string ascending;
  - `LosingOrigins` SHALL be sorted by `Origin` ascending.

  `Origin` alone TOTALLY orders both lists, and it does so only because AC-20 makes them
  de-duplicated sets: with each origin appearing at most once, no two entries can compare equal,
  so no tiebreak is needed or possible. This sentence depends on the set rule and would be FALSE
  without it — under a bag reading, two discarded identities sharing an origin would produce two
  entries that `Origin` cannot order, and AC-27's element-by-element comparison would rest on an
  undefined order between them. `Tier` is additionally functionally determined by `Origin` through
  `TierForOrigin`, so a tiebreak on `Tier` would carry no information even if entries could
  collide.
- **AC-29** THE SYSTEM SHALL sort definitions by the NAMED columns `Key`, then `Origin`, then
  `SecretID`, then `WorkspaceID`, then `Literal`, all ascending, before walking them. This is a
  total order over every `Definition` field that can affect the outcome, which the current
  three-column sort at `environment.go:97` is not. WHEN several definitions for one key share a
  `definitionIdentity`, THE SYSTEM SHALL select the first under that order, and input slice order
  SHALL NOT affect which one survives.
- **AC-29a** WHEN a definition set produced a deterministic result under the current three-column
  sort, THE SYSTEM SHALL produce that same result under the five-column sort. Adding sort columns
  can only break ties previously left to input order; it cannot reorder definitions that already
  differed on an earlier column.
- **AC-29b** WHEN two definitions share `Key`, `Origin` and a non-empty `SecretID` but differ in
  `WorkspaceID`, THE SYSTEM SHALL select the one with the lexicographically smaller `WorkspaceID`,
  deterministically. This case merges rather than conflicts (both have identity
  `"secret:" + SecretID`), and the surviving `WorkspaceID` decides whether
  `resolveEnvironmentDefinition` reveals the secret globally or through `RevealForWorkspace`, so
  leaving it to input order would make the revealed VALUE order-dependent.
- **AC-30** WHEN definitions are supplied in a different input order, THE SYSTEM SHALL produce the
  same result: the same winner, the same revealed value, the same error, and the same override
  records including their nested slices.

### Nil, empty, and error behaviour

- **AC-31** WHEN the definition list is empty or nil, THE SYSTEM SHALL return an empty (non-nil)
  map, a nil error, and a nil `[]OverrideRecord` per AC-26's pinned convention. The empty map
  matches today's behaviour, which allocates before the loop and returns it.
- **AC-32** WHEN a definition's `Key` is empty or whitespace-only, THE SYSTEM SHALL skip it,
  unchanged from today (`environment.go:111`).
- **AC-33** WHEN a secret-backed definition is selected and the reveal callback is nil, THE SYSTEM
  SHALL return a `*SecretError` naming the key and origin, unchanged from today.
- **AC-34** WHEN a secret-backed definition is selected and the reveal callback returns an error,
  THE SYSTEM SHALL return a `*SecretError` naming the key and origin, unchanged from today.
- **AC-35** WHEN a definition carries an origin string that this spec's tier table does not name,
  `TierForOrigin` SHALL return `TierAuthoritative` and THE SYSTEM SHALL apply peer-conflict
  behaviour to it. This covers the empty origin string and any string that is not a repository
  origin under the normative rule (for example `repositoryapp`), both of which fall through to
  the unrecognised default rather than into the repository row.
- **AC-36** IF more than one key conflicts, THEN THE SYSTEM SHALL report the lexicographically
  first conflicting key, unchanged from today (`environment.go:138`).
- **AC-37** WHEN precedence selects a winner, THE SYSTEM SHALL reveal secrets only for the
  surviving definition. A discarded definition SHALL NOT be revealed. Because AC-9 blocks every
  mixed-secret case, a discarded definition is always literal in practice; this criterion pins
  that no reveal is attempted for a loser.

### Boundary values and defaults

- **AC-38** WHERE a profile entry has an empty `Value` and an empty `SecretID`, no definition for
  that key SHALL reach the resolver from that profile, and its absence SHALL NOT count as an
  override. This SHALL hold on ALL THREE assembly paths. Two already drop the entry and are
  unchanged; the third does not drop it today and SHALL be changed to:
  - the LIFECYCLE agent-profile path drops the entry explicitly (`appendProfileDefinitions`,
    `environment_resolution.go:134`) — UNCHANGED;
  - the LIFECYCLE executor-profile path drops it explicitly
    (`executorProfileEnvironmentDefinitions`, the `value.SecretID == "" && value.Value == ""`
    skip, `environment_resolution.go:257-259`) — UNCHANGED;
  - the PRIMARY ORCHESTRATOR path (`Executor.taskEnvironmentSources`,
    `task_environment.go:66-70`) appends every `ProfileEnvVar` unconditionally today and **SHALL
    be changed to skip an entry whose `SecretID` and `Value` are both empty**, using the same
    condition as the lifecycle executor-profile path.

  **This reverses an instruction earlier drafts of this spec gave, and the reason is recorded so
  it is not reverted again.** Those drafts asserted the orchestrator path "cannot receive such an
  entry, because `validateEnvVarValue` (`profile_crud.go`) rejects it at profile save time", and
  told the builder not to test for a drop there. That justification is FALSE: `validateEnvVarValue`
  is agent-profile-only, the vars on this path are EXECUTOR-profile vars, and the executor-profile
  save path enforces no value-or-secret rule at all (see "The two profile kinds are NOT validated
  the same way"). A blank executor-profile value is therefore savable from the UI, HTTP, WS and
  agent-driven MCP, and under tier precedence it would classify Tier 1, beat a real Tier 2
  agent-profile value, and launch the agent with the key set to the empty string.
  DO write a test asserting the drop at the orchestrator assembly step (AC-38b). The save-time
  rejection test remains valid, but ONLY as a statement about agent profiles.
- **AC-38a** An override to the EMPTY STRING is therefore not expressible from a profile, on any
  path. Lifting that limitation is out of scope.
  The guarantee now rests on the ASSEMBLY-TIME SKIP being present on all three paths (AC-38), not
  on save-time validation. That distinction is the whole of F25: save-time validation covers only
  agent profiles, so an invariant resting on it was false for executor profiles.
- **AC-38b** WHEN `Executor.taskEnvironmentSources` receives a `ProfileEnvVar` whose `Value` and
  `SecretID` are both empty, THE SYSTEM SHALL NOT emit an `environmentSource` for it, and the key
  SHALL NOT appear in `req.EnvironmentDefinitions`.
  A test SHALL assert this directly at the orchestrator assembly step. A test SHALL also assert
  that the same profile produces the same definition set through BOTH assembly paths — the
  orchestrator path and `executorProfileEnvironmentDefinitions` — for an input containing one
  populated entry and one blank entry, because eliminating the divergence between those two paths
  is the point of the change and a one-sided fix would satisfy the first test alone.
  THE SKIP SHALL match the lifecycle condition exactly: `SecretID == "" && Value == ""`. An entry
  with a blank `Value` but a non-empty `SecretID` is secret-backed and SHALL still be emitted; an
  entry with a blank `SecretID` but a non-empty `Value` is a literal and SHALL still be emitted.
- **AC-38c** WHEN an executor profile carries a key whose `Value` and `SecretID` are both empty and
  an agent profile defines the same key with a non-empty literal, THE SYSTEM SHALL resolve that key
  to the AGENT-PROFILE value and SHALL NOT emit an override record, because after AC-38b only one
  definition for that key reaches the resolver.
  This is a deliberate BEHAVIOUR CHANGE and is the user-visible consequence of the AC-38 decision.
  Today the same pair returns a `*ConflictError` and blocks the launch. It is recorded as an AC
  rather than left implicit because a reader comparing before-and-after will otherwise read it as
  a regression in AC-49's "no behaviour change for currently-succeeding tasks" — that AC is about
  tasks that SUCCEED today, and this pair fails today.
  Related: WHEN an executor profile carries such an entry and NO other origin defines that key, the
  key SHALL be absent from the resolved environment. Today it resolves to the empty string on the
  primary path and is absent on the recovery path; after this change both paths agree that it is
  absent.
- **AC-39** WHEN a key is defined only by origin `executor profile` with a non-empty `Value` or a
  non-empty `SecretID`, THE SYSTEM SHALL resolve it to that value and SHALL NOT emit an override
  record, because nothing was overridden. The qualifier is required by AC-38b: an entry blank on
  both fields no longer reaches the resolver at all, so there is no value to resolve it to.
- **AC-40** THE **AGENT**-PROFILE save path SHALL continue to reject `TASK_DESCRIPTION` and any
  `KANDEV_*` key (`profile_crud.go:904`), unchanged by this spec. Precedence SHALL NOT create a
  new path to define a reserved key from a profile.
  THE SCOPE OF THAT FIRST CLAUSE IS THE AGENT PROFILE, AND SAYING SO IS THE POINT. The
  executor-profile save path rejects no reserved key (see "The two profile kinds are NOT validated
  the same way"), so a `KANDEV_FOO` executor-profile entry is savable today and reaches the agent
  today whenever no managed-runtime definition claims that key. An earlier draft stated this AC
  without the scope, which would send a builder to write a failing test against executor-profile
  save and then invent either new validation or a silently narrowed test.
  THE SECOND CLAUSE HOLDS AND IS OBSERVABLE, which is why this AC survives rather than being
  deleted: precedence creates no NEW path. A reserved key that a managed-runtime definition also
  sets puts two Tier 1 definitions against each other, so it conflicts exactly as it does today
  (AC-5); a reserved-prefixed key that nothing else sets resolved before this change and resolves
  after it. A test SHALL assert that an `executor profile` definition for a key also set by
  `managed runtime` returns `*ConflictError` and is NOT resolved by tier precedence.
  Closing the executor-profile validation gap itself is OUT OF SCOPE — see "Out of scope".

## Concurrency

Resolution is a pure function of its definition set. There is no shared row, no lock, and no
read-modify-write.

- **AC-41** WHEN two launches for different sessions resolve concurrently, each SHALL produce a
  result determined solely by its own definition set.
- **AC-42** WHEN a profile is edited after a launch has captured that profile's definitions, the
  in-flight launch SHALL keep what it captured and SHALL NOT re-read that profile. This inherits
  ADR-2026-08-03: "An already-running process or open terminal does not change when a binding or
  secret value changes." A later launch, resume, or Reset Environment re-resolves.

  **ONE LAUNCH HAS TWO CAPTURE MOMENTS, NOT ONE, AND THEY ARE NAMED HERE** because a test written
  against a single "the snapshot" fails against the actual code:

  | Definitions | Captured at | Effect of an edit after that moment |
  | --- | --- | --- |
  | `executor profile`, `repository <name>`, `managed runtime` | ORCHESTRATOR assembly — `resolveLaunchEnvironment` → `taskEnvironmentSources`, writing `req.EnvironmentDefinitions` (`executor_execute.go:1100`, `executor_interaction.go:747`) | not seen by this launch |
  | `agent profile` (and `AGENT_MODEL` / `AGENTCTL_AUTO_APPROVE_PERMISSIONS` derived from it) | LIFECYCLE — `resolveAgentProfile` (`manager_launch.go:1140`, `:1197`), appended in `resolveStrictEnvironment` (`environment_resolution.go:40`) | SEEN by this launch, because the read has not happened yet |

  So an EXECUTOR-profile edit between the two moments is not picked up, and an AGENT-profile edit
  between the two moments IS. THE SYSTEM SHALL preserve exactly that. This is today's behaviour and
  this spec does not change it; it is written down because the asymmetry is invisible from either
  call site alone and a reasonable reader assumes one snapshot covers both.
  A test SHALL assert the executor-profile half: a launch whose `req.EnvironmentDefinitions` were
  assembled from executor profile state E resolves against E even when the stored profile has since
  changed to E'.
- **AC-43** WHEN the orchestrator preflight ACCEPTS a definition set, that verdict SHALL NOT be
  treated as a guarantee that the launch resolves: the lifecycle resolve runs over a STRICTLY
  LARGER definition set and its verdict is the one that decides the launch.
  Observable outcome, and the whole content of "authoritative": THE SYSTEM SHALL fail the launch
  when `Validate` returns nil at `task_environment.go:118` and `Resolve` subsequently returns an
  error at `environment_resolution.go:51`. A test SHALL construct exactly that pair — a preflight
  set of Tier 1 definitions that validates cleanly, plus an `agent profile` definition appended at
  the lifecycle site that makes the key conflict — and assert the launch fails with the resolver's
  error, not the preflight's success.
  **THE CONVERSE SHALL NOT HOLD, and this is the clause that closes the ambiguity:** "authoritative"
  describes WHICH VERDICT BINDS, not a re-read of the data. The lifecycle resolve SHALL NOT re-read
  the executor profile, the repositories, or `req.Env`; it consumes `req.EnvironmentDefinitions` as
  captured (AC-42) and appends to them. An implementation that makes the lifecycle site re-load the
  executor profile satisfies a plain reading of the word "authoritative" and violates AC-42, and
  because no precedence criterion observes the difference, every other test in this spec would
  still pass. AC-19 remains true alongside this: the preflight never REJECTS a set the final
  resolve would accept, so the two verdicts can only diverge in the accept-then-fail direction.

## Idempotency and retry

- **AC-44** WHEN a launch fails after environment resolution and is retried with the same profile
  and repository state, resolution SHALL produce the same map and the same override records.
- **AC-45** WHEN a session is resumed, THE SYSTEM SHALL re-resolve from current definitions and
  SHALL apply the same rules as an initial launch. Resume SHALL NOT inherit a stale precedence
  decision.
- **AC-46** Emitting an override log record and incrementing the counter SHALL be tied to an
  actual resolution. A retried launch that resolves again SHALL emit again; these are per-resolve
  events, not deduplicated across retries.

## Migration

- **AC-47** THE SYSTEM SHALL NOT move, copy, or rewrite any environment variable between an agent
  profile and an executor profile.
- **AC-48** THE SYSTEM SHALL NOT require a database migration for this feature.
- **AC-49** WHEN an existing install already has the same key on both an agent profile and an
  executor profile with DIFFERING LITERAL-BACKED values, that install SHALL begin succeeding with
  the executor-profile value on the next strict-resolution launch, with an override record emitted
  per AC-20. The qualifier is load-bearing in four directions, and a migration test written
  without it contradicts other ACs:
  - if EITHER side is secret-backed, the veto still blocks the launch (AC-9, AC-10, AC-11). Such
    an install does NOT begin succeeding, by design;
  - if the two values are IDENTICAL, the install already succeeds today — they merge, and no
    override record is emitted (AC-15). Nothing changes for it;
  - if the EXECUTOR-profile side is blank on both `Value` and `SecretID`, no executor-profile
    definition reaches the resolver at all (AC-38b), so the install begins succeeding with the
    AGENT-profile value and NO override record (AC-38c). The empty string is a differing
    literal-backed value on a plain reading, so without this bullet AC-49 would demand the
    executor's empty value win and contradict AC-38a outright;
  - "strict-resolution launch" excludes the legacy fill-missing path scoped out under
    "The two assembly sites", which never receives executor-profile definitions at all.

Rationale for no automatic move: there is no safe general rule for classifying a key as
machine-scoped. `ANTHROPIC_BASE_URL` is obvious to a human and not to a matcher. Moving a value
off an agent profile would also change what every other task using that profile sees. And an
executor profile is frequently absent: in the observed install, 9 worktree sessions and 4 sessions
carried no `executor_profile_id`, and `Local Docker` had no executor profiles at all. The agent
profile must remain the fallback (AC-7, AC-8), so relocating values there would break more than it
fixes.

## Out of scope

Named exclusions. Each is a contract, not an omission.

- **The agent-profile matcher collision.** `buildAgentProfileMatcher` is a separate card and is
  not changed here.
- **The legacy fill-missing branch of `buildEnvForExecution`** (`manager_startup.go:314-339`),
  taken when neither `EnvironmentFinalized` nor `EnvironmentResolutionRequired` is set. It never
  reaches the resolver and never receives executor-profile definitions, so the conflict this
  feature resolves cannot arise in it. Full reasoning under "The two assembly sites".
- **A per-task or per-session environment override tier.** ADR-2026-08-03 rejected task-stored
  bindings for v1; this spec does not reopen that.
- **Any change to `mergeEnvFillMissing`** (`profile_env.go:57`) or to the process-environment
  merge it performs at agent launch.
- **Any change to which secrets a profile may reference.** Agent and executor profiles remain
  Global-secrets-only per ADR-2026-08-03.
- **Making an override expressible as the empty string.** AC-38 states the limitation; lifting it
  would require changing the assembly-time drop, which is out of scope.
- **A UI affordance for choosing a winner per key.** Precedence is by origin, not by user
  selection.
- **Surfacing override records in the web UI.** AC-20 through AC-26 require the record, the log,
  and the counter. Rendering them in a settings or task panel is deliberately deferred; no
  frontend change is required by this spec.
- **Reordering the Tier 1 peer group.** Making `executor profile` beat `managed runtime`, or
  `repository` beat `executor profile`, is explicitly not part of this change and would contradict
  ADR-2026-08-03 and the tests `TestResolveEnvironmentSources_RejectsEveryConflictingPair`
  (`task_environment_test.go:40`) and
  `TestBuildEnvForExecution_RejectsLateManagedValuesThatConflictWithRepositorySecrets`
  (`manager_launch_test.go:530`).
- **Adding reserved-key validation to the executor-profile save path.** `TASK_DESCRIPTION` and
  `KANDEV_*` are rejected only on the agent-profile path (AC-40), so an executor profile can hold
  such a key today. This spec does not close that: precedence creates no new path to it, closing it
  would change behaviour on three save surfaces (HTTP, WS, MCP) that this card never asked to
  touch, and it could reject profiles already persisted in existing installs on their next update.
  A separate card owns it.
- **Adding duplicate-key validation to the executor-profile save path.** Same reasoning. AC-6a
  pins that the resulting pair conflicts rather than silently picking a winner, which is the part
  this feature is responsible for.
- **Adding value-or-secret validation to the executor-profile save path.** The blank-value problem
  is solved at ASSEMBLY (AC-38b), not at save time, so an existing profile carrying a blank entry
  keeps saving and loading exactly as it does now — the entry is simply dropped before resolution,
  on both paths instead of one. Validating it at save time was considered and deliberately not
  chosen, for the same back-compatibility reason as the two entries above.
- **Changing `definitionIdentity`.** Comparing decrypted plaintext remains rejected.

## Verification surfaces

Recorded as the E2E decision input.

- **Backend only.** Every acceptance criterion above is observable from Go tests against four
  seams and three artefacts:
  - seams: `runtimeenv.Resolve` (map, `[]OverrideRecord`, error), `runtimeenv.Validate` (error),
    `Manager.resolveStrictEnvironment` (log + counter side effects),
    `Executor.resolveLaunchEnvironment` (preflight verdict), and
    `Executor.taskEnvironmentSources` (the emitted `environmentSource` list — this is where the
    AC-38b blank-entry skip and the AC-6a duplicate-key case are observed);
  - artefacts: the returned `[]OverrideRecord` (AC-20, AC-27, AC-28), the
    `environment override applied` log record and its named fields (AC-23), and the
    `environment_override_applied_total` expvar map read back by its label key (AC-24, AC-24a).
  The expvar map is readable in-process via `expvar.Get("environment_override_applied_total")`,
  so the counter ACs need no HTTP round trip to assert.
- **One Go signature changes; no data shape does.** `Resolve` gains a third return value
  (`[]OverrideRecord`) and `Validate` is unchanged (AC-19b). `Definition`, `ConflictError` and
  `SecretError` keep their exported fields, so no caller has to reinterpret existing data. The
  signature change touches two call sites: `Manager.resolveStrictEnvironment`, which consumes the
  records, and the test-only-today `resolveEnvironmentSources`, which discards them.

  **One production behaviour changes outside the resolver**, and it is the only such change in this
  spec: `Executor.taskEnvironmentSources` (`task_environment.go:66-70`) starts skipping profile
  entries blank on both `Value` and `SecretID` (AC-38, AC-38b). No signature changes there and no
  data shape changes; the function simply emits one fewer `environmentSource` for an input the
  restart-recovery path already discards. Nothing else in this feature touches the orchestrator.

  The `environment` package's complete new exported surface, all named rather than left to the
  builder:
  - three types: `Tier`, `OriginTier`, `OverrideRecord`;
  - three `Tier` constants: `TierAuthoritative`, `TierProfileDefault`, `TierAgentRuntimeDefault`;
  - six origin constants: `OriginManagedRuntime`, `OriginManagedCredentials`,
    `OriginManagedAgentDefaults`, `OriginAgentProfile`, `OriginExecutorProfile`,
    `OriginRepositoryPrefix`;
  - four functions: `TierForOrigin`, `IsRepositoryOrigin` (AC-19c), `NormalizeOriginLabel`,
    `JoinOriginLabels` (AC-25).

  The tier table itself stays UNEXPORTED behind `TierForOrigin` (AC-19c). One new package,
  `internal/agent/runtime/envmetrics`, is created with one exported function,
  `RecordOverrideApplied` (AC-24a).
  This is not a REST/WS/DTO change: nothing crosses the HTTP or WebSocket boundary.
- **No frontend change.** The agent-profile editor
  (`apps/web/components/settings/profile-edit/`) and the executor-profile editor
  (`apps/web/app/settings/executors/[profileId]/page.tsx`) already accept the env vars this spec
  reconciles. No new copy, so no i18n work.
- **No database migration.**
- **E2E recommendation: none required.** The behaviour is a pure-function decision several layers
  below any user-visible surface, and no user-visible surface changes. Existing backend tests plus
  new unit tests at the resolver and both assembly sites give better coverage per unit of runtime
  than a browser test could.

Note for Build: `apps/backend/internal/agent/runtime/environment/` currently has **no test file**.
The resolver is covered only indirectly through the orchestrator and lifecycle packages. This
feature changes that resolver's core decision, so package-level tests belong there.

## References

- `apps/backend/internal/agent/runtime/environment/environment.go`
- `apps/backend/internal/agent/runtime/lifecycle/environment_resolution.go`
- `apps/backend/internal/agent/runtime/lifecycle/profile_env.go`
- `apps/backend/internal/orchestrator/executor/task_environment.go`
- `apps/backend/internal/orchestrator/executor/executor_state.go`
- `apps/backend/internal/agent/settings/controller/profile_crud.go` — the AGENT-profile save path,
  and the only home of all three save-time validators (`validateEnvVarValue:913`, reserved-key
  `:904`, duplicate-key `:908`). AC-38 no longer relies on any of them; AC-40 cites the reserved-key
  rule with its scope stated, and AC-6/AC-8d cite the duplicate-key rule the same way
- `apps/backend/internal/task/service/service_resources.go` — `CreateExecutorProfile` (`:1587`),
  `UpdateExecutorProfile` (`:1625`) and `validateGlobalProfileEnvRefs` (`:1665`): the
  EXECUTOR-profile save path, which enforces none of the three rules above
- `apps/web/components/settings/profile-edit/env-vars-card.tsx` — `rowsToEnvVars`, which filters
  empty keys only, so a blank-valued entry is savable from the UI as well
- `apps/backend/internal/workflow/signalmetrics/metrics_vars.go` — the expvar counter convention
  AC-24a follows (`expvar.NewMap` + `k1=v1;k2=v2` label keys)
- `docs/decisions/2026-08-03-scope-and-merge-repository-secrets.md`
- `docs/specs/workspaces/repository-secrets.md`
