---
status: draft
system: ui
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
---
# Needs-you Inbox System Design Part 2

Part 1 (boundaries, input inventory, flag, components, data and contracts) is in
[`needs-you-inbox-01.md`](needs-you-inbox-01.md); Part 3 (design source,
fidelity decisions) is in
[`needs-you-inbox-03.md`](needs-you-inbox-03.md).

## Control flow

**Read.** Route mount or workspace change issues one request. The response
populates both the row list and the count slice entry for that workspace.

**The badge does not wait for the route (AC .34).** Route mount as the only
trigger would mean the operator must open the Inbox to learn they need to —
the Overview's own failure, and the shape Office has today (its badge is
written solely from `apps/web/app/office/inbox/inbox-page-client.tsx`). Two
triggers fix it, neither new machinery:

1. **Boot.** `bootStateBuilder` (`apps/backend/internal/backendapp/boot_state.go`)
   already resolves the active workspace from the `kandev-active-workspace`
   cookie and adds per-workspace keys to the boot state map. It adds, for that
   workspace, `{ count, has_more, next_snooze_expiry }` — NOT a bare integer.
   It runs the SAME bounded read as the HTTP endpoint, at the same default
   limit of 50, with the same sidecar exclusion resolved for the boot
   identity, because AC .40 requires every producer to share one bound. A bare
   integer cannot represent AC .14's capped badge, so 60 listable bundles would
   render an exact 60 before the visit and a capped 50 after it, and the badge
   would change value purely because the operator opened the page.
   `next_snooze_expiry` travels with it so the timer in AC .41 can be armed on
   first paint. When no active workspace resolves, or the flag is off, the key
   is absent and no badge renders — absent is not zero, and neither renders
   (AC .13).
2. **The refresh triggers below**, which keep it right thereafter.

**Refresh (AC .19). No event is the contract; the re-read is.** Every trigger
below does one thing: re-read the bundle path. None of them decides what a row
is, which is what keeps AC .6 intact — the client never reads `pending_action`
to decide whether a row exists.

That framing is load-bearing rather than pedantic, because the obvious event
does NOT fire on every exit. `publishSessionPendingActionChanged` returns early
unless `pendingActionProjectionChanged` is true, and that reduces to
`previous.action != action` for the SESSION. So the enum, not the bundle, is
what changed. Three real transitions emit nothing:

- a bundle resolved and immediately replaced by another in the same session —
  the projection stays `clarification` throughout;
- a session going terminal, which is an AC .8 exit but not a pending-action
  change;
- a turn being superseded, likewise.

Binding correctness to that one event would strand rows in all three. So:

| Trigger | Covers | Wiring |
| --- | --- | --- |
| `ws.ActionSessionPendingActionChanged` | set and clear of the pending action — the common case, near-instant | none; already `hub.BroadcastToWorkspaceOrDrop` with `workspace_id` on the payload |
| `ws.ActionSessionStateChanged` | a session becoming terminal | none; already `hub.BroadcastToWorkspace` for exactly this reason ("so the sidebar task switcher can track state changes for all tasks") |
| WebSocket (re)connect | everything missed while disconnected | none |
| Tab visibility regained | a backgrounded tab whose socket was throttled | none |
| A bounded periodic re-read, at most once every 60 seconds, only while the tab is visible and a workspace is active | the residual: same-session bundle replacement, superseded turn, and any future exit nobody thought to emit an event for | none |

**All five triggers are owned by the count slice, not by the Inbox route
component**, for the same reason the snooze timer is (below): a trigger mounted
with the route stops firing the moment the operator navigates away, and AC .34
and AC .41 both require the badge to stay right while the Inbox is closed. The
route subscribes to the same slice; it adds no triggers of its own.

**One canonical parameter set, owned by the same slice (AC .12).** Every read in
this design — boot, route mount, all five triggers, and the convergence re-reads
after a settled resolution or a `204` — is issued with the SAME parameters: the
active workspace and the default limit of 50, and NO cursor. That is what "the
same parameters the Inbox is displaying" means wherever it appears below. Without
one canonical set, a background trigger would write a count computed over a
different page than the rows the route is rendering, which is precisely the
disagreement AC .12 forbids.

**The v1 client never sends `cursor`, and v1 has no forward-paging control.** The
endpoint accepts one, so the contract is complete and the hidden endpoint shares
the shape, but the Inbox does not use it and no AC needs it: AC .11 asks for a
truncation INDICATOR rather than navigation, AC .14 asks for a capped count, and
AC .13's refill works *because* the re-read is cursorless — emptying a 50-of-60
page returns the remaining 10 instead of whatever follows a now-stale cursor. So
`next_cursor` is consumed as a boolean, the truncation flag behind AC .11 and
AC .14, and is never echoed back. Forward paging is not part of v1.

Boot participates in the stale-response guard rather than sitting outside it:
the boot value seeds the slice at generation zero, and the first read for that
workspace supersedes it. So a boot payload that lands after an early read
cannot overwrite fresher rows.

The periodic read is the honest part of this table. It is the reason AC .19 can
say "for every exit in AC .8, not only the ones a push notification announces",
and it is bounded so it cannot become a poll: one read per minute per active
tab, against a query already bounded to 50 rows. `session.clarification_requested`
remains excluded as a trigger because its `TaskSessionNotificationPayload`
carries no `workspace_id` and so cannot be filtered to the active workspace.

**Snooze expiry (AC .24, AC .41).** No event fires when a snooze expires — time
passing is not a transition anything publishes — so the client schedules one
re-read at `next_snooze_expiry`, which the boot payload and every list response
already carry (see [Data and contracts](needs-you-inbox-01.md#data-and-contracts)), refreshed
on every response and cleared when the field is `null`.

**The timer is owned by the count slice, not by the Inbox route component.**
That placement is the whole of AC .41. A timer mounted with the route only runs
while the operator is looking at the Inbox, so a bundle snoozed for four hours
and then left alone would stay off the badge until some unrelated event
happened to fire — reproducing, for snoozed bundles, the exact
"you have to open the Inbox to learn you need to" failure AC .34 exists to
remove. The count slice is live for the whole workspace session, so the badge
recovers whether or not the route was ever mounted.

**On a workspace switch the timer is cancelled and re-armed from the first
response for the newly active workspace**, so it never fires for a workspace the
operator has left; a new workspace whose response carries no
`next_snooze_expiry` leaves no timer armed, which is correct, because nothing is
snoozed there. The five refresh triggers need no equivalent: they are not
per-workspace subscriptions, they re-read whichever workspace is active when they
fire, and the stale-response guard drops anything that arrives for the wrong one.

Expiry is evaluated server-side against server time with `snooze_until <= now`
(see [Persistence](#persistence)), so the timer only decides WHEN to ask; it never decides
what is hidden. A client clock running slow asks late and the row appears late.
A client clock running fast asks early, the server returns the row still
hidden, and the response carries the same `next_snooze_expiry` again — so the
worst case is a small number of wasted bounded reads, not a hidden question and
not a spin: the reschedule is floored at 5 seconds, so even a badly skewed clock
cannot busy-loop.

**There is NO local mutation. Rows and count change only when a response lands.**
The Inbox never splices a row out of the list and never adjusts
`needsYouCountByWorkspaceId` itself; every removal and every count change arrives
in a list response. That is one rule for an answered bundle and a hidden one
alike, which is what makes AC .13's "behave identically" structural rather than
coincidental, and it is why AC .40's producer set stays closed at two: a
client-computed decrement would be a third producer, bounded by nothing and equal
to the rows on screen only by luck. AC .12 is then satisfied by construction,
since the Inbox applies no change ahead of a response and no rendered state can
disagree with itself. The rejected alternative was an immediate splice-and-
decrement on the settled outcome. It is wrong exactly where it matters: on a
truncated page, answering the last row of a 50-of-60 page empties the list
locally and renders AC .20's caught-up state over a workspace with 10 bundles
still waiting. Deferring to the response costs one bounded round trip and removes
that state entirely. AC .35 then holds a fortiori: a submission that settles as a
failure has nothing to restore, because nothing was removed. During the window
between a settled outcome and the response that removes the row, the row stays
rendered with its component still mounted in that settled state, which already
refuses a second submit, so the deferral cannot produce a double resolution.

**Stale responses (AC .38).** Reads carry a per-workspace request generation.
The store applies a response only when its generation is the newest issued for
the currently active workspace, and drops it otherwise. Without it, a fast
workspace switch lets a slower earlier response replace the active list, and a
read issued before a resolution can resurrect the row just answered. This is the
guard this repo already applies to HTTP/WS cache races; named here so Build does
not rediscover it.

Answer: the expanded row mounts `ClarificationPanelSection` with `pending`
true and the bundle's messages. Submission goes through the component's
existing path to `POST /api/v1/clarification/:id/respond`, which already
performs the resume. No resume logic is added here.

**A settled resolution converges exactly the way a hide does.** Each of the three
removing outcomes re-issues the list read with the canonical parameters above —
the same contract a `204` from a sidecar write carries — replacing
rows, `count`, `hidden_count`, `next_snooze_expiry` and `next_cursor` from that
one response. This is what makes AC .13's "removing every listed row shall behave
identically whether those rows were answered or hidden" structural rather than
coincidental. Without it, answering the last row of a 50-of-60 page leaves the
Inbox holding zero rows over a workspace with 10 listable bundles left, and
AC .20's "lists no rows" trigger renders that as caught up until some later
trigger fires. Leaning on `ws.ActionSessionPendingActionChanged` instead does not
close it: that event does not fire when a resolved bundle is immediately replaced
by another in the same session, because the projection stays `clarification`
throughout. The fourth outcome re-reads nothing — it removed nothing, so there is
nothing to converge. A re-read that itself fails is a READ failure (AC .21),
never reported as a failed answer.

The Inbox is a browser caller and is therefore not an automation principal, so
the caller-self filter in the MCP path does not apply to it: an operator sees
every answerable bundle in the workspace, including ones raised by the agent
they are watching.

## Failure and recovery

**Four outcomes, and the Inbox must be able to tell them apart.** Nothing is
removed until one of them arrives, so every outcome below either removes the row
or leaves it exactly where it was. The Row and Count columns state the OUTCOME,
not the mechanism: a removal is applied by the convergence re-read that outcome
triggers, never by a local splice (see [Control flow](#control-flow)). The Inbox
learns which through the optional outcome callback declared in
[Components](needs-you-inbox-01.md#components-and-responsibilities) (AC .39); the
values it carries are exactly the ones `useClarificationGroup` already parses into
`ClarificationRespondResult`, so nothing new is derived and the Inbox never
posts the resolution itself.

| Outcome | Wire | Row | Count | What the operator sees |
| --- | --- | --- | --- | --- |
| This caller won | `200`, `claimed: true` | REMOVED | decremented | nothing extra; the row leaving IS the feedback |
| Another caller won | `200`, `claimed: false`, with the winner's `status` and `answers` | REMOVED | decremented | a transient NON-error notice naming the bundle and the winning outcome (AC .17) |
| Bundle no longer active | `409`, code `not_active` | REMOVED | decremented | a transient NON-error notice naming the bundle and saying it is no longer waiting |
| Anything else | `5xx`, network, timeout, unparseable body, unrecognized status | UNCHANGED | UNCHANGED | a retryable error, with the entered answer preserved (AC .35) |

**AC .35 is satisfied by never having removed the row, not by putting it back.**
It requires a failed submission to leave the row and the count at their
pre-submission values; because nothing is ever removed locally and this outcome
triggers no re-read, those values are never disturbed, so the outcome it forbids
— a removed row over an unresolved bundle — is unreachable by construction rather
than by a rollback that could itself fail. Its other two clauses still need building: the entered answer
survives because the shipped component stays mounted and keeps its own form
state, and the retry is offered through that same component.

**The callback fires exactly once per submission attempt**, on the transition
into a settled state, mirroring the guard `useResolveCallback` already uses for
`onResolved` (`last.current !== submitState`). Without that rule a settled
`"error"` would re-fire on every render and restore the row repeatedly. It also
inherits the hook's existing ownership fence: a callback is delivered only for
the submission that still owns the request, so a bundle swapped under an
in-flight submit cannot attribute one bundle's outcome to another's row.

Two corrections to an earlier draft are folded in here. First, it claimed both
of the middle rows "surface the winning outcome": they cannot. A `409` body
carries only `error` and `code` — it is the resolver's NO-winner branch — so
there is no winner to name, and its notice says the bundle stopped waiting
rather than inventing who answered it. AC .17 governs the `claimed: false` row
specifically, which is the only one that is actually a concurrent-resolution
loss and the only one that carries a winner. Second, the whole table was
unreachable as specified, because the shipped `onResolved` fires only on
`submitState === "ok"` and takes no argument: the `409` and the failure rows
never reached the host at all, so the Inbox could not tell a loss from a failure
and had no signal it could safely remove a row on. Removing at submit instead
was the earlier draft's answer and it is not available: the component exposes no
submission-started signal, and AC .39 forbids the Inbox posting the resolution
itself, so there is no moment before the settled outcome at which the Inbox
learns a submission exists.

A failed read, as opposed to a failed submission, renders an error state that is
visually and textually distinct from the empty state, and suppresses the badge.
Rendering "All caught up" on a failed read is the specific defect AC .21
forbids: it is indistinguishable from success and it is the way this feature
dies in week one.

**A read that fails AFTER a successful one CLEARS the rows it was replacing.**
The error state replaces the list, and the rows and the count are dropped in the
SAME store update, the rule that already governs removal. Keeping the last good
rows on screen instead is the tempting alternative and it is wrong twice: AC .21
suppresses the badge on a failed read, so N rows would render under an absent
count, exactly the disagreement AC .12 forbids; and rows that survived a failed
refresh are rows the server may already have resolved, so the operator would
expand one and submit into a `409`. What a transient blip costs is bounded and
self-healing: the error state carries a retry for the read, and the count slice's
triggers re-read regardless, so the list returns within at most the 60-second
reconciling period with no operator action. A failed read never writes sidecar
state and is never reported as a failed hide (below).

Sidecar writes are never optimistic. The row moves only after the write
succeeds, because the write is cheap and a wrongly-hidden question is expensive.
A failed sidecar write leaves the list unchanged and shows a retryable error.

**A `204` followed by a failed re-read is a READ failure, not a write failure**,
and the two must not be conflated now that the write returns no body. The
sidecar row is already durable at that point, so the Inbox renders the read
error state of AC .21 and offers to retry the READ; it must not tell the
operator the hide failed, and it must not re-issue the write, which would be
correct but pointless since the `PUT` is an upsert and the `DELETE` is a no-op
on an absent row. Retrying the read is always sufficient, because the server
state the read reports is already the state the operator asked for.

Snooze uses stored absolute expiry evaluated against server time, so a client
clock skew cannot hide a row.

## Persistence

New table `clarification_inbox_sidecar`:

| Column | Notes |
| --- | --- |
| `user_id` | the request identity's `UserID`; with auth disabled that is the synthetic identity's default user id, mirroring Office's `dashboardUserID` |
| `pending_id` | The bundle |
| `state` | `dismissed` or `snoozed` |
| `snooze_until` | Null unless `state = snoozed` |
| `created_at`, `updated_at` | |

Primary key `(user_id, pending_id)`, which makes repeated dismiss or snooze an
upsert and satisfies AC .25 by construction.

**The filter runs inside the bundle query, before `LIMIT`.** The shipped
`ListClarificationBundlesOptions` has no user or sidecar field, so the naive
implementation filters the page in Go AFTER the SQL `LIMIT`. That breaks three
things at once: hidden rows consume page slots (a limit of 50 returns 30 with 20
still waiting); `HasMore` then describes a page the caller never saw, so AC .11's
truncation indicator and AC .14's capped count both lie; and a page can come back
empty while listable bundles remain, rendering the empty state over a non-empty
workspace.

So `ListClarificationBundlesOptions` gains one optional field, e.g.
`ExcludeSidecarUserID string`. Empty leaves the query byte-for-byte as it is
today, which is how the MCP caller keeps its behaviour. Set, the query adds a
`LEFT JOIN` to `clarification_inbox_sidecar` on `(user_id, pending_id)` and
excludes `dismissed` rows and `snoozed` rows with `snooze_until > :now`.
`HasMore` and the cursor comparison are unchanged and now describe the filtered
set, making `count == len(bundles)` structural.

This is additive and inside this design's declared boundary: it changes neither
the current-turn predicate, nor the bundle visibility rules, nor resolution
semantics. Filtering in the client instead would let `count` include rows the
list excludes, breaking AC .12 and AC .23.

The sidecar never writes to `task_session_messages`. AC .22 is asserted by
resolving a dismissed bundle from another surface and observing success.

Snooze durations are 1h, 4h and 24h, default 4h (AC .32); the rationale for the
closed set is at the endpoint. The value is stored as an absolute `snooze_until`
compared against SERVER time, so a skewed client clock cannot hide a row. "Still
hidden" is `snooze_until > now`, so an expiry equal to the current instant is
already expired and the row is listed (AC .24) — ties resolve toward showing the
question, the safe direction for a surface meant to stop questions going
unseen.

Dismissal is reversible (AC .33), and reversibility needs two things, not one.

The DISCLOSURE is `hidden_count`, workspace-wide, so it cannot come from the
page: a second bounded `COUNT` over the same predicates with the sidecar
predicate inverted. An earlier draft said it arrived "without a second query",
which was not achievable — the rows it counts are exactly the rows the main
query excluded.

**That one query answers three questions, so there is no third round trip.**
Alongside the `COUNT` it selects `MIN(snooze_until)` over the rows whose
`snooze_until` is strictly after server time. The main read reports the count as
`hidden_count` and the minimum as `next_snooze_expiry`; the hidden endpoint
reports the same count as its `total` (AC .37), which is what makes
`total == hidden_count` an identity rather than two SQL clauses kept in step by
hand. The hidden endpoint's own `count` stays page-scoped, like every other
`count` in this design.

The RESTORE needs identities, which a count cannot carry.
`GET /api/v1/clarification-inbox/hidden` returns the hidden bundles so the
operator can choose one; `DELETE .../sidecar/:pendingID` then removes the
sidecar row and the bundle returns. Without the enumeration endpoint AC .33's
"restore a hidden bundle" is not constructible: the UI would know how many rows
exist and never which.

Without both, one mis-click hides a blocking agent's question with no way back,
strictly worse than the pre-Inbox status quo where the task at least appeared in
Threads.

Rows for resolved bundles are harmless: the bundle stops being answerable, so
it leaves the list regardless. No retention job in this iteration.

## Status vocabulary

AC .26 resolves to the object-returning family in
`apps/web/lib/threads/thread-session-status.ts`. The `STATUS` table in that file
is a module-private `const` and is NOT exported; the exported entry point is
`resolveThreadSessionStatus(input): ThreadStatus`, whose input is
`Pick<TaskSession, "state"> & { pending_action?, foreground_activity? }`.

An Inbox row calls `resolveThreadSessionStatus` with the owning session's
`state` — carried on the row as `session_state`, which is why that field is on
the envelope at all, since the input type is `Pick<TaskSession, "state"> & {...}`
and `state` is REQUIRED, not optional — and `pending_action: "clarification"`,
receiving
`{ kind: "clarification", labelKey: "threads:statusQuestionFromAgent",
hasAttention: true }` — the resolver short-circuits on `pending_action`, so the
result does not depend on `state`. Passing `"clarification"` is not deriving the
row from session state: the row exists because the bundle is answerable, and an
answerable bundle is by construction a pending clarification. The row set still
comes only from the bundle read path (AC .6); only the LABEL comes from the
shared resolver.

**No rename or re-export is required.** An earlier draft asked Build to export
the unrelated private `resolveThreadStatus` in `thread-view-query.ts`. That is
not needed for AC .26 and is not in scope; the two same-named functions live in
different modules and neither is imported here. Leave them alone.

The Inbox defines no status strings of its own and introduces no new hue:
`IconMessageQuestion` with `text-yellow-500`, the pair already used for
waiting-on-input.

AC .27's agreement test is deliberately scoped. The two membership predicates
differ by design: Threads derives from `pendingAction || taskPendingAction`
(session state), the Inbox from the bundle path. They coincide only on the
fixture class the AC names, and its four enumerated divergences are asserted AS
divergences so a change that accidentally aligns them fails loudly.

## Security

Workspace scoping is enforced server-side from the request identity, not from
the query parameter.

**Do not reuse the MCP path's visibility resolver** (Correction 6): from an
HTTP handler it takes the unscoped branch, sets `opts.Unscoped = true` and
silently disables workspace filtering, the exact inverse of AC .31.

The correct mechanism already exists and is identity-based:

| Step | Contract |
| --- | --- |
| Identity is on every HTTP request | `auth/httpmw.Middleware` calls `authn.SetOnGin`; with auth disabled it injects `SyntheticIdentity()` (`UserID` = the default user, `Role` admin, `Synthetic` true) |
| Authorize the named workspace (the two reads) | `taskSvc.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead)` (`task/service/service_access.go`) |
| Authorize the addressed bundle (the two sidecar writes, which carry no `workspace_id`) | `Resolver.AuthorizeBundleAccess(ctx, pendingID)` -> `AuthorizeTaskAccess` -> `authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead)` (`clarification/resolver.go`, `task/service/service_access.go`) |
| Resolve the visible set for the query | `taskSvc.ListWorkspaces(ctx)`, which already applies `filterWorkspacesForCaller` |

`AuthorizeWorkspaceScope` also settles the status code: `requireWorkspaceScope`
already encodes this repo's 404-vs-403 rule — `ErrWorkspaceNotFound` (404,
non-disclosing) when the caller cannot read the workspace, `ErrForbidden` (403)
when they can read it but lack the scope. The handler maps both straight through
and adds nothing.

**Which rejection each endpoint can actually produce (AC .31).** All four
endpoints here authorize on `authz.ScopeWorkspaceRead`. For that scope
specifically the 403 branch is DEAD CODE, and saying so is the point:
`Decision.CanRead()` is defined as `d.Scopes.Has(ScopeWorkspaceRead)`, the same
predicate `requireWorkspaceScope` then tests for the 403. A caller who fails the
first test already got the 404; a caller who passes it cannot fail the second.
So an out-of-reach workspace is always `404`, never `403`, on every endpoint in
this feature.

| Endpoint | Scope | Workspace reached via | Reachable rejections |
| --- | --- | --- | --- |
| `GET /clarification-inbox` | `ScopeWorkspaceRead` | the `workspace_id` query parameter | `400` validation, `404` out of reach |
| `GET /clarification-inbox/hidden` | `ScopeWorkspaceRead` | the `workspace_id` query parameter | `400` validation, `404` out of reach |
| `PUT /clarification-inbox/sidecar/:pendingID` | `ScopeWorkspaceRead` | the addressed bundle's task | `400` validation, `404` unknown or out of reach |
| `DELETE /clarification-inbox/sidecar/:pendingID` | `ScopeWorkspaceRead` | the addressed bundle's task | `404` unknown or out of reach |

**Two routes to one rule, not two rules (AC .36).** `authorizeTaskScope` loads
the task's workspace and then runs the SAME `workspaceDecision(...)` it runs for
a named workspace, testing `CanRead()` and the same `ScopeWorkspaceRead`. So the
sidecar writes are authorized "by the same rule as the read"; only the way the
workspace is identified differs, and for a route carrying no `workspace_id` the
bundle's own task is the only honest source. The `403` branch is dead on this
path too, for the same reason and by the same predicate. `authorizeTaskScope`
short-circuits to allow when the caller is unscoped, matching the
auth-disabled behaviour described below, and its own not-found
(`ErrTaskNotFound`) is folded into `ErrBundleNotFound` by `AuthorizeBundleAccess`
so the caller cannot tell an unknown bundle from an unreachable one.

**The sidecar writes deliberately do NOT take a stronger scope.** Spec review
suggested giving them one so AC .31's forbidden outcome would be constructible
somewhere, and that is the wrong trade: `workspaceRoleScopes` gives
`WorkspaceRoleViewer` exactly `{ScopeWorkspaceRead}`, so requiring anything
stronger would stop a viewer dismissing a row from THEIR OWN sidebar. The
sidecar is per-user preference state that mutates nothing shared and discloses
nothing; gating it behind `ScopeTaskWrite` would degrade real behaviour to make
a status code testable. AC .31's security guarantee is untouched by this —
non-disclosure, server-side enforcement, and a client-supplied `workspace_id`
granting nothing all hold. The 403 branch stays in `requireWorkspaceScope` and
starts producing outcomes the day an endpoint here needs a scope a reaching role
can lack; today none does.

With auth disabled, `callerSubject` returns `Unscoped: true` for the synthetic
identity and every workspace is visible. That is intended single-user behaviour,
not a bypass: AC .31 is satisfied by having consulted identity resolution, and it
starts constraining the moment auth is enabled, with no change here.

The sidecar is keyed by user, so one user's dismissal cannot hide a row from
another user once authentication is enabled.

Agents may only enqueue themselves. This is preserved rather than added: a
bundle is created by the agent's own session through the clarification request
path, and the Inbox adds no way for any caller to insert a row for a session it
does not own. The Inbox is read-plus-resolve only.

## Observability

Structured logs on read failure and on sidecar write failure, each carrying
workspace id and, where relevant, `pending_id`. No new metric namespace this
iteration: the population is small enough that logs suffice, and a counter that
is almost always zero invites the same "learn to ignore it" failure the badge
rule guards against.

## Prior art

**Leg 1, internal wiki — DID NOT RUN.** Receipt: `@henry` resolved
`OBSIDIAN_VAULT_PATH=/Users/henry/Documents/henry/wiki`,
`QMD_WIKI_COLLECTION=wiki`. The path exists but every read returns `Operation not
permitted` even with the sandbox disabled (macOS TCC, not the agent sandbox), and
there is no `qmd` MCP server or CLI, so the grep fallback needed the same blocked
filesystem. No wiki content read and none claimed; a re-run needs Full Disk Access.

**Leg 2, cross-vendor `saas-kb` — DID NOT RUN.** Receipt: no `saas-kb` MCP server
attached and no local checkout or CLI, so `search_fsm_docs` with
`category: "ai_sdlc"` could not be called. No vendor claims cited.

**Leg 3, in-repo at `20efe4855` — RAN, and is this design's basis.** Office's inbox
(`docs/specs/office/requirements/inbox.md`) is the closest precedent and supplies
four adopted positions: computed view not a table (AC-OFFICE-INBOX-001.2); a
dismissal sidecar keyed by `(user_id, item_kind, item_id)`; identical inputs give
identical output across restart; the badge uses the same aggregation as the list.
Also read: `docs/decisions/2026-08-14-current-turn-clarification-ownership.md`,
`docs/decisions/2026-09-05-bounded-clarification-response-path.md`.

**Differently from Office:** one source rather than eight item types from five,
because a count assembled from several sources cannot be guaranteed equal to the
list it labels; reachable outside Office mode; and ordered oldest-first, because
the failure being fixed is an old question nobody saw.

## Related decisions

- [Current-turn clarification ownership](../../../decisions/2026-08-14-current-turn-clarification-ownership.md)
- [Bounded clarification response path](../../../decisions/2026-09-05-bounded-clarification-response-path.md)
- [Runtime feature flags](../../../decisions/0007-runtime-feature-flags.md)
