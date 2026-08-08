# ADR-2026-08-08-office-domain-ownership-boundary: Office Sub-Packages Own Domain CRUD; `office/service` Owns Run Execution

**Status:** proposed
**Date:** 2026-08-08
**Area:** backend

## Context

`internal/office/service` (7,818 LOC across 38 non-test files) carries a verbatim
copy of logic that also lives in the sub-packages extracted out of it. An
AST-based scan of `internal/office/**` (committed at
`docs/plans/office-service-collapse/officedup/`) finds **40 groups of
byte-identical function bodies** spanning package boundaries and **37 same-name
near-duplicate pairs** between `office/service` and its sub-packages.

Both copies are compiled into the binary. `internal/backendapp` wires
`office/service` *and* `office/agents`, `office/config`, `office/projects`,
`office/routines`, `office/skills`, `office/channels`, `office/scheduler`.

The extraction did complete on the routing side, and this is the fact that
settles the decision: `internal/office/service` contains **no `gin` import and
has no HTTP surface**. `office/routes.go` builds every handler from the
sub-package services, and `office/services.go:26-40` wires
`Agents`/`Skills`/`Projects`/`Routines`/`Config`/`Channels` to sub-package types.
Even the `office/runtime` action interfaces — `Projects.CreateProject`,
`Skills.DeleteSkill`, `AgentModifier.UpdateAgentInstance` — resolve to
`svcs.Projects` / `svcs.Skills` / `svcs.Agents`, not to the facade.

What the extraction never did was delete the source. The result is not a facade
delegating to sub-packages; it is an orphaned copy whose duplicated CRUD is
reachable only from inside `service` itself, from its own test suite, and from
six `office/scheduler` call sites.

Meanwhile the duplication has started to diverge, in both directions. The facade
imports a config bundle across *every* workspace and writes rows with an empty
`WorkspaceID`, where `office/config` scopes correctly; and `office/config` writes
those rows through the repository directly, skipping the validation and
defaulting the facade performs. Elsewhere the facade swallows a repository error
and admits a duplicate agent name, where `office/agents` propagates it. Each
divergence is a latent bug that exists *because* nobody owns the behavior.

## Decision

**Domain ownership in `internal/office` follows the HTTP boundary that already
exists.**

1. The feature sub-packages — `agents`, `skills`, `projects`, `routines`,
   `config`, `channels`, `costs`, `approvals`, `labels`, `dashboard`,
   `onboarding` — own their domain's CRUD, validation, config import/export, and
   HTTP routes. They are the single implementation.

2. `internal/office/service` owns **run execution only**: the tick and dispatch
   loop, prompt and environment construction, executor resolution, wake payloads,
   failure and retry handling, event subscribers, task-tree controls, and
   workspace deletion. It has no `gin` import and must not gain one. New office
   CRUD goes in the sub-package, never in `service`.

3. Where a duplicate exists, the **owner's** copy survives and the other is
   deleted — not wrapped, not delegated to. Delegation is what produced the
   current state.

4. The one reversal: `office/scheduler` already imports `office/service`
   (`scheduler/run.go:154` holds `svc *service.Service`; `run.go:54` and
   `executor_resolver.go:11` alias `service.LaunchContext` and
   `service.ExecutorConfig`). Run execution is `office/service`'s domain under
   rule 2, so for the scheduler fork the **facade is the owner** and
   `office/scheduler`'s copies collapse onto it. Inverting that edge would create
   an import cycle.

5. Duplication is measured, not asserted. The AST detector at
   `docs/plans/office-service-collapse/officedup/` is the check; its group count
   is the regression signal.

The migration is sequenced in
[`docs/plans/office-service-collapse/plan.md`](../plans/office-service-collapse/plan.md)
as ten independently mergeable PRs.

## Consequences

**Positive.** One implementation per behavior, so the six recorded divergences
resolve by construction rather than by future bug reports. Roughly 2,390 LOC of
production code and a comparable volume of duplicated tests are removed. The
boundary is mechanically checkable — a new duplicate raises the detector's group
count. `office/config` and `office/projects` gain their first test coverage, as
part of relocating the suites that today only exist on the facade side.

**Negative.** Ten PRs of churn across a load-bearing subsystem. The riskiest
step is config import/preview, where the correct end state exists on *neither*
side: it needs `office/config`'s workspace scoping combined with the facade's
validated create path. Test coverage is unevenly distributed — `office/config`
and `office/projects` have zero tests while the facade has 720 LOC covering
those domains, so the relocation is a genuine porting exercise, not a file move.
Two changes are externally observable and are called out for explicit decision
rather than assumed: the `events.OfficeCommentCreated` source string
(`"office-service"` → `"channels-service"`) and the `GetSkillFromConfig` error
form (plain → `ErrSkillNotFound`-wrapped).

**Neutral.** `office/workspaces` (61 LOC) and `office/tree_controls` are
unaffected. They are thin gin handlers over `*service.Service` containing no
duplicated logic; under this ADR they are the definition of the run-engine
surface the facade retains, not candidates for collapse.

## Alternatives Considered

**Absorb the sub-packages back into `office/service`.** Rejected. It contradicts
the wiring that already ships — the sub-packages own every office HTTP route —
so it would mean rewriting `office/routes.go` and `office/services.go` to
unwind a completed migration. It also recreates a single 15,000-LOC package
against the `.golangci.yml` complexity limits, and discards the better behavior:
`office/config`'s workspace scoping and `office/agents`' error propagation both
live on the sub-package side.

**Keep the facade and make it delegate.** This is the option the original
extraction implicitly aimed at, and it is the most tempting because it is the
smallest diff. Rejected because it preserves two public entry points for every
behavior and therefore preserves the conditions that produced the drift — a
delegating facade still lets a caller reach CRUD through `office/service`, and
the next divergence arrives the same way this one did. It also carries a
permanent indirection cost for a facade that, per the wiring evidence above, has
no external CRUD caller to serve.

**Leave it alone and document the duplication.** Rejected. The six recorded
divergences are already live bugs, not hypothetical risk: cross-workspace config
import, rows created without a `WorkspaceID`, imports bypassing validation, a
swallowed uniqueness error, and stale runs rescheduled instead of cancelled.
Documentation does not fix any of them, and the duplication is still growing —
three of the forty duplicate groups (`office/onboarding`, `office/dashboard`)
postdate the original extraction.

## References

- Plan: [`docs/plans/office-service-collapse/plan.md`](../plans/office-service-collapse/plan.md)
- Evidence: [`docs/plans/office-service-collapse/inventory.md`](../plans/office-service-collapse/inventory.md)
- Detector: `docs/plans/office-service-collapse/officedup/`
- Constraint: `apps/backend/internal/office/repository/sqlite/base.go:60`
  (`Repository` embeds `*runssqlite.Repository`; run-write ownership is untouched)
