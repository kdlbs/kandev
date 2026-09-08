---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001
created: 2026-08-09
updated: 2026-08-09
owners:
  - nova28
---
# Automations — "Pull request merged" trigger System Design Part 7

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Scenarios

Detection and matching:

- **GIVEN** the same valid `github.task_pr.updated` event delivered once by the in-memory
  bus as `*github.TaskPR` and once through the NATS JSON wire representation, **WHEN** the
  subscriber evaluates each delivery, **THEN** both normalize to the same trigger data and
  firing decision.
- **GIVEN** a malformed NATS-decoded event object, **WHEN** it is delivered, **THEN** it is
  ignored without listing or firing a trigger.
- **GIVEN** an enabled automation in workspace W with a `github_pr_merged` trigger and
  `all_repos: true`, **AND** task T in W with a linked PR `acme/api#7` in state `open`,
  **WHEN** the PR poller observes the PR is merged, **THEN** the automation fires once and
  a run row is recorded with dedup key `pr_merged:<T>:acme/api#7`.
- **GIVEN** that firing, **WHEN** the run task is created, **THEN** its interpolated prompt
  contains T's id and names `archive_task_kandev`, and its title is
  `[Auto] PR merged — acme/api#7`.
- **GIVEN** that firing has already created a task, **WHEN** a further
  `github.task_pr.updated` arrives for the same task and PR (e.g. a late review count
  settles), **THEN** the automation's run count for that dedup key stays at one and no
  second task is created. Note what is **not** asserted: dedup suppression writes **no**
  run row at all — it returns a skip result to its caller and writes a debug log, and this
  subscriber discards that result. The observable is the absence of a second run, not a
  recorded reason.
- **GIVEN** an automation at `max_concurrent_runs: 1` whose matching firing was recorded as
  `skipped` for the cap, **WHEN** a later `github.task_pr.updated` arrives for the same task
  and PR, **THEN** the trigger fires and a task is created — the `skipped` row did not
  consume the dedup key.
- **GIVEN** a matching firing that was recorded as `failed` (e.g. no repository available),
  **WHEN** a later `github.task_pr.updated` arrives for the same task and PR, **THEN** the
  trigger fires — the `failed` row did not consume the dedup key either.
- **GIVEN** a `github_push` trigger whose firing was recorded as `skipped` for the cap,
  **WHEN** an event with the same dedup key is redelivered, **THEN** it is still suppressed
  — the do-not-consume rule is scoped to `github_pr_merged` and other trigger types keep
  their existing behavior.
- **GIVEN** the same PR `acme/api#7` is linked to two tasks T1 and T2 in W, **AND** the
  automation's `max_concurrent_runs` is **2** (or unlimited), **WHEN** it merges, **THEN** two
  runs are created, one per task, with distinct dedup keys.
  **The cap value is load-bearing in this scenario and must be set explicitly, not left at the
  default.** Two linked tasks produce two separate `github.task_pr.updated` events carrying two
  different `task_id`s, so the keys differ and dedup suppresses neither — but the two firings land
  on the *same* automation, so the concurrency cap is what decides whether the second one creates
  a task. At the shipped default of `max_concurrent_runs = 1` the outcome is **not
  deterministic**: per [Concurrency](#concurrency) the cap is non-atomic and the first firing's
  run row is written asynchronously, so the second firing usually observes zero active runs and
  fires, but may observe one and be capped instead — in which case it records a `skipped` row
  whose `dedup_key` is **empty** per
  [Idempotency](#which-runs-consume-the-dedup-key), and "two runs with distinct dedup keys" is
  false. At `2` (or unlimited) both firings are admitted whichever way the race falls, because
  the active count can only be 0 or 1 and both are below the cap. A conformance test that leaves
  the cap at its default is therefore testing the race, not this rule, and will flake against a
  correct build.
- **GIVEN** a merged-PR event whose `TaskPR.State` is `"Merged"` (mixed case), **WHEN** it
  is evaluated, **THEN** it matches — the state comparison is case-insensitive and trimmed.
- **GIVEN** a merged-PR event whose `MergedAt` is nil, **WHEN** it is evaluated, **THEN**
  the trigger fires and `{{data.merged_at}}` renders as an empty string.
- **GIVEN** a linked PR that transitions to `closed` without merging, **WHEN** the event is
  published, **THEN** no `github_pr_merged` trigger fires.

Filters:

- **GIVEN** a trigger with `all_repos: false` and `repos: [{owner: "acme", name: "api"}]`,
  **WHEN** a PR in `acme/web` merges, **THEN** the trigger does not fire.
- **GIVEN** the same trigger, **WHEN** a PR in `ACME/API` merges, **THEN** the trigger
  fires — repository comparison is case-insensitive.
- **GIVEN** a trigger with `all_repos: false` and `repos: []`, **WHEN** any PR merges,
  **THEN** the trigger does not fire.
- **GIVEN** a trigger whose stored config is `{}`, **WHEN** any PR merges, **THEN** the
  trigger does not fire.
- **GIVEN** a trigger with `base_branches: ["release/*"]`, **WHEN** a PR merges into
  `release/v2`, **THEN** it fires; **WHEN** a PR merges into `main`, **THEN** it does not.
- **GIVEN** a trigger with `base_branches: [" main ", ""]`, **WHEN** a PR merges into
  `main`, **THEN** it fires — entries are trimmed and empty entries dropped.
- **GIVEN** a trigger with `base_branches: ["", "  "]`, **WHEN** a PR merges into any
  branch, **THEN** it fires — the list is empty after dropping empties.
- **GIVEN** a trigger with `all_repos: false` and `repos: [{owner: "acme", name: ""}]`,
  **WHEN** a PR in `acme/api` merges, **THEN** it fires; **WHEN** a PR in `other/api`
  merges, **THEN** it does not — an entry with an empty `name` is an organization wildcard.
- **GIVEN** a trigger with `all_repos: false` and `repos: [{owner: "", name: "api"}]`,
  **WHEN** any PR merges, **THEN** it does not fire — an entry with no owner matches
  nothing.
- **GIVEN** a merged-PR event whose `Owner` or `Repo` is empty, **WHEN** it is evaluated,
  **THEN** no trigger fires.
- **GIVEN** a merged-PR event whose `PRNumber` is `0` or negative, **WHEN** it is
  evaluated, **THEN** no trigger fires and no trigger is listed.
- **GIVEN** a merged-PR event whose `PRURL` is empty, **WHEN** it is evaluated, **THEN** the
  trigger fires and `{{data.pr_url}}` renders as an empty string.
- **GIVEN** a merged-PR event whose `BaseBranch` is empty and a trigger with
  `base_branches: []`, **WHEN** it is evaluated, **THEN** it fires and
  `{{data.base_branch}}` renders as an empty string; **GIVEN** the same event and
  `base_branches: ["*"]`, **THEN** it fires; **GIVEN** the same event and
  `base_branches: ["main"]`, **THEN** it does not fire.

Data-map and prompt contract — these are the security contract, and they are asserted
mechanically rather than inferred from the agent's behavior:

- **GIVEN** any `github_pr_merged` firing, **WHEN** its `trigger_data` is inspected,
  **THEN** its keys are **exactly** `task_id`, `repo`, `pr_number`, `pr_url`,
  `base_branch`, `merged_at` — no more and no fewer.
- **GIVEN** the same firing, **WHEN** its `trigger_data` is inspected, **THEN** it contains
  **none** of `pr_title`, `body`, `author_login`, `head_branch`, under any spelling. This is
  a standing assertion, not a restatement of the one above: it is what fails if someone
  later widens the map for convenience.
- **GIVEN** the trigger-type registry, **WHEN** the `github_pr_merged` entry's
  `default_prompt` is read, **THEN** it matches the pinned text in
  [API surface](#trigger-type-registry) exactly.
- **GIVEN** the registry entry, **WHEN** its `placeholders` are read, **THEN** the six
  type-specific records match the key/description/example table in
  [API surface](#trigger-type-registry), followed by the common placeholders.

Retroactivity and first observation:

- **GIVEN** a workspace with existing merged pull requests linked to tasks, **WHEN** an
  automation with this condition is created and enabled, **THEN** nothing fires — creating a
  trigger publishes no event and there is no backfill sweep.
- **GIVEN** an enabled automation with this condition, **WHEN** the user links an
  already-merged pull request to a task in that workspace, **THEN** the association
  publishes `github.task_pr.updated` with `state = merged`, the trigger fires once, and a
  run row records which task it targeted.
- **GIVEN** a task whose linked PR merged and was already archived by a previous firing,
  **WHEN** any further event arrives for that pair, **THEN** dedup suppresses it and no
  second run is created.

Scoping and safety:

- **GIVEN** an automation in workspace W and a merged PR linked to a task in workspace V,
  **WHEN** the event is evaluated, **THEN** the automation does not fire.
- **GIVEN** a merged-PR event whose `TaskPR.WorkspaceID` is `""` and whose task resolves to
  workspace W, **WHEN** an automation in W evaluates it, **THEN** it fires — the payload
  field is not consulted.
- **GIVEN** a merged-PR event whose `TaskPR.WorkspaceID` says W (a stale value) while the
  task lookup resolves the task to workspace V, **WHEN** an automation in W evaluates it,
  **THEN** it does **not** fire, and an automation in V **does** — the lookup is
  authoritative and the payload field has no vote.
- **GIVEN** a merged-PR event whose `TaskPR.WorkspaceID` is `""` and whose task cannot be
  resolved, **WHEN** it is evaluated, **THEN** no automation fires.
- **GIVEN** a merged PR linked to a task whose origin is `automation_run`, **WHEN** the
  event is evaluated, **THEN** no `github_pr_merged` trigger fires.
- **GIVEN** a merged PR whose linked task was deleted before the event is evaluated (e.g. by
  a review watch with `cleanup_policy: auto`), **WHEN** it is evaluated, **THEN** the task
  lookup does not resolve, **no trigger is listed and no run row of any status is written** —
  not a `failed` run, not a `skipped` run, nothing.
- **GIVEN** no task lookup is wired, **WHEN** any merged-PR event arrives, **THEN** no
  `github_pr_merged` trigger fires.
- **GIVEN** the subscriber starts before a task lookup is wired, **WHEN** the lookup is
  installed later and a subsequent valid event arrives, **THEN** that event is evaluated
  and can fire without restarting or re-subscribing the subscriber.
- **GIVEN** no task lookup is wired, **WHEN** the automation bus subscriber's `Start` runs,
  **THEN** it emits exactly **one** log line at **`warn`** naming `github_pr_merged` and
  stating the type will not fire. Asserted on the level and the trigger-type name, because
  the fail-closed outcome alone is identical to a build that emits nothing.
- **GIVEN** a task lookup **is** wired, **WHEN** `Start` runs, **THEN** it emits a line at
  **`info`** recording that the merged-PR subscription is active, and **no** `warn` line.
  Both branches are asserted: a requirement observed only on the failure path cannot
  distinguish a correct build from one that never logs.
- **GIVEN** a wired lookup whose underlying query **errors** for a task id, **WHEN** a
  merged-PR event for that task is evaluated, **THEN** the event fails closed **and** the
  adapter logs at **`warn`** carrying the task id and the underlying error.
- **GIVEN** a wired lookup that resolves cleanly but finds **no such task**, **WHEN** a
  merged-PR event for that task id is evaluated, **THEN** the event fails closed **and** the
  adapter logs at **`debug`**, with no `warn` emitted. This scenario and the one above assert
  the split that the identical fail-closed outcome hides — a build that logs both at `debug`
  passes every other scenario in this spec.
- **GIVEN** a `github_pr_merged` firing, **WHEN** its run task is created, **THEN** the
  merged PR is **not** associated with that run task and no `github_task_prs` row is
  created for it.
- **GIVEN** a `github_pr_merged` firing in a workspace whose automation has
  `repository_ids: [R]`, **WHEN** its run task is created, **THEN** the task is pinned to R
  on R's default branch, regardless of which repository the merged PR belonged to.
- **GIVEN** a merged-PR event whose `TaskID` is empty, **WHEN** it is evaluated, **THEN**
  no trigger fires.

Engine integration:

- The ordering is proved behaviorally, not with a source-line assertion. A narrow helper or
  fake component seam may be extracted if needed to drive the real start sequence, but it
  must preserve production ownership and must not introduce a subscriber-to-poller
  dependency.
- **GIVEN** a linked PR whose row still reads `open` while Kandev is down, **AND** the PR was
  merged during that outage, **WHEN** Kandev starts and the poller's first `checkPRWatches`
  sweep runs, **THEN** the orchestrator's automation-event subscription and the merged-PR
  subscriber are already attached, the merged-state event is delivered, and the trigger
  fires. This is the down-time recovery guarantee in
  [Persistence guarantees](#persistence-guarantees), observed end to end rather than assumed.
- **GIVEN** a matching firing whose task was created but whose `recordSuccessRun` write
  **fails**, **WHEN** the firing completes, **THEN** the created task is deleted, **no run
  row of any status exists**, and a later `github.task_pr.updated` for the same task and PR
  **does** fire and create a task — the key was never consumed because no row ever carried
  it.
- **GIVEN** an automation at `max_concurrent_runs: 1` with a run already in flight,
  **WHEN** a second PR merges and matches, **THEN** a `skipped` run row is recorded naming
  the cap, and no task is created.
- **GIVEN** a disabled automation with a matching trigger, **WHEN** a PR merges, **THEN**
  nothing fires and `last_triggered_at` does not move.
- **GIVEN** a trigger whose config JSON is malformed and a second, valid
  `github_pr_merged` trigger, **WHEN** a matching PR merges, **THEN** the valid trigger
  still fires.
- **GIVEN** two `github_pr_merged` triggers created in the same clock tick, **WHEN** an
  event is evaluated, **THEN** they are evaluated in ascending `(created_at, id)` order.
- **GIVEN one automation** carrying two `github_pr_merged` triggers that both match the same
  event (e.g. `base_branches: []` and `base_branches: ["main"]`), **WHEN** the event is
  evaluated, **THEN** exactly **one** run row is created, and its `trigger_id` is the
  **first** trigger in `(created_at ASC, id ASC)` order. This must hold without waiting on
  the asynchronous run-row write — it is the per-event fired-key set, not the persisted dedup
  check, that produces it.
- **GIVEN two different automations** in the same workspace each carrying a matching
  `github_pr_merged` trigger, **WHEN** one event is evaluated, **THEN** **both** fire — the
  fired-key set is keyed by `(automation_id, dedup_key)`, so one automation's firing never
  suppresses another's.
- **GIVEN** a firing whose run created a task, **AND** that run was subsequently moved to
  `failed` by `MarkRunFailedByTaskID` (auto-start aborted or the agent's turn errored),
  **WHEN** a later `github.task_pr.updated` arrives for the same task and PR, **THEN** the
  trigger does **not** fire again and no second task is created — a run that created a task
  consumes its key regardless of the status it ends in.
- **GIVEN** an automation at `max_concurrent_runs: 1` still at its cap, **WHEN** three
  further matching events arrive for the same PR, **THEN** three additional `skipped` rows
  are recorded — repeats are not collapsed — and each carries an empty `dedup_key`.
- **GIVEN** the trigger-type registry, **WHEN** it is read, **THEN** it has exactly six
  entries whose `Type` values in array order are `scheduled`, `github_pr`,
  `github_pr_merged`, `github_push`, `github_ci`, `webhook` — pinning both membership and
  the index-2 position of the new entry.
- **GIVEN** an enabled automation whose `github_pr_merged` trigger row has
  `enabled = false`, **WHEN** a matching PR merges, **THEN** nothing fires.
- **GIVEN** two enabled automations in the same workspace, each with a matching
  `github_pr_merged` trigger, **WHEN** one PR merges, **THEN** both fire, each recording
  its own run row.

Agent behavior:

These assert what the *system* does around the agent, not what a language model chooses to
do. They are exercised against the **mock agent** (`apps/backend/cmd/mock-agent`). Naming the
harness is part of the contract: without it these read as manual QA notes and get dropped.
None of them is a test of a real model's judgement — that is explicitly
[out of scope](#out-of-scope).

**The harness form is the inline MCP script, NOT a `/e2e:<name>` scenario.** This matters
enough to specify, because the obvious choice does not work. A `/e2e:<name>` prompt routes
into `scenarioRegistry`, which is `map[string]func(e *emitter)` — a registered scenario
receives no prompt text and therefore can never read the interpolated `{{data.task_id}}`;
and reaching one at all requires the prompt to *start with* `/e2e:`. The form that works is
the mock agent's inline script (`script.go`), where each line is a directive and
`e2e:mcp:<server>:<tool>(<json_args>)` performs a real MCP call. So these scenarios set the
automation's prompt to, for example:

```text
e2e:mcp:kandev:archive_task_kandev({"task_id":"{{data.task_id}}"})
```

The engine interpolates `{{data.task_id}}` before the agent ever sees the line, so the call
really does carry the id the trigger resolved — which is the thing being asserted. The
absence-of-a-call scenarios use a script with no `e2e:mcp:` line.

**Consequence, stated so nobody double-counts the coverage:** because the prompt is replaced
by the script, these scenarios do **not** exercise the pinned `default_prompt`. That text is
covered separately and mechanically by the exact-match registry assertion in the
[Data-map and prompt contract](#scenarios) group. Neither group substitutes for the other.

- **GIVEN** a run task launched by this trigger and a scripted agent that archives the id it
  is given, **WHEN** the run executes, **THEN** `archive_task_kandev` is called with the id
  from `{{data.task_id}}` and the target task becomes archived.
- **GIVEN** a run task launched by this trigger and a scripted agent that supplies a
  different task id reachable by the same owner, **WHEN** it calls `archive_task_kandev`,
  **THEN** the backend rejects the request and neither task is mutated.
- **GIVEN** a `github_pr_merged` automation run whose target-binding metadata is absent or
  malformed, **WHEN** it calls `archive_task_kandev`, **THEN** the backend fails closed and
  archives nothing.
- **GIVEN** an ordinary task session or an automation run from another trigger type,
  **WHEN** it calls `archive_task_kandev` for an owner-authorized target, **THEN** its
  existing behavior is unchanged.
- **GIVEN** the target task is already archived, **WHEN** the scripted agent calls
  `archive_task_kandev`, **THEN** the call succeeds with `already_archived: true` and the
  run is recorded as succeeded.
- **GIVEN** the target task has been deleted, **WHEN** the scripted agent calls
  `archive_task_kandev` and the call fails, **THEN** the run is still recorded as
  **succeeded** provided the turn ended cleanly, and no other task is archived — run status
  reflects the turn, never the archive.
- **GIVEN** a run task launched by this trigger, **WHEN** the scripted agent ends its turn
  without calling `archive_task_kandev`, **THEN** the run is still recorded as succeeded and
  the target task remains unarchived.
- **GIVEN** an automation carrying this trigger, **WHEN** the operator clicks Run manually,
  **THEN** the firing carries trigger type `manual` with no `task_id`, the interpolated
  prompt contains an empty id, and the scripted agent makes **no** `archive_task_kandev`
  call.

Editor:

- **GIVEN** the automation editor's condition picker, **WHEN** the user opens it, **THEN**
  "Pull request merged" appears under the GitHub group and is selectable.
- **GIVEN** the condition is selected, **WHEN** the user expands its card, **THEN** they
  can toggle "All repositories", pick specific repositories, and type a comma-separated
  base-branch list; and the collapsed summary line describes the condition rather than
  echoing `github_pr_merged`.
- **GIVEN** the condition is selected, **WHEN** the user looks at the repository picker in
  the configuration section, **THEN** it is **enabled** (unlike `github_pr`, which
  disables it).
- **GIVEN** the condition is selected and the prompt is still the seeded default, **WHEN**
  the user saves and reopens the automation, **THEN** the condition, its config and the
  prompt round-trip unchanged.
- **GIVEN** the condition is selected, **WHEN** the user unchecks "All repositories" and
  selects no repository, **THEN** the panel shows the inline "this condition will never
  fire" warning, and saving still succeeds.
- **GIVEN** the condition is selected, **WHEN** the user hovers its info icon, **THEN** the
  tooltip mentions both the workspace-linked-task requirement and the up-to-a-minute
  detection lag — not the placeholder "not implemented" copy.
- **GIVEN** the condition's config panel is open, **WHEN** the user unchecks "All
  repositories" and opens the repository selector, **THEN** the repository-search request the
  selector issues **carries this workspace's id** — i.e. the panel received a `workspaceId`
  and passed it down. That request is the observable, and it is what fails if the
  pass-through is missed and the panel ships inert.
  **The assertion is on the outbound REQUEST. Asserting on what the search RETURNS, or on
  the organization list, is forbidden for this scenario.** Both of those were tried in
  earlier drafts and both are conditional on fixture state rather than on the pass-through:
  `showOrgBadges = !scope || scope.repo_scope_mode !== "repos"` renders **no** org badges at
  all in a `"repos"`-scoped workspace even when `workspaceId` is threaded correctly; and
  `useRepoSearch(workspaceId, org, query)` early-returns on `if (!workspaceId || !org)
  return` while rendering `org ? results : []`, so "search returns results" additionally
  requires an org to be selected **and** the workspace to actually contain a matching
  repository the provider returns. Either form can fail against a correct build, and — worse
  — can be made to pass by seeding data instead of by fixing the pass-through. The outbound
  request is the only signal that isolates the one thing this scenario exists to catch.
  **Do NOT "repair" this by adding a fixture precondition: the precondition IS the defect.**
  Two successive attempts to fix this observable by changing *which* fixture state it depends
  on both failed review, which is why the class of assertion is pinned here rather than the
  fixture.
- **GIVEN** the condition's config panel with "All repositories" unchecked, **AND** a
  workspace whose `repo_scope_mode` is **not** `"repos"` (so organization badges render at
  all), **WHEN** the user selects an organization badge (an `owner/*` entry) and no
  individual repository, **THEN** the dead-configuration warning is **not** shown and saving
  succeeds — an organization wildcard is a live configuration. The scope precondition is
  named because without it this scenario is not reachable in a `"repos"`-scoped workspace,
  and a builder would be left debugging a missing badge rather than the warning predicate.
- **GIVEN** an automation carrying this condition, **WHEN** the automations **list page**
  renders it, **THEN** its badge shows the localized "GitHub PR Merged" label in the same
  purple styling as the other GitHub types — not the raw type id.
- **GIVEN** the condition's config panel with "All repositories" **unchecked** and one repository
  selected, **WHEN** the user checks "All repositories", saves, reopens the automation and
  unchecks it again, **THEN** `repos` is **empty** and the dead-configuration warning **is**
  shown — checking the toggle cleared the list, per [Editor surface](#editor-surface). This is the
  observable for the clear-versus-preserve rule; asserting only that the toggle round-trips would
  pass under either behaviour.
- **GIVEN** a stored trigger whose config is `{}`, **WHEN** the user opens its card, **THEN**
  "All repositories" renders **unchecked** and the dead-configuration warning **is** shown —
  the panel's absent-field default is `false`, matching the backend, so the editor never
  claims a condition matches everything when the backend matches nothing.
- **GIVEN** the condition picker, **WHEN** the GitHub group is rendered, **THEN** "Pull
  request merged" appears immediately after "New pull requests" and before "Push to branch"
   — the adjacency, not merely membership.
- **GIVEN** an automation carrying **both** a `github_pr` trigger and a `github_pr_merged`
  trigger, with `github_pr` first, **WHEN** the configuration section renders, **THEN** the
  repository picker is **disabled** — the resolved condition is `github_pr`. This asserts the
  accepted gap recorded under [Editor surface](#editor-surface), so that the behaviour is
  pinned rather than rediscovered as a bug.
- All new user-facing copy in the editor goes through `t()`. Where the copy comes from is
  split, and both halves must be provided: the **condition picker** renders the trigger's
  label and description from the **backend registry** (English strings authored on the
  backend, as for every other trigger type), while the **list-page badge** renders a
  **frontend** i18n key, `automations:triggerLabelGithubPrMerged`. Neither substitutes for
  the other; a build that adds only the registry entry does not compile.
- **GIVEN** the mobile Chrome settings flow, **WHEN** the user selects this trigger, changes
  repository and base-branch settings, saves, and reopens it, **THEN** the configuration
  round-trips using the same state as desktop, the page has no horizontal overflow, and all
  controls remain visible and touch-operable.
