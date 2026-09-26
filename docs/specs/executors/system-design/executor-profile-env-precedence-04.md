---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001
created: 2026-08-17
owners:
  - tbd
---
# Executor-Profile Environment Precedence System Design Part 4

## Purpose and boundaries

This design preserves the technical source detail for `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-EXECUTORS-EXECUTOR-PROFILE-ENV-PRECEDENCE-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

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
