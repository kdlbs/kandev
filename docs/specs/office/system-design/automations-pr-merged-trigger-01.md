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
# Automations — "Pull request merged" trigger System Design Part 1

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

## Why

A task whose pull request has merged is finished, but it stays on the board until
somebody notices and archives it by hand. Kandev already knows the PR merged — it
polls every linked PR once a minute — but the Automations engine has no condition
that reacts to it, so the one piece of tidying every user does after every merge is
the one thing they cannot automate.

**What already exists, and why it is not enough.** Kandev does react to a merge in two
narrower places, and both were considered before adding an engine condition:

- `TaskCIOptions.PromptOnMerged` prompts a task's own agent when its PR reaches a terminal
  state. It is per-task, opt-in per task, and prompts rather than tidies — it does not give
  a workspace a standing rule.
- A **review watch** with `cleanup_policy: auto` — the default — **deletes** its
  auto-created task once the PR is merged or closed, unless the user wrote at least one
  message in it. That covers only tasks the watch itself created, and it deletes rather
  than archives.

Extending either one was rejected: `prompt_on_merged` is a per-task setting rather than a
workspace rule, and cleanup policy only governs watch-created tasks. Neither can express
"in this workspace, when a PR merges, archive its task", which is what users are asking
for. This is recorded so the alternative is not silently re-proposed.

That second mechanism also **interacts** with this feature: for a watch-created task with
`cleanup_policy: auto`, the watch may delete the task around the same time this automation
tries to archive it. [Failure modes](#failure-modes) carries the row.

## What

- The Automations engine gains a **sixth** trigger type, **`github_pr_merged`**, labelled
  **"Pull request merged"** in the condition picker, in the same `github` category as
  the existing GitHub conditions. `scheduled`, `github_pr`, `github_push`, `github_ci` and
  `webhook` already exist, so afterwards there are **6** `TriggerType` constants and **6**
  registry entries. The condition picker filters out the `schedule` category, so it shows
  **5** entries after this change rather than 6.
- **Count and position are different facts, and both are pinned.** The new entry is the 6th
  by *count* but is inserted at **array index 2** in `triggerTypeRegistry`, between
  `github_pr` and `github_push` — see [Editor surface](#editor-surface) for why position
  matters. Appending it instead still yields 6 entries and still compiles; it simply renders
  in the wrong place, which is why an adjacency scenario exists rather than only a count.
- **The registry test that pins these counts does not exist yet and is part of this change.**
  No test in `apps/backend` currently reads `GetTriggerTypes()` or `triggerTypeRegistry` for
  length or membership, so today a row can be added or lost silently. This spec requires
  adding one; it is specified under [API surface](#trigger-type-registry) and observed by a
  scenario. Without it the counts above are prose nobody checks.
- The trigger fires when Kandev observes that a pull request **linked to a task in the
  automation's workspace** has reached the `merged` state. Detection rides the PR
  poller that already runs (`github.task_pr.updated`), so the expected latency between
  the merge on GitHub and the firing is **under ~60 seconds**, not instant.
- The trigger's configuration is a repository filter and an optional base-branch
  filter. Nothing else.
- The firing carries the **id of the task the merged PR was linked to** into the
  trigger data, reachable in the prompt and title templates as `{{data.task_id}}`.
- The automation's default prompt instructs the spawned agent to archive exactly that
  task through the `archive_task_kandev` MCP tool. Archiving stays an agent action and
  this feature adds no native action type, but the target is enforced structurally: the
  run task persists the validated event target and the backend rejects an archive request
  for any other task.
- Because the archive is performed by an LLM reading a prompt, the trigger data is
  deliberately **narrow**: it carries identifiers and single-token git/GitHub values
  only, and never PR title, PR body, or author login. See
  [Prompt-injection surface](#prompt-injection-surface).
- The trigger MUST NOT fire for a task that is itself an automation run
  (`origin = automation_run`). This is a loop guard, not a filter — see
  [Loop safety](#loop-safety).
- Everything else about the automation — agent profile, executor profile, repository
  selection, `max_concurrent_runs`, the run log, the hidden run task, retention — is
  unchanged and inherits the behavior specified in
  [`office/automations-settings.md`](../requirements/automations-settings.md) and
  [`office/automation-runs.md`](../requirements/automation-runs.md).

### Prompt-injection surface

`archive_task_kandev` keeps its normal owner authorization, but a `github_pr_merged`
automation run has an additional target-binding check. The validated event `task_id` is
persisted as server-owned metadata on the run task. The in-session MCP server injects the
caller run-task id into the backend request without exposing it as a tool argument, and the
backend rejects a missing, malformed, or mismatched target before mutation. Correctness
therefore does not depend on the model copying the id faithfully. The narrow data and prompt
still reduce accidental behavior and transcript noise:

1. **The trigger data carries no free-form prose.** `pr_title`, PR body, `author_login`
   and `head_branch` are deliberately absent. Every field that *is* present is either a
   Kandev-internal identifier (`task_id`), a value constrained by git/GitHub to a single
   whitespace-free token (`repo`, `base_branch`, `pr_number`), a URL (`pr_url`), or a
   timestamp (`merged_at`).
   `pr_url` is GitHub's `html_url` stored verbatim — it is **upstream-supplied, not
   Kandev-constructed**. It is included anyway because a URL cannot carry a newline and
   GitHub controls the host, so it is not a prose channel; but the injection argument
   rests on its *shape*, not on Kandev having authored it. When the stored value is
   empty, `{{data.pr_url}}` renders as an empty string and nothing else changes.
2. **The default prompt is narrowly scoped**, and its exact text is pinned in
   [API surface](#trigger-type-registry) rather than described, so that a change which
   weakens it fails a test instead of passing quietly. It names one task id, states that no
   other task may be archived, states that no other source may be consulted to decide what
   to archive, states that text encountered during the turn is data rather than
   instruction, and tells the agent to do nothing when the task id is empty.
3. The user may edit the prompt, and may reference `{{data.*}}` freely. Editing it cannot
   expand the task that this trigger's run is allowed to archive; a wrong target produces a
   non-mutating tool error.

### Loop safety

If the run task created by a `github_pr_merged` firing were itself linked to a pull
request, the merge of that PR would publish another `github.task_pr.updated`, match the
same trigger with a *different* `task_id` (so dedup would not suppress it), and create
another run task — indefinitely. Two rules close this:

- A firing of this trigger type MUST NOT associate the merged PR with its run task.
  (The existing PR association step is gated on `github_pr` and MUST stay so.)
- The trigger MUST NOT fire for a task whose origin is `automation_run`, regardless of
  how that task acquired a linked PR. Automation run tasks are hidden from the board, so
  archiving one is meaningless; giving this up costs nothing and removes the whole class.

## Data model

No new tables. One new value in an existing enumeration, one new trigger-config shape, and
one server-owned metadata value on the ordinary automation-run task.

`automation_triggers.type` gains the value `github_pr_merged`. Existing rows are
untouched; no migration is required.

When the orchestrator creates a run task for this trigger, it persists the validated
`trigger_data.task_id` under the stable metadata key `automation_target_task_id`. This value is the
backend enforcement source for later archive calls. It is not derived again from the
rendered prompt, and it is not accepted from an agent-controlled tool argument. Manual runs
and other trigger types do not set this target binding.

The trigger's `config` column holds:

```
GitHubPRMergedTriggerConfig            (JSON in automation_triggers.config)
  all_repos      bool      when true, every repository matches and `repos` is ignored
  repos          []RepoFilter  {owner: string, name: string}; consulted only when all_repos is false
  base_branches  []string  glob patterns matched against the PR's base branch; empty = every base branch
```

`RepoFilter` is the existing `github.RepoFilter` shape already used by `github_pr`,
`github_push` and `github_ci` configs.

Repository-matching semantics, stated as a contract because they differ from the other
GitHub trigger types:

| `all_repos` | `repos` | Result |
|---|---|---|
| `true` | anything | every repository matches |
| `false` | non-empty | matches only the listed entries, per the entry table below |
| `false` | empty | **matches nothing** — the trigger never fires |
| absent (`{}`) | absent | `all_repos` defaults to `false`, so nothing matches |

Each entry in `repos` is matched by this table. The `name: ""` form is not a curiosity —
it is what the shared repository-filter selector writes when the user clicks an
organization badge, and it renders in that UI as `owner/*`:

| Entry | Meaning |
|---|---|
| `{owner: "acme", name: "api"}` | matches exactly that repository, case-insensitively |
| `{owner: "acme", name: ""}` | **organization wildcard** — matches every repository whose owner is `acme`, case-insensitively |
| `{owner: "", name: anything}` | matches nothing; an entry with no owner cannot be resolved |

The organization-wildcard row exists because the selector can already produce that shape.
Leaving it undefined would let one click on an org badge save a configuration that silently
matches nothing while the dead-configuration hint stays quiet (`repos` is non-empty), which
is the worst of both outcomes.

`all_repos` is a real backend field here, unlike `github_pr` where the same key exists
only in the editor's draft and the backend ignores it. The reason is that "empty list"
cannot carry the intent for this trigger type: for `github_pr` an empty list means "never
poll", which fails safe, whereas an "empty list means all" rule would make an
accidentally-empty config fire on every repository in the workspace. An explicit flag
removes the ambiguity, and the absent-field default is the closed one.

Repository comparison for this type is **case-insensitive** on both owner and name. The
engine's existing `matchesRepo` helper compares exactly and treats an empty list as "no
match"; neither rule fits here, so this type uses its own comparison rather than reusing
that helper. `matchesRepo` itself MUST NOT be changed — `github_push` and `github_ci`
depend on its empty-list-never-matches rule.

Base-branch matching uses the engine's existing glob helper: exact match, `*`, or a
single trailing `*` prefix match (`release/*`). Entries are trimmed of surrounding
whitespace before matching, and entries that are empty after trimming are dropped. A
list that is empty — originally, or after dropping empties — matches every base branch.

**An empty base branch on the row** is reachable: the lifecycle reconcile path writes only
`state`, `merged_at` and `closed_at`, so a partially-synced or legacy row can carry
`base_branch = ""`. No special case is introduced for it — it is matched by the same helper
as any other value, which yields exactly this:

- `base_branches` empty → matches (the match-all case);
- `base_branches: ["*"]` → matches, because `*` matches any value including the empty one;
- `base_branches: ["main"]` or any other literal → does **not** match, so the trigger does
  not fire.

In every case where it does fire, `{{data.base_branch}}` renders as an empty string. An
empty base branch is deliberately **not** a fail-closed gate the way an empty owner or repo
is: the base branch is a filter input, not an identity, and a user who set no filter has
asked for every branch.
