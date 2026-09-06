---
status: draft
created: 2026-09-05
updated: 2026-09-05
owner: Kandev
---

# Workflow Engine Operation Ledger Lifetime

Decisions:

- [ADR-0004 task-model-unification](../../decisions/0004-task-model-unification.md) — makes the
  workflow engine the coordinator for task-scoped agent runs and gives every Phase 2 trigger an
  `OperationID`-keyed idempotency contract.

Related spec (states the contract this card makes true):

- [`docs/specs/workflow-on-agent-error-dispatch/spec.md`](../workflow-on-agent-error-dispatch/spec.md)
  — **AC-C5**: "The exactly-once guarantee is scoped to one backend process lifetime, because
  `workflowStore.appliedOps` is an in-memory `sync.Map`." That sentence is the contract; the code does
  not honour it. Its **AC-E13** placed the fix here rather than there.

## Why

`Engine.HandleTrigger`'s idempotency check is the only thing between a redelivered workflow trigger
and a second execution of its actions — a second `queue_run`, a second step transition, a second
`clear_decisions`. The ledger it consults is `orchestrator.workflowStore.appliedOps`, an in-memory
`sync.Map` (`apps/backend/internal/orchestrator/workflow_store.go:104`, read at `:886`, written at
`:894`).

`Service.initWorkflowEngine` (`service.go:1860`) builds a **brand-new** `workflowStore` on every
call (`store := newWorkflowStore(...)` at `service.go:1864`, assigned to `s.workflowStore` at
`:1867`). `newWorkflowStore` returns a fresh struct literal (`workflow_store.go:140`) in which
`appliedOps` is zero-valued and never copied from the previous store; no branch reuses one.
**Every call therefore discards the ledger.**

Thirteen `Service` methods reach `initWorkflowEngine` — two directly (`SetWorkflowStepGetter`
`service.go:1813`, `SetStepHistoryRecorder` `:1823`) and eleven through `reinitWorkflowEngine` (every
`Set*` between `:1888` and `:1971`, `SetReviewRunner` among them).

So the guarantee's real scope is not "one backend process lifetime". It is **"since the last `Set*`
call"**.

### The reachable window, measured

Verified by reading `apps/backend/internal/backendapp/main.go` and `helpers.go`.

| # | Boot site | Effect |
|---|---|---|
| 1 | `orchestrator.go:210-211` — `SetStepHistoryRecorder`, `SetWorkflowStepGetter` | first store built, **before** `Start` |
| 2 | `main.go:1000` — `orchestratorSvc.Start(ctx)` | reconcilers run, then `watcher.Start` / `scheduler.Start`: **live event delivery begins** |
| 3 | `main.go:1343` — `services.Office.RegisterEventSubscribers(eventBus)` | office subscribers go live |
| 4 | `main.go:1656-1671` — ten `SetEngine*` / `SetPrimaryAgentResolver` calls in `wireWorkflowEngineForOffice` | **ten wipes** |
| 5 | `helpers.go:1841` — `SetReviewRunner`, from `registerRoutes` → `registerMCPAndDebugRoutes` (`main.go:2286`) | **eleventh wipe** |

Two exposure windows follow, both open while triggers can fire:

- **Orchestrator-sourced triggers** — step 2 to step 5, spanning **eleven** wipes.
- **Office-sourced triggers** — step 3 to step 5, spanning **one** wipe. Before step 3 the office
  dispatcher is nil (assigned at the end of `wireWorkflowEngineForOffice`) and office triggers are
  dropped, so nothing earlier is at risk on that route.

The startup reconcilers named in the brief — `reconcileUnpublishedPromptTurnsOnStartup` and
`reconcileExecutorSessionsOnStartup` (`service.go:2696`, `:2708`) — run **before** `watcher.Start` and
mark no operations, so they are not a source of exposure. Answering the brief's first question:
**the reachable redelivery is not in the reconcilers, it is in the live post-`Start` window above.**

### A redelivery that is structurally guaranteed, not theoretical

`markTaskCompletedForTerminalStep` (`event_handlers_children_completed.go:59`) persists the task, then
does both of: (1) `publishTaskStateChanged` (`:89`), consumed by the orchestrator's own watcher
(`service.go:1434` wires `OnTaskStateChanged: s.handleTaskStateChanged`; `watcher/watcher.go:279`
subscribes it), which calls `processParentChildrenCompletedForTaskState`; and (2) that same call
directly (`:90`).

Both deliveries reach `processOnChildrenCompleted` for the same parent and read the same persisted
child rows, so `childCompletionOperationID` (`:320`) hashes to the **identical** operation id — the
row's `UpdatedAt` was written before either delivery. `lockChildCompletionOperation` serializes them
and `childCompletionAlreadyApplied` (`:179`) is expected to reject the second. It does, unless a
`Set*` call lands between them: then the second re-evaluates, and a transition that already committed
commits again, re-running the target step's `on_enter` sequence.

### What is *not* affected, contrary to the task brief

The brief states the ledger backs "every trigger (`on_enter`, `on_turn_start`, `on_turn_complete`,
`on_children_completed`, and now `on_agent_error`)". Verified by grepping every non-test
`OperationID:` producer and every `HandleTrigger` call site:

- **`on_turn_start` and `on_turn_complete` pass no `OperationID`**
  (`event_handlers_workflow.go:5620` and `:5039`); `isOperationAlreadyApplied` short-circuits on the
  empty string (`engine.go:313-315`), so neither touches the ledger. The brief's suggested regression
  test — "at least one trigger already in production use (e.g. `on_turn_complete`)" — would be
  vacuous. [AC-T2](#t-regression-coverage) names the triggers that do use it.
- **`on_enter`'s step-entry path has its own durable ledger.** `processOnEnter`
  (`event_handlers_workflow.go:3135`) keys on `step_entry:<entryID>:<position>` and claims it through
  `Repository.ClaimStepEntryMarker`, a database row; `Engine.DispatchStepEntry` never calls
  `IsOperationApplied`. Only the `switch_workflow` on_exit/on_enter route
  (`phase8_callbacks.go:111,133`, via `workflow_callbacks.go:104`) uses the in-memory ledger for
  `on_enter`.

The operations that **do** depend on `appliedOps` today, with their key shapes:

| Operation id shape | Producer | Consumer |
|---|---|---|
| `on_children_completed:<parent>:<sha256 of rows>` | `event_handlers_children_completed.go:320` | orchestrator |
| `wake:<parent>:<childSetKey>` | `office/service/event_subscribers.go:1099` (edge), `scheduler_wake_reconciler.go:124` (sweep) | engine (office) |
| `agent_error:<runID>` | `office/service/event_subscribers.go:676` | engine |
| `agentErrorOperationID(<session>,<agentExec>)` | `orchestrator/event_handlers_agent_error.go:58` | engine |
| `<commentID>` via `commentkeys.TaskComment` | `office/service/event_subscribers.go:1156`, `office/dashboard/service_tasks.go:1024` | engine |
| `blockers_resolved:<taskID>` | `office/service/event_subscribers.go:1027` | engine |
| `approval_resolved:<approvalID>` | `office/service/event_subscribers.go:1179` | engine |
| `heartbeat:<task>:<step>:<unix>` | `scheduler/cron/heartbeat.go:229` | engine |
| `budget:<ws>:<scope>:<id>:<pct>:<periodStart>` | `scheduler/cron/budget.go:174` | engine |
| `decision:<task>:<step>:<decision>` | `workflow/engine/quorum.go:673` | engine (direct) |
| `<parentOpID>:switch_workflow:on_(exit|enter)` | `workflow/engine/phase8_callbacks.go:111,133` | engine |

Every key except `heartbeat` is **level-triggered** — derived from durable state, not the moment of
delivery — so a redelivery reproduces it exactly. That is what makes the ledger load-bearing rather
than decorative.

## What

### Design decision

The set of operation ids marked applied is a property of the **orchestrator service**, not of any
`workflowStore` instance, and rebuilding the engine does not disturb it.

The ledger is a **field of `Service` whose zero value is usable**. It needs no construction step,
so it exists and works for every `Service` however that `Service` was built; it is never replaced;
and its address is handed to every `workflowStore` that `initWorkflowEngine` builds.
`workflowStore.IsOperationApplied` / `MarkOperationApplied` delegate to it, so
`engine.TransitionStore` is unchanged and the engine needs no modification.

The zero value is the load-bearing choice, not an implementation detail: a constructed ledger only
exists on a `Service` some constructor initialised, making correctness depend on how each `Service`
was built, whereas a zero value is correct for every `Service` that exists at all. It removes the nil
case rather than defining behaviour for it.

Three alternatives were considered and rejected:

- **Reorder boot so every `Set*` call precedes `Start`.** Rejected on evidence. `SetReviewRunner` is
  reached from `registerMCPAndDebugRoutes` inside `registerRoutes`, called from `buildHTTPServer`
  (`main.go:1183`) deliberately after `Start`, because the bootstrap listener must be bound and the
  recovery sweeps must run before the real router is built (`main.go:1174-1182`, and
  `docs/specs/startup-listener-before-recovery/spec.md`). Moving it fights an existing contract. More
  decisively, boot ordering is an invariant nothing enforces: the next `Set*` call added anywhere
  reintroduces the defect silently.
- **A constructed ledger passed as a nilable dependency, with a ledger-less store failing open.**
  Rejected after review: it makes the guarantee conditional on `Service` construction. A bare
  `&Service{...}` literal that never ran the constructor reaches `initWorkflowEngine` via
  `SetWorkflowStepGetter` and hands the store a nil ledger, which type-checks with no diagnostic. The
  store then never dedups — indistinguishable from a correct one until a duplicate arrives. A
  fail-open branch turns a wiring mistake into silence, worse than the boot-window bug this card
  fixes, which at least ends when boot ends.
- **Copy `appliedOps` forward inside `initWorkflowEngine`.** Rejected: `sync.Map` has no atomic
  snapshot, so a copy racing a concurrent `Store` drops entries — a partial wipe is no better than a
  full one, only harder to reproduce. It also leaves a store captured before the reinit (the
  `agentErrorDeps` snapshot at `service.go:1875`) writing into a map nothing reads afterwards.

### Acceptance criteria

#### L. Ledger lifetime

- **AC-L1** WHEN an operation id has been marked applied, THEN `IsOperationApplied` shall report it
  applied for the remainder of the process, across any number of `initWorkflowEngine` and
  `reinitWorkflowEngine` calls. This is the criterion the current code fails.
- **AC-L2** A `Service` shall hold exactly one ledger — the field AC-L3 describes. Every
  `workflowStore` that `initWorkflowEngine` constructs shall resolve to that same instance; a store
  built before a reinit and a store built after it shall observe each other's writes in both
  directions. A `workflowStore` constructed **directly**, outside `initWorkflowEngine`, is outside
  this criterion: it resolves to whichever ledger its caller passed, which for an isolated unit test
  is normally a ledger of its own. That is not a second ledger on a `Service`; it is a store that no
  `Service` owns.
- **AC-L3** The ledger shall be a **direct field of `Service` whose zero value is usable**: no
  constructor call, no initialisation statement and no assignment shall be needed to make it work, so
  a `Service` obtained by ANY means — `NewService`, or a bare `&Service{...}` literal naming none of
  its fields — has a working ledger from the moment the struct exists, before `Start` can run.
  The field shall never be reassigned, and `Service` shall never be copied by value — the orchestrator
  uses `*Service` throughout and `go vet`'s `copylocks` fails a build that copies it — which is what
  lets the field hold a `sync.Map` or mutex **directly** rather than behind a pointer. Taking its
  address is not a write, so nothing needs synchronising around the field itself; AC-L6 is the
  ledger's own guarantee. Lazily creating a ledger inside
  `initWorkflowEngine` does **not** satisfy AC-L2: it returns early when `workflowStepGetter` is nil
  (`service.go:1861`), so a lazily-created ledger has the same conditional lifetime as the bug.
  **No lock, atomic, or once-guard shall be added around the ledger reference itself** — two `Set*`
  calls racing each other read the same immutable field. (They still race on `s.workflowStore` and
  `s.workflowEngine`; that race is [out of scope](#out-of-scope) and AC-L1 does not depend on it.)
- **AC-L3a** A ledger at its zero value shall report every operation id as not applied. It shall
  not be pre-seeded, warmed, or hydrated from anything.
- **AC-L4** A `Service` on which `initWorkflowEngine` never ran — `workflowStepGetter` still nil —
  shall still have a **usable** ledger: an id marked on it shall be reported applied by a later check
  on the same `Service`, and neither call shall panic or return an error — this follows from AC-L3's
  zero value and needs no wiring.
  **No production reader is reachable in that state, and this card changes none of the guards that
  make it unreachable**: `processOnChildrenCompleted` returns before both
  `childCompletionAlreadyApplied` (`event_handlers_children_completed.go:180`) and
  `markChildCompletionApplied` (`:247`) whenever `workflowEngine`, `workflowStore` or
  `workflowStepGetter` is nil (`:94`); `switchWorkflowDispatcher` returns when the engine is nil
  (`workflow_callbacks.go:106`) and skips its check when the store is nil (`:113`). AC-L4 is therefore
  observed **directly on the ledger** (AC-T3(a)); reachability through a dispatch path is explicitly
  **not** claimed, and no guard shall be relaxed to manufacture one.
- **AC-L5** The ledger shall remain **in-memory and process-scoped**. A backend restart clears it.
  This restates `workflow-on-agent-error-dispatch` AC-C5 and is now true rather than aspirational;
  durability is [out of scope](#out-of-scope).
- **AC-L6** The ledger shall be safe for concurrent use by multiple goroutines without external
  locking, matching the `sync.Map` semantics its callers assume.

#### S. Semantics preserved exactly

- **AC-S1** IF `operationID` is the empty string, THEN `IsOperationApplied` shall return
  `(false, nil)` and `MarkOperationApplied` shall be a no-op returning `nil` — the behaviour at
  `workflow_store.go:886-901` today. An empty id shall never be stored, and shall never be reported
  applied on a later call.
- **AC-S2** `MarkOperationApplied` shall be idempotent: marking an already-marked id succeeds and
  leaves it marked. There shall be **no** un-mark, delete, clear, or reset operation on the ledger's
  production surface. Tests may construct a fresh ledger; they shall not clear a live one.
- **AC-S3** Neither method shall return a non-nil error under any input. Callers' fail-closed
  handling of an error (`childCompletionAlreadyApplied` treats an error as "applied",
  `event_handlers_children_completed.go:181-187`) stays in place but remains unreachable.
- **AC-S3a** No `workflowStore` reached by production code shall ever hold a nil ledger, and the
  guarantee shall be **structural, not conventional** — it shall not depend on how the owning
  `Service` was constructed.
  The ledger shall reach `workflowStore` as a **required positional parameter** of
  `newWorkflowStore`, last before its existing `publishers ...interface{}` variadic tail
  (`workflow_store.go:108-114`). It shall **not** be carried in that tail, which is dispatched by a
  type switch, so an omitted entry stays nil with no diagnostic. Positional placement also means a
  future `Set*`-plus-reinit method cannot obtain a store without naming a ledger, which is what AC-B2
  relies on.
  `initWorkflowEngine` shall pass **the address of the `Service`'s own ledger field**. That expression
  cannot be nil for any receiver, so the single production call site (`service.go:1864`) is non-nil
  **by construction for every `Service` however it was built** — including the bare `&Service{...}`
  literals that bypass `NewService` and still reach it through `SetWorkflowStepGetter`. With AC-L3's
  zero value there is therefore no `Service`, and no production `workflowStore`, without a working
  ledger.
  A nil ledger is **not a supported input** and **no defensive branch shall be added for it**: no
  nil-check in either method, no fail-open and no fail-closed path. A caller passing nil explicitly
  panics at first access — intended, and **loud**. Silently reporting "not applied" for a missing
  ledger is what this criterion forbids: a store that never dedups is indistinguishable from a correct
  one until a duplicate arrives in production, whereas a panic shows up on the first run.
  The production call site and all 35 direct test call sites shall pass a ledger explicitly. That
  fallout is cheap because the zero value is usable: a test uninterested in sharing passes a
  zero-valued ledger and gets exactly today's deduping behaviour, which is what AC-S4 relies on.
- **AC-S4** No operation-id derivation shall change. Every producer in the
  [table above](#what-is-not-affected-contrary-to-the-task-brief) keeps its exact key shape. This
  card changes **where the ledger lives**, not what is written into it. The existing operation-id
  tests shall keep passing with **unchanged deduping behaviour**, not merely green: every store they
  construct still has a working ledger (AC-S3a). Making one pass by removing its deduping would
  violate this criterion.
- **AC-S5** Ordering shall be unchanged: the engine marks an operation only after `processActions`
  returns without error (`engine.go:275`), and marks it when no actions are declared (`engine.go:267`).
  A ledger that marked earlier would swallow the retry `workflow-on-agent-error-dispatch` AC-C6
  requires.
- **AC-S6** The ledger shall not be a mutual-exclusion primitive. WHEN two goroutines check the same
  unmarked id concurrently, THEN both may observe "not applied" and both may proceed; serialization
  remains the caller's job through the mechanisms that already do it —
  `lockChildCompletionOperation` (`event_handlers_children_completed.go:196`) and
  `acquireTurnCompletionLock` (`service.go:2476`). Adding check-and-set to the ledger is
  [out of scope](#out-of-scope).
- **AC-S7** Operation ids shall be compared by **exact byte equality**. The ledger shall not trim,
  case-fold, normalize, hash, truncate, or otherwise transform a key, and shall impose no length
  limit or character restriction. Keys are opaque to it: `commentkeys.TaskComment` emits a prefixed
  comment id that `phase2_callbacks.go:422` inspects with `HasTaskCommentPrefix`, and
  `phase8_callbacks.go` derives `<parentOpID>:switch_workflow:on_enter` by concatenation, so any
  normalization would silently split or merge keys those sites construct.

#### R. Reader parity

- **AC-R1** The engine's `TransitionStore.IsOperationApplied` / `MarkOperationApplied` path shall
  read and write the shared ledger. `Engine.reevaluateGuardedTransitions`, which calls
  `e.store.IsOperationApplied` directly (`quorum.go:673`), is covered and needs no separate change.
- **AC-R2** The two orchestrator-level readers that bypass the engine shall observe the same ledger,
  and their result shall not depend on which `workflowStore` instance `s.workflowStore` points at:
  - `childCompletionAlreadyApplied` / `markChildCompletionApplied`
    (`event_handlers_children_completed.go:179`, `:246`);
  - `switchWorkflowDispatcher`'s pre-dispatch check (`workflow_callbacks.go:114`).
- **AC-R3** A dispatch holding a store captured **before** a reinit shall write to the same ledger a
  dispatch started after that reinit reads. This covers the `agentErrorDeps` atomic snapshot
  (`service.go:1875`), which pins one engine/registry/store triple for a dispatch's duration and
  today pins a ledger a concurrent reinit orphans.
- **AC-R4** WHEN an operation is marked through one reader class and checked through another — say
  marked by the engine under `on_agent_error` and checked by `switchWorkflowDispatcher` — the check
  shall see it. There is one ledger, not one per reader.

#### B. Observable boot behaviour

- **AC-B1** WHEN a trigger operation is marked applied at any point after `watcher.Start` and the
  identical operation id is redelivered later in the same process, THEN the redelivery shall be
  deduped, regardless of how many `Set*` calls executed in between. The structurally guaranteed
  redelivery described above is the concrete case this must cover.
- **AC-B2** The contract shall hold without depending on boot ordering. Adding a new
  `Set*`-plus-`reinitWorkflowEngine` method, or moving an existing call site, shall not be able to
  reintroduce the defect. A fix expressed only as a reordering of `main.go` does **not** satisfy this
  criterion.
- **AC-B3** `initWorkflowEngine` may continue to rebuild everything else — the store's publishers,
  the callback registry, the engine, the `agentErrorDeps` snapshot, and `stepCache`
  (`workflow_store.go:150`), a TTL cache with an explicit invalidation path whose loss costs one
  reload per key and is not a correctness change. Only the ledger is exempt from the rebuild.

#### T. Regression coverage

- **AC-T1** A test shall prove AC-L1 directly: mark an operation id, call a `Set*` method that
  triggers `reinitWorkflowEngine`, and assert the id still reads applied through the **new** store.
  Asserting only that the old store still remembers it does not satisfy this — the old store is
  exactly what production stops reading.
- **AC-T2** A test shall prove end-to-end dedup across a reinit for **both** reader classes, using a
  trigger already in production use:
  - the **engine** class — `on_agent_error` through `HandleTrigger` with a fixed
    `agentErrorOperationID`, asserting the second delivery returns `Idempotent: true` and runs no
    action;
  - the **orchestrator** class — `on_children_completed` through `processOnChildrenCompleted` with an
    unchanged child-row set, asserting the second delivery applies no second transition.

  `on_turn_complete` shall **not** be used: it passes no `OperationID` and would assert nothing.
- **AC-T3** A test shall prove the two wiring cases the mechanism turns on. Both are positive
  assertions about a working ledger, not "does not panic" checks, and both are in-package tests
  reading the `Service`'s unexported ledger field directly — no exported accessor shall be added:
  - **(a) AC-L4** — a `Service` whose `workflowStepGetter` was never set, so `initWorkflowEngine`
    never ran and `workflowStore` is nil: mark an id on its ledger field, assert a later check reports
    it applied. It shall **not** route through `processOnChildrenCompleted` or
    `switchWorkflowDispatcher`, whose guards make those paths unreachable here (AC-L4), and shall
    **not** require any guard to be relaxed.
  - **(b) AC-S3a** — a `Service` built as a bare `&Service{...}` literal, bypassing `NewService`, then
    wired with `SetWorkflowStepGetter`: assert the `workflowStore` it produces resolves to **that
    `Service`'s** ledger in both directions. This is the regression guard against a
    `Service`-construction-dependent mechanism; `createEngineService` (`workflow_e2e_test.go`) has
    exactly that shape.
- **AC-T4** A test shall pin AC-L3 structurally — that `initWorkflowEngine` neither declares nor
  assigns a ledger of its own, and reaches the ledger only as the address of the `Service`'s existing
  field — so a later refactor that gives the rebuild path a ledger of its own fails loudly rather
  than silently restoring "since the last `Set*` call".

## Concurrency and ordering

- **Two callers, same unmarked id.** Both may proceed (AC-S6). Unchanged, and deliberately so: the
  ledger is a *memo*, the existing per-operation locks are the *mutex*. Collapsing them would change
  serialization semantics this card does not touch.
- **Reinit concurrent with an in-flight dispatch.** The dispatch's check and its later mark land in
  the same ledger (AC-R3), so a reinit cannot split one dispatch across two ledgers. The dispatch may
  still use a *pre-reinit* engine and registry — pre-existing behaviour, and the `agentErrorDeps`
  snapshot's stated design.
- **Mark-to-commit ordering is unchanged** (AC-S5): the engine marks after actions succeed, and
  `applyEngineTransition` may still decline a transition the engine already marked —
  `workflow-on-agent-error-dispatch` AC-C7 covers that and is not reopened here.
- **No ordering is defined between two distinct operation ids.** The ledger is an unordered set: no
  iteration order, no observable insertion sequence, nothing may be derived from one.

## Failure modes

- **Restart.** The ledger is empty; every level-triggered operation is redelivered once and
  re-executes. That is today's behaviour and AC-L5's stated limit, and is why the paths that cannot
  tolerate it carry durable receipts of their own: `ClaimStepEntryMarker` for step entry,
  `UpsertWakeReceiptTx` for the parent-wake reconciler
  (`office/service/scheduler_wake_reconciler.go:211`).
- **Unbounded growth.** Never pruned; one entry per distinct operation id for the process lifetime.
  This card does not change the growth rate — after boot there are no further reinits today, so
  entries already accumulate for the whole process. The wipes it removes all occur during boot, when
  the ledger holds at most a handful of entries. Bounding it is [out of scope](#out-of-scope).
- **A ledger read that could not answer.** Not reachable (AC-S3). The fail-closed branches treating
  an error as "already applied" stay as written, so if a future ledger *can* error the conservative
  outcome is a skipped duplicate, not a double execution.

## Out of scope

Each entry is a named exclusion and part of the contract.

- **Durability across restart.** Surviving a restart means a table, a retention policy and a
  migration. `workflow-on-agent-error-dispatch` AC-C5 already declares the process-lifetime bound and
  `workflow-on-enter-action-dispatch`'s step-entry record owns the durable-marker question. This card
  makes the declared bound true; it does not widen it.
- **Eviction, TTL, or any bound on ledger size.** Requires deciding what a *safe* eviction is for a
  level-triggered key like `blockers_resolved:<taskID>`, which has no natural expiry — evicting it
  re-arms a duplicate wake. A separate design with its own risk.
- **Defensive nil-ledger handling.** AC-S3a makes a nil ledger unreachable from production *by
  construction* rather than tolerated, so no nil check, fail-open branch, fail-closed branch, or
  substitute-a-private-ledger fallback shall be added to either ledger method or to
  `newWorkflowStore`. Explicit nil panics on first access; that is the specified behaviour. A fallback
  would restore the silent divergence this card was re-scoped to remove: a store deduping against
  nothing, or against a private ledger no other store can see, reporting success either way.
- **Synchronizing `s.workflowStore` and `s.workflowEngine` themselves.** Both are plain fields,
  written by `initWorkflowEngine` (`service.go:1867`, `:1874`) while the watcher is live and read
  from bus goroutines. That is a pre-existing data race, was already excluded by
  `workflow-on-agent-error-dispatch` AC-E13, and is excluded here too. **The fix for this card must
  not depend on it being fixed:** a shared ledger reached through whichever store instance a racing
  reader happens to observe is still the same ledger, so AC-L1 holds either way. A separate card
  should still close the race.
- **`engineOptions` accumulation.** `s.engineOptions` is appended to on each `Set*` call and never
  reset (`service.go:1887` and siblings), so it grows across reinits. Bounded by the number of `Set*`
  methods and harmless (later options win), but the same wiring and worth a look someday. Not here.
- **Giving `on_turn_start` / `on_turn_complete` an `OperationID`.** They deliberately have none; the
  turn-completion path has its own duplicate-suppression (`acquireTurnCompletionLock`,
  `acquireTurnCompletionCriticalSection`), and an engine-level key would add a second,
  differently-scoped dedup over the same event. That is a contract change to those triggers, not a
  ledger change.
- **Boot reordering.** Rejected as the fix ([Design decision](#design-decision)), and excluded as an
  addition: this card shall not move `orchestratorSvc.Start`, `wireWorkflowEngineForOffice`,
  `RegisterEventSubscribers` or `registerRoutes` relative to one another.
- **Anything specific to `on_agent_error`'s own dispatch.** `agentErrorOperationID`, the
  `agentErrorDeps` snapshot's contents and the fire-site pin test are unchanged; AC-R3 constrains only
  which ledger that snapshot's store resolves to.
- **Reducing the number of `reinitWorkflowEngine` calls.** Thirteen `Set*` methods rebuilding the
  engine is its own smell, but collapsing them is a wiring refactor across `main.go`, `helpers.go` and
  `orchestrator.go`, and it is not needed for AC-B2, which holds by construction once the ledger is
  exempt from the rebuild.

## Prior art

Two legs were attempted. **Both were unavailable; neither result means "there is no prior thinking
here."**

- **Wiki (`wiki-query @henry`).** Receipt: config `~/.obsidian-wiki/config.henry`,
  `OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`, collections `wiki` / `sources`. **The
  vault could not be read** — `ls`, `head` and a sandbox-disabled retry all returned `Operation not
  permitted` (macOS TCC on `~/Documents`, not a missing vault; `ls -d` on the parent resolves).
  Neither `obsidian-wiki` nor `qmd` is on `PATH` here and no `mcp__qmd__*` tool is exposed, so the QMD
  and graph paths were also unavailable. **Zero pages consulted.**
- **`saas-kb` (`search_fsm_docs`, `category: "ai_sdlc"`).** **Tool not available** — the only MCP
  servers exposed are `kandev` and `codex`. No query run, no vendor comparison made.

**In-repo prior art, available and the substantive input here:** this codebase has already solved "an
idempotency marker must outlive the thing that consumes it" twice, both times by giving the marker a
lifetime independent of the engine — `ClaimStepEntryMarker` / `CompleteStepEntryMarker` for
step-entry actions (`event_handlers_workflow.go:3136`), and `UpsertWakeReceiptTx` for the parent-wake
reconciler (`office/service/scheduler_wake_reconciler.go:211`). This card is the same move at a
smaller radius: the ledger keeps in-memory storage (AC-L5) and gains only an independent
**lifetime**.

## Verification

No user-visible surface changes: no route, no WS payload, no DTO field, no copy. **No E2E coverage is
required or added.** Every criterion is observable from Go unit tests in
`apps/backend/internal/orchestrator`.


```bash
cd apps/backend
go test -race ./internal/orchestrator/... ./internal/workflow/...
golangci-lint run ./... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m
```

Criteria and their observation points:

| Criterion | Observed by |
|---|---|
| AC-L1, AC-L2 | mark → `Set*` → read through the new `s.workflowStore` (AC-T1) |
| AC-L3, AC-B2 | `initWorkflowEngine` neither declares nor assigns a ledger, reaching it only as the address of the `Service` field (AC-T4) |
| AC-L4 | nil `workflowStepGetter` service: mark then check on its ledger field reports applied, no production guard changed (AC-T3(a)) |
| AC-L6 | `-race` on a concurrent mark/check test |
| AC-L3a | a zero-valued ledger reports nothing applied |
| AC-L5 | a new `Service`'s ledger reports nothing applied (AC-L3a's mechanism; a restart *is* a new process and a new `Service`), and no persistence is added: no table, no migration, no `repo` write |
| AC-S1, AC-S2, AC-S3, AC-S7 | direct ledger unit tests |
| AC-S3a | bare `&Service{}` literal wired via `SetWorkflowStepGetter` yields a store resolving to that `Service`'s ledger, both directions (AC-T3(b)) |
| AC-S4 | unchanged producers; existing operation-id tests keep passing with unchanged deduping behaviour, not merely green |
| AC-S5 | existing engine tests (`engine_test.go`, `idempotency_key_test.go`) |
| AC-S6 | the same `-race` mark/check test as AC-L6, asserting both goroutines may observe "not applied" and proceed; the ledger exposes no check-and-set, compare-and-swap or `LoadOrStore`-style method usable as a mutex |
| AC-R1, AC-R4 | mark via engine, check via `switchWorkflowDispatcher`'s path |
| AC-R2 | `on_children_completed` dedup across a reinit (AC-T2) |
| AC-R3 | `on_agent_error` dedup across a reinit (AC-T2) |
| AC-B1 | AC-T2, both classes |
| AC-B3 | `stepCache` still rebuilt; no assertion that it survives |
