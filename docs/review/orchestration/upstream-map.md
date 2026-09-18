# Proposed upstream contribution map

This is extraction design for private review, not an exported branch or a public
PR. The integration baseline remains exact v0.94.0
(`bf819a0228e742d069c528293d848c985a4d1bd1`). Confirm the actual target branch and
scope in issue #3752 before publishing. Never push the private integration
branch or its review/operational history to the public fork.

The issue was rechecked on 2026-09-18. The current
[maintainer response](https://github.com/kdlbs/kandev/issues/3752#issuecomment-5713059573)
asks to gather Coordinator requirements, proposes existing task statuses/groups
beside agent chat, and points to the related plugin. The public contribution's
precise scope and target branch still need agreement; assistant scope has not
been accepted there. No additional public message was posted during this check.

## Source-to-contribution map

| Proposed contribution | Private source checkpoints | Required behavior and validation |
| --- | --- | --- |
| Core runtime, schema and privacy foundations | `3536e04ca`; PostgreSQL fixture correction `fc05e0617` | Shared runs/comments, required-store boot order, scoped conversation/task/session access, retained ownership, queue identity and lifecycle callbacks. Preserve native Office behavior. Run persistence/upgrade/race and native access tests. |
| Coordinator roles, persistent chat and central task view | Coordinator portions of `3536e04ca`, observations `13a3a5ae5`, interface `a77994f13` | Existing workspace/workflow execution, canonical status, bounded paging, owner/workspace isolation, task links and callbacks, desktop/phone controls. Run Coordinator, workspace chat and Automation together; include Office compatibility and inspected synthetic media. |
| Optional Automation destination | Automation portions of `3536e04ca` and shared follow-up fixes | Coordinator assignment owns execution configuration; retries preserve occurrence identity; dispatch is distinct from completion. Portable YAML/ZIP export remains explicitly unsupported until separately designed. |
| Personal assistant, as a separately agreed series | Gate `d515fbcfb`; context `3cf603264`; directory `914325adf`; broker `f2757b392`; attention `c7aa9afc5`; resolution `759182a0c`; interface `7bcf8c464`; maintenance `3309fb99f`/`61c5c5ec4`; workspace grants `41de43a14`; integration corrections `c357f9ab9` | All 39 assistant criteria, native effect enforcement, retained context/privacy and distinct human approvals. Keep default-off rollout and unsupported-provider reporting. Full qualification and separate provider evidence remain required. |

The original import mixes these concerns, so it is a patch source rather than a
single contribution to cherry-pick. Later topic commits also contain shared
integration seams. Their commit boundaries do not prove independent buildability.

## Dependency constraints

The current required-store catalog orders Orchestration after tasks and agent
settings, and Office after tasks, settings, runs and Orchestration. The retained
Office adapter initializes the Orchestration store for compatibility. The first
full PostgreSQL qualification exposed seven fixtures that had omitted settings;
their correction must accompany any extraction with that schema dependency.

Retained conversation ownership, native task/session access checks, profile
deletion cleanup and migration markers are mandatory even if the first public UI
omits the assistant. Removing a registration or disabling either flag must not
make private history reachable through a legacy Office or ordinary task route.
Do not remove these guards to make a smaller diff compile.

The shared chat renderer accepts feature-specific transport/identity adapters.
Coordinator extraction must retain those seams without importing Office pages or
stores into Orchestration. Native Office task creation and workspace navigation
remain compatibility requirements; the qualification-discovered New Task
regression must not reappear in the export. Its correction is `2bcfd279a4`; the
onboarding/navigation fixture coverage is `3bb229028`.

If the coordinator slice still needs assistant runtime symbols, either retain
the smallest required invariant or introduce a tested narrow interface before
removing optional code. A file list alone is not a dependency proof. Each public
slice must compile, migrate and pass its scoped behavioral checks independently.

## Material excluded from public history

Keep `WORKBENCH.md`, `docs/review/orchestration/**`, private delivery receipts,
operational manifests, database backups and raw test/provider logs in the private
workbench. Public contribution docs should be rewritten around the exported
behavior, with only reviewed requirements/design/usage notes and generic media.
The current private evidence images identify their original capture commits;
capture again from the actual export revision before attaching PR evidence.

No plugin source was copied. The [plugin review](plugin-review.md) records a
pinned review and API compatibility limits, not a tested plugin integration.

## Export checklist

1. Resolve maintainer scope/target guidance while retaining the user's v0.94.0
   integration baseline unless a newer base is explicitly requested.
2. Create a fresh checkout from the agreed public base; apply reviewed scoped
   patches and focused commits without importing private ancestors.
3. Inspect dependency closure, migration order, feature-off paths and ordinary
   Kanban/Office compatibility for every slice. Record private source SHA to
   exported commit SHA once each slice exists.
4. Run the affected unit, race, SQLite/PostgreSQL, locale and browser checks with
   normal hooks. Evidence from the larger private branch does not qualify a
   differently extracted tree.
5. Capture and inspect generic screenshots/video from that export, scan its full
   outgoing history, and rewrite the PR draft around its final scope.

Daily-use evidence and the actual extraction remain delivery work. There is no
public code push, PR or new issue comment associated with this map.
