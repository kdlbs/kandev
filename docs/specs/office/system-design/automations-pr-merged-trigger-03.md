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
# Automations — "Pull request merged" trigger System Design Part 3

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## State machine

The trigger itself is stateless. One event is evaluated and either fires some triggers or
none.

**Gate cardinality is part of the contract**, because it decides how many task reads a busy
workspace does per merge. The gates split into two groups:

**The cost in an install with ZERO `github_pr_merged` triggers is accepted, and the gate
order MUST NOT be rearranged to avoid it.** Gate 6 does one task read per merged-state
event even when no automation anywhere has this condition — and per
[the publish-path table](#which-publish-paths-touch-a-merged-row)
merged rows keep republishing, so that is not a one-off cost. It is accepted for three
reasons: it is a single indexed primary-key read against the task the event already names;
it happens only on events that already passed gates 1–5, i.e. genuinely merged rows with a
task id and a valid repository identity; and reordering to list triggers first would trade
it for a table query on **every** such event, which is not obviously cheaper and is
certainly not simpler. A builder who "optimises" by listing triggers before gate 6 is
violating a pinned contract, not improving it. If this ever needs changing it is a spec
change, because the gate numbering and the once-per-event lookup guarantee are both
observable and both scenario-covered.

**Per event, evaluated ONCE, before any trigger is listed.** If any of these fails, the
event is dropped and **no trigger is evaluated or listed at all** — the trigger table is
never queried:

1. **Payload** — the event data normalizes from either a non-nil typed `*github.TaskPR` or
   its NATS JSON-decoded object representation into a valid `github.TaskPR`.
2. **Task id present** — `TaskID` is non-empty. There is nothing to archive otherwise.
3. **Merged** — `State`, trimmed and compared case-insensitively, equals `merged`.
   `MergedAt` is deliberately **not** part of this test: some sync paths persist the
   state without the timestamp, and gating on the timestamp would silently drop real
   merges. A merged row with a nil `MergedAt` fires with `merged_at` rendering as `""`.
   This gate tests the row's *current* state, not a transition — see
   [Retroactivity](#retroactivity-and-first-observation-semantics).
4. **Repository identity present** — `Owner` and `Repo` are both non-empty. A row missing
   either cannot be matched against a filter and cannot produce a meaningful `repo`
   value, so it fails closed.
5. **Pull request number valid** — `PRNumber` is greater than zero. A non-positive number
   cannot identify a pull request; it would otherwise flow into the dedup key as `#0`, into
   the default task title, and into the prompt. Fails closed.
6. **Task resolvable** — the task lookup returns `ok`. The lookup is called **once per
   event**, and its result is reused for every trigger evaluated below. A workspace with
   twenty enabled triggers does one task read per merge, not twenty.
7. **Not an automation run** — the lookup reports `isAutomationRun == false`. This is the
   loop guard from [Loop safety](#loop-safety).

**Per trigger**, for each enabled `github_pr_merged` trigger in `(created_at ASC, id ASC)`
order. The first failure ends evaluation for that trigger only; the remaining triggers are
still evaluated, and nothing is recorded for a trigger that does not fire:

8. **Workspace** — the workspace returned by the lookup equals the automation's workspace.
   `TaskPR.WorkspaceID` is not consulted; see [Task lookup](#task-lookup) for why. When the
   lookup's workspace is empty, or the automation's is empty, the trigger does not fire.
9. **Repository** — per the entry table in [Data model](#data-model).
10. **Base branch** — the PR's base branch matches `base_branches`, or the list is empty.
11. **Not already fired for this automation during this event** — the trigger's
    `(automation_id, dedup_key)` pair is not in the per-event fired-key set described under
    [Ordering](#ordering). If it is, the trigger is skipped **without** calling
    `FireTrigger`, and nothing is recorded. On passing, the pair is added to the set before
    `FireTrigger` is called, **and it is NOT removed if `FireTrigger` then returns an
    error** — so when the first matching trigger on an automation fails infrastructurally,
    every later trigger on that same automation is skipped for this event too and the event
    produces nothing at all. That is deliberate and it follows the Failure-modes rule that a
    `FireTrigger` error is "not retried within this event": the alternative — dropping the
    pair so a sibling trigger can try again — would turn one dedup-query or cap-count blip
    into a second attempt whose only distinguishing feature is a different `trigger_id`, on
    an automation that has already shown it cannot admit a run right now. Stated explicitly
    because the retention is otherwise only inferable by composing two sections.
    This gate is what makes "exactly one run per automation per
    event" true; the persisted dedup check cannot do it, because the run row is written
    asynchronously and does not exist yet.

`PRURL` is deliberately **not** gated. It is carried into the data map for the prompt's
benefit only, is never used to identify anything, and an empty value renders as an empty
string.

A trigger that passes all eleven calls the engine's normal `FireTrigger` path, which then
applies the engine's own existing admission rules in this order: automation exists →
automation enabled → dedup key unseen → `max_concurrent_runs` not reached. Those rules are
unchanged by this spec, and `HasRunWithDedupKey` in particular keeps its exact current
behavior. What changes is only *which rows carry a key to be found*: per
[Idempotency](#which-runs-consume-the-dedup-key), a `github_pr_merged` run row is written
with its dedup key only when a task was created, so for this trigger type "dedup key unseen"
resolves to "no run that actually created a task has used this key" without the query itself
being touched.

Further rules that follow from existing engine behavior, restated because a conformance
test needs them:

- A trigger row with `enabled = false`, or one whose automation has `enabled = false`,
  is never listed and therefore never evaluated.
- **The listing is a snapshot, and trigger rows are NOT re-read before firing.** The bullet
  above is a statement about *listing time* only. Between the listing and the `FireTrigger`
  call, a trigger row can be disabled, deleted or reconfigured by a concurrent user edit, and
  none of that is re-checked: `FireTrigger` reloads the **automation** (and returns early
  when it is missing or disabled) but never re-reads the trigger row or its `config`. So a
  trigger disabled mid-event still fires once, on its listing-time config. This is
  deliberate, not an oversight: the window is microseconds, the archive is idempotent, and
  re-reading every trigger row inside the loop would add a query per trigger to buy nothing
  a user could notice. Stated because every other gate in this section is explicit, so
  silence here would read as an omission a builder has to resolve by guessing.
- An automation whose `workspace_id` is empty never matches, because gate 8 requires a
  non-empty workspace on both sides.
- **An already-archived target task does not suppress the firing.** There is no
  `archived` gate: the lookup reports workspace and automation-origin only, the trigger
  fires, and the agent's `archive_task_kandev` call returns `already_archived: true`. Adding
  a gate was considered and rejected — it would need a third fact from the lookup to avoid
  one cheap no-op run, and with the do-not-consume rule in
  [Idempotency](#which-runs-consume-the-dedup-key) a no-op firing that
  occupies the concurrency slot no longer costs a real merge its only chance to fire.
- Two different automations in the same workspace that both match one event each fire
  independently, each with its own run row and its own dedup key namespace (dedup is
  scoped per automation).
- An automation may carry a `github_pr_merged` trigger alongside triggers of other types;
  the engine already supports several triggers per automation and this spec does not
  change that. All of them share the automation's single prompt, so a `{{data.task_id}}`
  reference resolves for a merged-PR firing and is stripped to nothing for a firing of any
  other type.
- **The editor's "which trigger is *the* condition" rule, stated exactly**, because a
  requirement below depends on it: `getConditionType()` returns the first trigger whose type
  is neither `scheduled` **nor** `webhook` — it is *not* simply "the first trigger". That
  single answer seeds the placeholders, the default task title, and the repository picker's
  enabled/disabled state. Making the editor genuinely multi-condition-aware is
  [out of scope](#out-of-scope), and this spec does not change the rule.

### Run task shape

A firing produces the engine's ordinary run task: hidden by `origin = automation_run`,
auto-started, repliable, finalized on turn completion, worktree subject to the standard
retention window.

Its run status is the agent turn's outcome, exactly as for every other trigger type — it
reports whether the agent's turn ended cleanly, **not** whether the target task was
actually archived. A run can therefore read `succeeded` while nothing was archived, if
the agent chose not to call the tool. That is the accepted cost of the LLM-driven design
recorded under [Out of scope](#out-of-scope); the run's transcript is the evidence.

This rule is absolute and it governs every row in [Failure modes](#failure-modes): a failed
`archive_task_kandev` call inside a turn that then ends cleanly produces a **`succeeded`**
run, because nothing propagates a tool error into run finalization. There is no case in
which the archive's outcome sets the run's status. A run reads `failed` only when the
firing itself failed before the agent ran (no repository available, task creation failed)
or the agent's turn errored.

Its repository is resolved through the engine's **default** path — the automation's
configured `repository_ids` when set, otherwise the workspace's first repository, each
pinned to its own default branch. It MUST NOT be resolved from the merged PR's
repository. Two reasons: the merged head branch is usually deleted immediately after the
merge, so checking it out fails; and the run's only job is one MCP call, which needs no
particular checkout. Consequently the editor's repository picker stays **enabled** for
this condition — the existing "the PR decides the repository" disablement is specific to
`github_pr` and MUST NOT be widened to this type.

## Editor surface

The condition picker and the trigger card are driven partly by the backend registry and
partly by per-type frontend tables. What the registry supplies (label, description,
placeholders, defaults) is listed under [API surface](#api-surface). The frontend must
additionally provide, for `github_pr_merged`:

- **Type union** — the frontend's `TriggerType` union gains `"github_pr_merged"`.
- **Exhaustive per-type tables** — five `Record<TriggerType, …>` maps are compiler-enforced
  and the build does not compile until each has an entry. They live in **two** files, and
  missing the second file is the likely build break:
  - in the trigger card: `TRIGGER_ICON`, `TRIGGER_COLOR`, `TRIGGER_INFO_KEYS`;
  - in the automations **list page** table: `TRIGGER_BADGE_VARIANT`, `TRIGGER_LABEL_KEYS`.
  A sixth map, the trigger card's collapsed-summary table, is `Partial<…>` and therefore
  compiles without an entry — which is exactly why one is required below.
- **Two things the compiler will NOT catch, so both are required explicitly.** The five maps
  above fail the build when an entry is missing; these two fail silently and ship a
  wrong-looking UI:
  - the `Partial<…>` collapsed-summary table, which falls through to rendering the raw type
    id `github_pr_merged` at the user;
  - **the per-type config dispatcher.** `TriggerConfigForm`'s `switch` carries a `default:`
    branch that renders `automations:unknownTriggerType`, so omitting the new panel
    **compiles cleanly** and the condition's card renders an "unknown trigger type" message
    instead of its configuration. This is the same trap as the partial map one level up, and
    it is called out for the same reason: a builder who trusts the compiler to enumerate the
    work will miss it. A scenario covers the panel, so this is a missing warning rather than
    a missing observation — but the warning belongs here, next to the maps it is easily
    confused with.
- **Picker position** — the entry appears in the GitHub group immediately after
  "New pull requests". `trigger-picker.tsx` filters out `category !== "schedule"` and then
  groups by category **preserving `triggerTypeRegistry`'s array order**, so the picker has no
  ordering of its own: position is decided entirely by where the entry sits in the backend
  array. Achieving "immediately after New pull requests" therefore means inserting at
  **array index 2**, between the `github_pr` and `github_push` entries — not appending. A
  scenario asserts the adjacency, because a count-only assertion passes either way.
- **Icon and colour** — `IconBrandGithub` with `text-purple-400`, the same pair the other
  three GitHub conditions use, so the group reads as one family. Named concretely because
  "the same colour the others use" is not something a test can assert.
- **List-page badge** — `TRIGGER_BADGE_VARIANT` gets the same purple classes the other
  GitHub types use, and `TRIGGER_LABEL_KEYS` gets a **new** frontend i18n key,
  `automations:triggerLabelGithubPrMerged`, with the English value **"GitHub PR Merged"**
  (matching the existing "GitHub PR" / "GitHub Push" / "GitHub CI" pattern).
- **Collapsed summary** — a localized static line, "Pull request merged". The trigger
  card's summary table is partial, so a missing entry silently renders the raw type id
  `github_pr_merged` at the user; this entry is required, not cosmetic.
- **Info tooltip** — localized copy that states (a) the PR must be linked to a task in
  this workspace, and (b) detection is poller-driven and can lag the merge by up to a
  minute. The placeholder "not implemented yet" copy used by the push and CI conditions
  MUST NOT be reused.
- **Config panel** — an "All repositories" toggle bound to `all_repos`, the shared
  repository-filter selector bound to `repos`, and a comma-separated text input bound to
  `base_branches`.
- **Checking "All repositories" CLEARS `repos`. Unchecking it leaves `repos` exactly as it is.**
  The panel writes `{...config, all_repos: checked, repos: checked ? [] : repos}` — the same shape
  `GitHubPRConfig` already uses, so the two panels agree here even though they deliberately
  disagree about the absent-field default below. Stated because it is **observable and was
  otherwise a coin flip**: check → save → reopen → uncheck ends either with an empty list and the
  dead-configuration warning showing (clearing) or with the old list back and no warning
  (preserving), and no requirement or scenario distinguished the two, so the warning — which *is*
  required — would fire or not depending on which a builder guessed. A scenario now asserts the
  cycle.
  Clearing is the chosen rule for two reasons. With `all_repos: true` the backend ignores `repos`
  entirely (see [Data model](#data-model)), so a preserved list is stored bytes that match nothing
  and mean nothing; and for this type `all_repos` is a **real backend field** rather than the
  editor-only draft key it is for `github_pr` (see [Data model](#data-model)), so those bytes are
  actually persisted rather than dropped on save. The cost to a user is stated rather than hidden:
  toggling "All repositories" on and back off loses the previous selection, and they pick the
  repositories again.
- **The base-branch input stores what the user typed, split on commas and nothing more.**
  The panel splits the field on `,` and writes the resulting entries **verbatim** — it does
  **not** trim them and does **not** drop empty ones. Trimming and empty-dropping happen
  exactly once, in the backend at match time, per [Data model](#data-model). Stated because
  the backend half of that rule is pinned precisely and the editor half would otherwise be
  silent: two builders would store different bytes for the same keystrokes (`" main , "` →
  `[" main ", " "]` here, versus `["main"]` if the editor normalised), both of which match
  identically, so no scenario would catch the divergence. One normalisation point, and it is
  the backend — which is also why the filter scenarios can store `[" main ", ""]` and
  `["", "  "]` directly and still describe reachable states.
- **The panel's default for an absent `all_repos` is `false`, NOT `true`.** This is called
  out because the panel this one is modelled on does the opposite: `GitHubPRConfig` reads
  `(config.all_repos as boolean) ?? true`, which is right for `github_pr` (where the backend
  ignores the field entirely) and **wrong** here. The new panel MUST read it as
  `?? false`, so that the editor agrees with the backend's absent-field default in
  [Data model](#data-model). Getting this backwards produces the worst available outcome: a
  stored `{}` reopens showing "All repositories" **checked**, the dead-configuration hint
  stays quiet because its predicate never becomes true, and the user is shown a live-looking
  condition that matches nothing. A scenario asserts the `{}` case.
- **The config panel receives `workspaceId`.** The shared repository-filter selector needs
  it to list organizations and to search repositories; without it the org list and the
  repository search are inert and the user cannot pick a repository at all. Today the
  per-type config dispatcher passes `workspaceId` to the webhook panel only, so this new
  panel must be added to that pass-through. See
  [the scope note](#scope-note-the-shared-repository-filter-selector) for what this is
  allowed to touch.
- **Dead-configuration hint** — when `all_repos` is false and `repos` is empty, the panel
  shows an inline warning that the condition will never fire. Note the predicate is
  `repos` **empty**, not "no exact repository selected": an organization-wildcard entry is
  a valid, live configuration per [Data model](#data-model), so it must not trip the
  warning. The backend accepts and stores a dead configuration (see
  [Validation](#validation)); the editor's job is to make sure the user knows what they
  saved.
- **Repository picker in the configuration section stays enabled** — scoped precisely:
  **when `github_pr_merged` is the automation's resolved condition**, i.e. it is the first
  trigger that is neither `scheduled` nor `webhook`. The picker's state is derived from that
  one resolved condition (`isPRTrigger = conditionType === "github_pr"`), so this
  requirement is about the resolved condition, not about "an automation that happens to
  contain this trigger". See [Run task shape](#run-task-shape) for why it stays enabled.
- **Mixed `github_pr` + `github_pr_merged` automations are a known, accepted gap.** If both
  are present, whichever comes first decides, and when `github_pr` wins the picker renders
  **disabled** even though a `github_pr_merged` trigger is also attached. That is
  pre-existing single-condition behavior, not something this spec introduces, and correcting
  it means making the editor multi-condition-aware — [out of scope](#out-of-scope). Recorded
  so that a builder meeting it does not read it as a defect in this feature, and so the
  picker requirement above is not read as a promise the editor cannot keep.

### Scope note: the shared repository-filter selector

The repository-filter selector is shared with the `github_pr` condition, and
[Out of scope](#out-of-scope) says that condition is not to be changed. Those two statements
would collide if left as they are, so the boundary is drawn explicitly here:

- Threading `workspaceId` into the new panel is **permitted**, and is an addition to the
  per-type dispatcher rather than a change to any existing panel.
- The `github_pr` panel continues to receive **no** `workspaceId`, exactly as today. This is
  deliberate: passing it would make that condition's currently-inert org list and repository
  search suddenly live, which is a behavior change to `github_pr` and is out of scope.
- The selector component itself is **not** modified. The organization-wildcard shape it
  already emits is given meaning in this spec's matching rules rather than by changing the
  component.
- Forking the selector is **not** permitted. A second copy of a shared widget is a worse
  outcome than a scoped pass-through, and nothing here requires one.

All new copy goes through `t()` in the `automations` namespace. No i18n allowlist edit is
needed: `components/automations/*.{ts,tsx}` and
`components/automations/trigger-configs/*.tsx` are already guarded globs.

### Validation

No server-side validation is added for this trigger's config. A malformed or dead config
saves successfully and simply never fires, matching every other trigger type — cron
expressions are the sole configuration the engine parses at save time, and widening that
to per-type schema validation is a change to the engine's contract rather than to this
trigger. The editor's dead-configuration hint above is the user-facing guard.

**Yes, `{all_repos: false, repos: []}` is deliberately saveable.** Refusing it at the API
would diverge from every other trigger type for no gain, and a user mid-edit legitimately
passes through that state. What makes it safe to leave saveable is that the one outcome
worth fearing is gone: a dead configuration can no longer be confused with "this matched
once and the key was silently burned", because a cap-skip or a pre-task failure now writes a
visible `skipped` / `failed` row **and** leaves the key unconsumed for a retry, per
[Idempotency](#which-runs-consume-the-dedup-key).

**But an empty run log does not by itself prove the filter never matched**, and it would be
wrong to tell an operator that it does. These paths all match-then-write-nothing, and each is
specified in [Failure modes](#failure-modes) with a log line rather than a row:

- listing enabled triggers failed (logged at error, event dropped);
- the task lookup returned `ok == false`, whether the task is gone or the query failed;
- `FireTrigger` returned early on a dedup-query, cap-count or publish error;
- `FireTrigger` found the automation **missing or disabled** at fire time — it returns a skip
  result and writes nothing. This is the *synchronous* stage. **The two-stage split described in
  [Failure modes](#failure-modes) applies only to the MISSING case:** the orchestrator's later
  asynchronous reload writes a keyless `failed` row when the automation cannot be **loaded**, so
  that one IS visible in the run log. It does **not** apply to a disabled automation — see the
  next bullet, which is a separate path and not a restatement of this one;
- the orchestrator's later asynchronous reload found the automation **disabled** — it logs at
  `debug` and returns, writing **no run row of any status**. `recordFailedRun` is reached only
  when the automation cannot be *loaded*; an automation that loads and reads `enabled = false` is
  a plain early return. So "automation disabled" writes nothing at **either** stage, unlike
  "automation not found", which writes nothing at the first and a keyless `failed` row at the
  second. This path is reachable only in the narrow window where an automation is disabled between
  the trigger listing — which lists nothing for a disabled automation — and the task-creation
  goroutine, but it is enumerated here because this section's job is to say what an empty run log
  does and does not prove, and a case that writes nothing belongs in the list of cases that write
  nothing;
- the trigger's own `config` JSON was invalid.

So the honest reading is: **an empty run log means the trigger never fired — either because
nothing matched, or because one of the logged failures above dropped the event.** The logs
distinguish them; the run log alone does not. The editor's dead-configuration hint exists
precisely so the common case never has to be diagnosed from either.
