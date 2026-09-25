---
status: draft
system: ui
requirements:
  - REQ-UI-INBOX-HISTORY-001
---
# Inbox History System Design

This part carries the sampled input inventory and the code-level evidence behind
[`REQ-UI-INBOX-HISTORY-001`](../requirements/inbox-history.md). The requirement
holds the contract; this document holds the measurements, the citations and the
reasoning the criteria are grounded in, so a reader can check the contract rather
than trust it. One section is load-bearing for the contract rather than merely
explanatory: **Isolation mechanism** names the closed sink set AC .5 requires.

All figures were sampled from the running instance on 2026-09-14, SQLite
`mode=ro`, rather than assumed.

## Record shape

A clarification or permission request is a row in `task_session_messages` with
`type` of `clarification_request` or `permission_request`. The fields this
capability reads are all in `metadata`: `pending_id` (groups a bundle), `status`,
`question` (an object carrying `id`, `title`, `prompt` and `options[]`),
`question_index`, `question_total`, `context` (free prose, observed over 900
characters), and `agent_disconnected`. Turn identity is the column `turn_id`,
joined to `task_session_turns`.

Two further shapes are load-bearing for the requirement's AC .3 fixture, because
without them the "no fourth reason arises" test cannot construct its
counterexamples:

- `metadata.parent_question` (`models.MetaKeyParentQuestion`), the truthy marker on
  an autopilot agent's question to its PARENT task rather than to the operator.
- the per-user dismiss/snooze sidecar rows behind `ClarificationSidecarFilter`,
  which are keyed by user and not by bundle alone.

### A permission record is a different shape, and that drives three criteria

A `permission_request` carries NO `question` object. Its human-readable text is
the message's own `content`; its choices are `metadata.options[]`, whose entries
are `{kind, name, option_id}` and carry a `name`, not a `label`; it also holds
`action_type`, `action_details`, `request_id` and `tool_call_id`. It has no
`title`, no `prompt`, no `question_index` and no `context`. `buildPermissionInteraction`
(`service_interactions.go`) projects exactly that: `Title` from `row.Content`,
`Options` from `permissionOptionsFromMetadata`.

Measured over the whole table: **52 of 52 permission rows carry no resolvable
question identifier** (neither `metadata.question_id` nor nested
`metadata.question.id`), while **0 of the clarification rows lack one**. 12 of the
52 are non-terminal. Each of the 52 has a distinct `pending_id`, so a permission
request is always a bundle of one.

Three contract consequences follow, and they are why review round 3 returned
NEEDS RETHINK:

1. **AC .2 scopes `unreadable` to clarification bundles.** The `unreadable` reason
   exists to mirror the shipped `has_missing_question_id` drop, which is itself
   hard-scoped to `WHERE m.type = 'clarification_request'`. Applied
   type-agnostically it would match every permission row, so a live, current-turn,
   newest (`rn = 1`), non-terminal-session permission request, one the operator is
   genuinely being asked to answer right now, would be listed in an audit tab that
   states no answer is awaited. It would also break AC .3: the union subtracts the
   live permission-type bundles Needs-you excludes, while `unreadable` would
   simultaneously admit them to History.
2. **AC .3's fixture needs a live current-turn permission request.** That row is
   the one that fails if `unreadable` is ever widened back past clarifications.
   The ordinary live answerable clarification bundle in the same fixture is the row
   that fails if the union's subtraction term is ever restated as the complement of
   AC .2 (see "Why AC .3's union is not stated as a complement" below). Both rows
   are required; either alone leaves half the algebra unpinned.
3. **AC .30 needs a permission clause.** With permissions still reaching History
   via `superseded` and `session_ended`, and AC .12 requiring them labelled, a
   criterion mandating `title`, `prompt`, `options[].label` and `context` has no
   valid projection for them. AC .30 therefore names `content` and `options[].name`
   for a permission bundle and omits the clarification-only fields, rather than
   rendering them empty, which AC .13's "say so rather than render blank" would
   otherwise trigger on every permission row.

## There is not one operational read; there are two

This is the evidence behind the requirement's AC .2 naming three concrete
exclusion reasons instead of the phrase "excluded by the operational predicate".

- `ListPendingInteractions` (`pending_interactions.go`) covers BOTH
  `clarification_request` and `permission_request`, composing `currentTurnAuthority`
  with `nonTerminalSessionPredicate` (lines 64-70). It backs the pending-action
  projection. Its `ranked_permissions` CTE keeps only `rn = 1` per session within
  the current turn, which is the second clause of AC .2's `superseded`.
- The Needs-you tab renders something narrower:
  `ListUnresolvedClarificationBundles` (`inbox_handlers.go`). Its query
  hard-filters `WHERE m.type = 'clarification_request'` and additionally drops any
  bundle whose question identifier cannot be resolved
  (`clarification_bundle_query.go`, the flat `metadata.question_id` falling back to
  nested `metadata.question.id`, aggregated with `MAX(...)` so the drop is
  bundle-granular). Both filters are independent of turn currency.

**Every filter each read applies, and where it lands in the contract.** The
requirement's AC .2 and AC .3 were written against this enumeration; a filter
missing from it is a hole in the contract.

| Filter | Read | Contract home |
|---|---|---|
| current turn (`currentTurnAuthority`) | both | AC .2 `superseded` |
| non-terminal session | both | AC .2 `session_ended` |
| unresolvable question id (bundle-granular, `MAX(...)`) | Needs-you | AC .2 `unreadable`, clarifications only |
| `type = 'clarification_request'` | Needs-you | AC .3, live-and-excluded |
| `parent_question` truthy | Needs-you | AC .2 eligibility, AC .3 |
| per-user dismiss/snooze sidecar | Needs-you | AC .3, live-and-excluded |
| newest permission per turn only (`ranked_permissions`, `rn = 1`) | projection | AC .2 `superseded`, second clause |

Three consequences the contract depends on. A permission request never appears in
the Needs-you tab at any turn. An unresolvable-question-id bundle is dropped from
Needs-you even when its turn is current, which is why AC .13's divergence is real.
And a `parent_question` record is excluded on an OWNERSHIP boundary, not a
staleness one: it is a question to a parent agent, so History excludes it too
rather than surfacing agent-to-agent traffic the operator was never asked to
answer. The sibling requirement already encodes this at `needs-you-inbox.md:38`.

**Why AC .3's union is not stated as a complement.** Writing it as "the
non-terminal unarchived bundles less those AC .2 classifies as live" is
degenerate: that expression IS AC .2's definition of History, so the formula would
assert `Union(History, Needs-you) = History` and, combined with AC .3's own
disjointness clause, force Needs-you to be empty. The subtraction term must name
only the live bundles Needs-you ITSELF excludes.

**Why `superseded` resolves on turn authority alone.** `currentTurnAuthority`
(`turn_authority.go`) is defined purely from turn lifecycle and ordering, with no
session-state term, matching the ADR. Composed with `nonTerminalSessionPredicate`
(as the two shipped reads compose it) a terminal session has NO current turn, so
every row in it would read `superseded`; since AC .2 evaluates `superseded` first,
`session_ended` would become unreachable and AC .10's omission test unwritable.

`nonTerminalSessionPredicate` (`message_clarification_response.go`) excludes on
SESSION state (`COMPLETED`, `FAILED`, `CANCELLED`), a different axis from turn
supersession. A row under an abandoned session therefore has no superseding turn,
which is why AC .10 scopes the superseding-turn field to the `superseded` reason.

## Row, bundle and the unit the criteria use

The requirement fixes three terms because review round 3 found them conflated: a
*row* is a `task_session_messages` record, a *bundle* is the rows sharing a
`pending_id`, and a *bundle row* is the rendered list item.

Eligibility is a BUNDLE property, not a row property. The shipped sibling settles
this by construction: `clarificationBundleTableExpr` computes `has_pending` as
`MAX(CASE WHEN ... THEN 1 ELSE 0 END)` over the bundle's rows, and
`buildInboxBundleViews` then hydrates the bundle through `FindMessagesByPendingIDs`,
which returns EVERY message for that `pending_id` including already-answered ones.
So a partially answered bundle is listed whole and rendered whole. AC .2 and AC .30
adopt that behavior rather than inventing a row-level filter that would drop a
terminal question out of the middle of a bundle the operator is reading.

## Terminal status

Already decided in code by `InteractionStatus.IsTerminal()` (`interactions.go`):
terminal is any status other than absent or `pending`. This capability consumes
that helper and does not restate the set. The model also carries `cancelled`,
which the histogram below does not observe in this instance but which
`IsTerminal()` treats as terminal. `InteractionStatusFromMetadata` normalizes an
absent or empty value to pending, which is AC .26.

## Status values in use

`clarification_request`: `answered` 477, `pending` 359, `expired` 25, `rejected`
14. `permission_request`: `approved` 38, absent 12, `expired` 2. Absent status
retains its existing legacy meaning of pending.

## Population and distribution

Non-terminal status, task not archived, turn no longer the session's current
conversational turn. Archived tasks hold 302 of the 371 non-terminal rows
instance-wide and are excluded by AC .4.

Two samples, two days apart, because the population is live and moving:

| Bucket | Rows | Bundles | Tasks | Sample |
|---|---|---|---|---|
| Step starts no agent | 46 | 41 | 13 | 2026-09-12 |
| Step starts an agent | 18 | 16 | 7 | 2026-09-12 |
| **Total** | **64** | **57** | **20** | 2026-09-12 |
| Step starts no agent | 46 | 41 | 13 | 2026-09-14 |
| Step starts an agent | 22 | 18 | 8 | 2026-09-14 |
| **Total** | **68** | **59** | **21** | 2026-09-14 |

An earlier revision of this document reported the first sample's task total as 19.
That was an arithmetic error, not a measurement: the two buckets are disjoint,
because AC .11 classifies by the task's single current step, so the column must
sum. It is 20.

**The 46 is the load-bearing figure and it is the one that reproduces.** The
no-agent bucket is identical across both samples while the agent-started bucket
grew, which is what a capability justified by "questions sitting where nothing will
re-raise them" should predict. Implementation must cite 46, not a total.

## What "starts no agent" can and cannot mean

The split is grounded in real config: a step whose `events.on_enter` holds no
`auto_start_agent` entry. Verified per step on this card's own workflow: `PR Review`
and `Done` are `{}`; `Spec`, `Build`, `Testing`, `Review`, `Create PR` and
`PR Fixup` all auto-start. The empty-`on_enter` case is therefore load-bearing for
the 46, which is why AC .11 states it explicitly.

AC .11 no longer speaks of a "disabled" entry. The persisted shape is
`OnEnterAction{ Type OnEnterActionType; Config map[string]interface{} }`
(`workflow/models/models.go`) and `HasOnEnterAction` is a pure `Type` equality
check that never inspects `Config`. There is no enabled/disabled field on the
action, on `StepEvents.OnEnter`, or in the loader's known-type map
(`config/workflows/loader.go`). The only shipped "prevent auto start" concept,
`PreventAutoStartAgentOnOpen`, is a global per-user setting applied at session
launch and is not readable per step. So no disabled entry exists to discount, and
an entry whose type string the loader does not recognize simply is not
`auto_start_agent`. Presence is the whole test.

The `clarification` package has no workflow dependency today: `Handlers` carries
store, hub, messageCreator, repo, eventBus, resolver, logger, `inboxTasks`,
`inboxBundles` and `now`. AC .11 therefore adds a new clarification-to-workflow
read. That is a build cost, not a contract gap.

## Shipped primitives reused, all confirmed present

- `IconShieldQuestion` and `STYLE_PERMISSION = "text-amber-500"`
  (`apps/web/lib/ui/state-icons.tsx`). A `PENDING_PERMISSION_ICON` pairing already
  exists, so AC .12 reuses rather than introduces.
- The `variant="line"` tab strip (`apps/packages/ui/src/tabs.tsx`) and the
  `secondary` badge variant (`apps/packages/ui/src/badge.tsx`).
- Bundle ordering `ORDER BY b.created_at ASC, b.pending_id ASC`
  (`clarification_bundle_query.go`).
- Limit bounds `defaultInboxLimit = 50` / `maxInboxLimit = 200`
  (`inbox_handlers.go`), with `limit=0` REJECTED 400 rather than clamped
  (`TestHttpListInbox_ZeroLimit_RejectedNotClamped`).
- `encodeInboxCursor` / `decodeInboxCursor`, the opaque `(created_at, pending_id)`
  codec AC .20 adopts unchanged.
- The `/hidden` sibling endpoint (`httpListInboxHidden`) is the closest precedent
  for the new read: same bundle view plus a reason field, page `count` vs workspace
  `total`, cursor pagination.
- The shipped stale-response guard AC .23 mirrors: `generationByWorkspaceId` and
  `appliedGeneration` in `apps/web/lib/state/slices/needs-you-inbox/types.ts`.
- The whole `/api/v1/clarification-inbox` route group is already gated on
  `needsYouInboxEnabled` (`handlers.go`). Riding that flag is the path of least
  resistance; the requirement mandates neither, deliberately leaving it to Build.

### Cursor uniqueness, stated correctly

`pending_id` is `uuid.New().String()` on the clarification path
(`clarification/store.go`), but a PERMISSION `pending_id` is a composite of the
form `<uuid>-<tool_call_id>-<unix_nanos>`. It is therefore not a UUID across the
whole population History reads. Uniqueness still holds (52 distinct values over 52
permission rows, and the composite embeds a nanosecond timestamp), so
`(created_at, pending_id)` remains a total order over bundles and AC .20 needs no
further bundle-level tiebreak. The earlier "it is a UUID" justification was simply
narrower than the population the contract now covers.

## Why AC .19 needs a third key

`orderInboxMessages` (`inbox_handlers.go`) sorts by normalized `question_index`
then `questionIDFromMetadata`, using `sort.SliceStable`. For the bundles AC .13
uniquely admits, `questionIDFromMetadata` returns `""` for every unresolvable
question, so two such questions with absent or equal indexes tie on BOTH keys and
the result falls back to hydration order, which is not a named column. AC .21's
"identical order" would then rest on something the contract never named. AC .19
adds the message `id` as a third key. Needs-you never lists such a bundle, so the
two tabs still never order a shared bundle differently, and AC .19's "NOT
`orderBundleMessages`" distinction stands: that function tie-breaks by
`created_at` then `id` for MCP resolution and does not clamp a negative index.

## Isolation mechanism

**This section is normative by reference: AC .5 requires the rule to scan exactly
the set enumerated here.**

AC .5, .17 and .22 are enforced by a new architecture-lint rule,
`ARCH-INBOX-HISTORY-ISOLATION`, per `docs/architecture-lint.md`. Rules are
implemented under `scripts/architecture_lint/rules` and their grandfathered
finding sets live under `config/architecture-lint/<rule>.json` in the shape
`{"version": 1, "rule": "ARCH-...", "entries": [...]}`. This rule ships with an
EMPTY `entries` list, so any violation fails; baselines can only shrink.

**Why a source-text scan rather than an import scan.** Every shipped rule is a
per-file scanner: a `Rule` carries an `applies_to(path)` predicate and a
`scan(path, source)` function. Two shapes exist in-tree. `task_office_import.py`
and `runtime_import.py` scan IMPORTS, which in Go are package-granular.
`frontend_root_state_cast.py` scans SOURCE TEXT within one named file. AC .5 needs
the second shape, because the history read must reuse the package-private
`currentTurnAuthority` and `nonTerminalSessionPredicate` and therefore must live in
`apps/backend/internal/task/repository/sqlite`, alongside most of its own sinks.
Within one Go package there is no import statement to forbid, so an import rule
would be structurally vacuous for them, and a cross-package violating fixture would
pass AC .5's positive test while proving nothing about the real risk. That is why
AC .5 requires the fixture to sit in the SAME package as the read.

**The closed sink set.** Five files, realizing all four sink categories AC .5
names. A rule scanning fewer does not satisfy AC .5:

| Path | Sink category |
|---|---|
| `apps/backend/internal/task/repository/sqlite/pending_interactions.go` | pending-action projection (`ListPendingInteractions`) |
| `apps/backend/internal/task/repository/sqlite/message.go` | pending-action projection (`GetPendingActionsBySessionIDs`) |
| `apps/backend/internal/task/repository/sqlite/pending_action_projection.go` | pending-action projection (epoch allocation) |
| `apps/backend/internal/task/repository/sqlite/message_clarification_response.go` | answer and resolution path |
| `apps/backend/internal/task/service/service_events.go` | pending-action event emitters (`addTaskPendingActionEventField`, `addPrimarySessionPendingActionEventField`) |

No workflow-engine file reads the operational interaction listing directly today,
so the "gates a workflow transition" category is realized transitively through the
projection files above rather than by a file of its own. If a direct reader
appears, it joins this table and the rule's set in the same change.

**The history-module source set, for AC .17 and AC .22.** Those two criteria
invert the direction: the forbidden SOURCES are the history modules and the
forbidden TARGETS are the sidebar badge state key and the Needs-you count key in
the web store slice (AC .17), and the event-stream subscription entry points
(AC .22). The rule's `applies_to` predicate for that half shall be the History
feature directory Build adds under `apps/web/`, plus the backend file declaring
the history read. Build owns both paths, since neither exists yet, and owns naming
the read's exported entry point, which is the identifier the sink half of the rule
forbids. The rule's own module shall enumerate all of them rather than infer them
from a substring match on "history", so the set is auditable. A source predicate
that matches nothing is the same vacuity AC .5's positive fixture test guards
against on the sink half, and the fixture requirement applies to this half too.

## Why the shipped sidecar routes are not gated

AC .15 constrains what the History surface CALLS, not what the Needs-you sidecar
routes ACCEPT, and `## Out of scope` records that those routes are unchanged.

`httpUpsertInboxSidecar` and `httpDeleteInboxSidecar` (`inbox_handlers.go`)
validate the payload, call `authorizeBundleAccessOrRespond(pendingID)`, which
checks durable bundle existence plus task access, and then write. A History
bundle's `pending_id` is a real durable bundle, so those routes accept one TODAY.
Making them reject one would mean two things the requirement forbids itself.

First, it is a write-path change to an operational endpoint, which AC .5's
additive-read-path constraint exists to prevent.

Second, and harder: History eligibility is not a stable property of a `pending_id`.
A live bundle becomes a History bundle the moment its turn is superseded, with no
write to the sidecar row and no event. So an operator who legitimately dismissed a
live bundle at T0 would, at T1, hold a sidecar row that a gate now classifies as an
illegal call. A gate must therefore answer what happens to that already-written
row: leave it (the gate is cosmetic), delete it (the capability mutates operational
state), or fail the read (a read-only tab breaking an operational one). None of
those is decidable inside a read-only audit view, which is why it is deferred
whole rather than half-answered.

## Rejected alternatives, recorded because they will be proposed again

The ADR (accepted, amended 2026-08-15 and 2026-08-24) already rejects the two
changes that would close the same visible gap:

- **Rewriting old rows when a new turn starts**, rejected at ADR line 103: it
  *"adds a mutation to every turn-entry path."*
- **Treating every historical pending row as active**, rejected at ADR line 93: it
  *"preserves unlimited late answers ... and block[s] workflow progress after newer
  turns."*

The requirement therefore changes neither expiry nor the operational predicate.

## Correction to the originating card

The card recorded 109 clarification rows across 25 live tasks plus 6 permission
rows, and stated that 85 sat in `PR Review` (70) and `PR Fixup` (15). Re-measured
on 2026-09-14 those figures do not reproduce: totals are lower and the
concentration has moved. The largest single step is now `Backlog` (18 rows), with
`PR Review` at 17 and `PR Fixup` at 4. The instance-wide histogram does reproduce
and has grown consistently, so this is real movement in the population between
2026-09-12 and 2026-09-14, not a measurement error. The capability is justified by
the 46 rows sitting where nothing will re-raise them, not by the card's 85.
Implementation must not re-copy the card's numbers.

## Honest limit

This makes the 46 rows sitting where nothing will re-raise them visible. With
re-ask and clearing both out of scope, it does not resolve them. "The Inbox fixes
the parked-question problem" would be an overclaim. "The Inbox stops them being
invisible" is the claim.

## A naming collision to avoid

`parked` already has an unrelated meaning in `state-icons.tsx`:
`parkedOnBackgroundWork` is a session waiting on background work. It is not this
capability's "the owning task sits in a step that starts no agent". These must not
share a symbol, and AC .29 forbids the E2E from naming the label "parked".
