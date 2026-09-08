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
# Automations — "Pull request merged" trigger System Design Part 8

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-PR-MERGED-TRIGGER-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

- **GIVEN** assistive technology focuses the base-branch field, **WHEN** its accessible name
  is computed, **THEN** it is associated with the localized base-branch label.

## Out of scope

- **No GitHub App webhook changes.** No `pull_request` webhook subscription is added to
  the App manifest. Doing so would require every existing installation to reinstall and
  re-consent. The ~60s poller latency is accepted in exchange, and is stated in the
  condition's own description so users are not surprised by it.
- **No native "archive" action type.** The engine's only action stays "create and start a
  task". Archiving remains an agent MCP call. The backend mutation boundary enforces the
  event-selected target, so deterministic safety does not require a second action model.
- **No change to the existing `github_pr` trigger's behavior, with ONE named carve-out.**
  Its open-PR polling, its `all_repos` editor-only key, and its repository-picker
  disablement are untouched, and it continues to receive no `workspaceId` so its selector
  stays exactly as inert as it is today. This is a promise about *behavior*, not a freeze on
  every file it touches: adding a branch to the shared per-type config dispatcher is
  permitted and changes nothing for `github_pr`. See
  [the scope note](#scope-note-the-shared-repository-filter-selector).
  **The carve-out:** the `ORDER BY t.created_at ASC, t.id ASC` tiebreak in
  [Ordering](#ordering) lands on the shared `ListEnabledTriggersByType` query and therefore
  also affects the `github_pr` and `scheduled` listings. That is permitted and intended. It
  changes no observable `github_pr` behavior — it cannot alter which triggers are listed or
  how they evaluate, only the order of an already-unordered tie, and `github_pr` triggers are
  evaluated independently of one another so tie order was never outcome-bearing for them.
  The carve-out is named here so that this bullet and Ordering § cannot be read as
  contradicting each other, and so nobody "fixes" the contradiction by scoping the tiebreak
  per trigger type — which Ordering § forbids.
- **No forking of the shared repository-filter selector**, and no modification of it. The
  organization-wildcard shape it already emits is given meaning in this spec's matching
  rules instead.
- **No persisted retry queue for firings that could not run.** When a firing is cap-skipped,
  fails before creating a task, or has its task rolled back by a failed run-row write, the
  dedup key is left unconsumed and the firing is retried only if another
  `github.task_pr.updated` happens to arrive for that row. Nothing durably remembers "this
  merge still needs archiving". Building that would mean a new table and a sweeper, which is
  a larger change than this trigger type; the residual is recorded under
  [Idempotency](#which-runs-consume-the-dedup-key) instead. It is also the correct fix if the
  cap-skip noise or the unretried-merge residual ever needs tightening — not storing the key
  on the rows the consume rule needs keyless.
- **No runtime dependency between the subscriber and poller.** Start-up order is expressed
  by backend composition and proved through the down-time-recovery behavior; the subscriber
  does not gain a reference to the GitHub poller or a production-only readiness assertion.
- **No per-row observation checkpoint.** Distinguishing "this PR just merged" from "this PR
  was already merged when we first saw it" would require persisting a prior-state marker per
  linked PR. That is not built; the consequences are stated under
  [Retroactivity](#retroactivity-and-first-observation-semantics) instead.
- **No test of a real model's judgement.** The agent-behavior scenarios run against the
  scripted mock agent. Whether a frontier model can be induced to archive the wrong task is
  not something this spec's tests answer. The backend target binding makes that judgment
  irrelevant to which task can be mutated; the narrow data map and pinned prompt still make
  the intended action clear.
- **No change to `HasRunWithDedupKey` at all.** Not its signature, not its query, not its
  meaning for any trigger type. The consume rule is implemented on the **write** side by
  controlling what goes into `automation_runs.dedup_key`, so `github_push` and `github_ci`
  keep counting runs of any status and keep writing their key on every row. See
  [Idempotency](#which-runs-consume-the-dedup-key).
- **No multi-condition awareness in the editor.** `getConditionType()` resolves exactly one
  condition per automation and everything the editor derives — placeholders, default title,
  repository-picker state — follows from that one answer. An automation carrying both
  `github_pr` and `github_pr_merged` therefore renders for whichever comes first. This is
  pre-existing behavior; making the editor handle several conditions at once is a separate
  change with its own surface, and a scenario pins the current outcome so it cannot regress
  silently.
- **No collapsing of repeated cap-skip rows.** Several `skipped` rows for one pull request
  are an accepted outcome; deduplicating them would require storing the key on exactly the
  rows the consume rule needs keyless.
- **No new trigger placeholders in the interpolator.** This type uses `{{data.*}}` only;
  no `{{merged.*}}` prefix family is introduced.
- **No "PR closed without merging" condition.** Only the merged transition is covered.
- **No BACKFILL SWEEP for pull requests already merged when the trigger is created.**
  Creating or enabling a trigger publishes no event and starts no scan, so nothing fires at
  creation time. **This is a promise about the sweep, NOT about the pull requests.** An
  already-merged PR is not permanently excluded: when some later sync republishes its row —
  which per [the publish-path table](#which-publish-paths-touch-a-merged-row) is ordinary
  rather than rare — the trigger fires then, and that is intended behaviour, not a leak past
  this exclusion. So enabling this condition in a workspace with recently-merged linked PRs
  **can archive some of them** as their rows next settle. The authority for that behaviour is
  [Retroactivity](#retroactivity-and-first-observation-semantics), third bullet, which three
  scenarios pin; this bullet excludes only the sweep.
  The wording is spelled out this carefully because the shorter form ("no re-firing for pull
  requests already merged") reads as suppression, and suppression is **not** what happens and
  **must not** be built: it would require the per-row observation checkpoint excluded in the
  very next bullet, so a builder who implemented it would be stuck between two out-of-scope
  bullets.
- **No global restriction on the generic archive tool.** Target binding applies only when
  the caller is a `github_pr_merged` automation run with an authoritative event target.
  Other callers keep their existing owner-authorized behavior.
- **No per-trigger author, label or draft filters.** A merged PR is a merged PR; the
  repository and base branch are the only axes offered.
