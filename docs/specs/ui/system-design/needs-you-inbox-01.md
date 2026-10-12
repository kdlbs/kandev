---
status: draft
system: ui
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
---
# Needs-you Inbox System Design Part 1

Part 2 (control flow, failure, persistence, status vocabulary, security, prior
art) is in [`needs-you-inbox-02.md`](needs-you-inbox-02.md); Part 3 (design
source, fidelity decisions) is in
[`needs-you-inbox-03.md`](needs-you-inbox-03.md).

## Purpose and boundaries

UI owns the destination, the count, the row presentation, and the per-user
dismiss and snooze sidecar. It consumes, and does not own:

- The answerable-bundle predicate and bundle visibility rules, owned by
  integrations (`REQ-INTEGRATIONS-EXTERNAL-QUESTION-ANSWERING-001`, labels
  L1a-L16, D4, D6).
- The current-turn predicate and its total order
  (`docs/specs/current-turn-authority`, AC-3).
- Clarification resolution semantics: first-write-wins, idempotent replay, and
  task resume.

The single most important boundary: the row set is defined by the bundle read
path alone, never derived from session state.

## Requirement mapping

All rows are `REQ-UI-NEEDS-YOU-INBOX-001`; the AC suffixes map as:

| ACs | Design section |
| --- | --- |
| .1-.4 | [Feature flag](#feature-flag) |
| .5-.11, .31, .38 | [Data and contracts](#data-and-contracts), [Security](needs-you-inbox-02.md#security) |
| .12-.14, .40 | [Data and contracts](#data-and-contracts), [Control flow](needs-you-inbox-02.md#control-flow) |
| .15-.19, .34 | [Control flow](needs-you-inbox-02.md#control-flow) |
| .20-.21, .35, .39 | [Failure and recovery](needs-you-inbox-02.md#failure-and-recovery), [Components and responsibilities](#components-and-responsibilities) |
| .22-.25, .32-.33, .36-.37, .41 | [Persistence](needs-you-inbox-02.md#persistence), [Data and contracts](#data-and-contracts), [Control flow](needs-you-inbox-02.md#control-flow) |
| .26-.27 | [Status vocabulary](needs-you-inbox-02.md#status-vocabulary) |
| .28-.30 | [Components and responsibilities](#components-and-responsibilities) |

## Verified input inventory

Every path and line below was read at commit `20efe4855`. Nine citations the
originating card and early review rounds got wrong are corrected here; a builder
following the card without this list looks for files that do not exist, and C6 is
a silent authorization bypass.

1. **The bounded, workspace-scoped, cursor-paginated list query already exists**
   (the card says it does not; that is true only of the HTTP layer).
   `ListUnresolvedClarificationBundles`
   (`apps/backend/internal/task/repository/sqlite/clarification_bundle_query.go:19`)
   already takes `WorkspaceID`, `CreatedSince`, a `(created_at, pending_id)`
   cursor and a `Limit` in one query. New backend work is HTTP exposure plus the
   sidecar predicate, not a new query.
2. **Ordering is already defined and already total:** `ORDER BY b.created_at ASC,
   b.pending_id ASC` (`:150`), matching the cursor comparison at `:65`, documented
   as spec L6 on `models/clarification_bundle.go:45`. AC .9 restates it.
3. **A count equal to list length already exists:** `resp.Count =
   len(resp.Bundles)` (`mcp/handlers/question_handlers.go:225`).
4. **`resolveThreadStatus` names two functions with incompatible return types.**
   The card cites `thread-view-query.ts:92-93`, NON-exported, returning the string
   union at `:15`. A second, exported one (`components/threads/thread-column.tsx:21`)
   returns the object `ThreadStatus` (`lib/threads/thread-session-status.ts:23`).
   AC .26 resolves to the latter family and needs no rename or re-export: the Inbox
   calls the already-exported `resolveThreadSessionStatus`. See
   [Status vocabulary](needs-you-inbox-02.md#status-vocabulary).
5. **Office DOES emit a clarification-ENTITY inbox row**
   (`office/dashboard/service_inbox.go:451-483`, `Type: "permission_request"`,
   `EntityType: "clarification"`, from `ListPendingPermissions()`). The card's
   substantive point survives: no Office row comes from the clarification BUNDLE
   path. Hence permission requests are excluded here and named in the empty state.
6. **The MCP visibility resolver cannot be reused from HTTP.**
   `resolveBundleVisibility` is unexported and its scoped branch is gated on
   `callerScoped(ctx)` = `resolvedByFromContext(ctx) != ""`, set only by an MCP tool
   call, so from a browser-facing handler it takes the UNSCOPED branch and disables
   workspace filtering. Use the identity mechanism in
   [Security](needs-you-inbox-02.md#security). Caution: the task service has its own
   `callerScope`/`callerSubject` (`task/service/service_access.go`) reading
   `authn.IdentityFromContext` — same names, different package, and those are the
   correct ones.
7. `lib/threads/thread-session-status.ts` declares `const STATUS = {` with NO
   `export`; only `resolveThreadSessionStatus` and `resolveThreadColumnStatus` are.
8. `models.ClarificationBundleSummary` is `{PendingID, SessionID, TaskID,
   CreatedAt}` — no task title.
9. `models.ClarificationBundlePage.HasMore` is computed by the shipped query under
   the same filters; do not re-derive truncation from cursor presence.

Reused unchanged:

| Contract | Location |
| --- | --- |
| `currentTurnAuthority` returns predicate **and** order; excludes lifecycle-only turns | `apps/backend/internal/task/repository/sqlite/turn_authority.go:74-87` |
| Bundle list query, parent-question exclusion, missing-question-id exclusion, visibility disjunction | `clarification_bundle_query.go:109-205` |
| Page options: default limit 50, cap 200 | `apps/backend/internal/task/models/clarification_bundle.go:19-50` |
| Bundle messages by id | `FindMessagesByPendingID`, wired at `apps/backend/internal/backendapp/helpers.go:1902-1904` |
| Question projection, response envelope, base64url cursor | `clarification/bundle_question.go:68-86`, `mcp/handlers/question_handlers.go:205-265` |
| Resolution endpoint `POST /api/v1/clarification/:id/respond` | `apps/backend/internal/clarification/handlers.go:207-212` |
| Answer component props `{pending, messages, onResolved, shortcutScopeRef, maxHeightVh}` | `apps/web/components/task/chat/clarification-panel-section.tsx:54-60` |
| Shared status vocabulary, including `clarification` and `permission` kinds | `apps/web/lib/threads/thread-session-status.ts:9-99` |
| Office count key, sole writer and sole reader | `office-slice.ts:258`, `office/selectors.ts:42` |
| Sidebar chrome, row icon/hue, em-dash check, five locales | cited in the card; `app-sidebar-*`, `lib/ui/state-icons.tsx`, `scripts/check-no-em-dash-ui.mjs`, `src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}` |
| Request identity on every HTTP route, synthetic when auth is disabled | `apps/backend/internal/auth/httpmw/middleware.go` (`Middleware`, `SyntheticIdentity`), `auth/authn.IdentityFromContext` |
| Workspace authorization with the 404-vs-403 rule | `taskSvc.AuthorizeWorkspaceScope` / `requireWorkspaceScope`, `apps/backend/internal/task/service/service_access.go` |
| Caller-visible workspace set | `taskSvc.ListWorkspaces` -> `filterWorkspacesForCaller`, same file |
| Workspace-wide pending-action broadcast, fail-closed | `ws.ActionSessionPendingActionChanged` -> `hub.BroadcastToWorkspaceOrDrop`, `apps/backend/internal/gateway/websocket/task_notifications.go` |
| Boot-state per-workspace keys and active-workspace cookie | `bootStateBuilder`, `apps/backend/internal/backendapp/boot_state.go` |
| Page truncation flag | `models.ClarificationBundlePage.HasMore` |
| Flag registry and profile shape | `apps/backend/internal/runtimeflags/registry.go:32-48`, `profiles.yaml:39-45` |

## Feature flag

Registry key `features.needsYouInbox`, env var
`KANDEV_FEATURES_NEEDS_YOU_INBOX`, following the camelCase-to-SCREAMING_SNAKE
convention (`features.multiTenancy` / `KANDEV_FEATURES_MULTI_TENANCY`). Profile
defaults `prod: "false"`, `dev: "true"`, `e2e: "true"`. The e2e value is
load-bearing: AC .30 makes an end-to-end spec mandatory and it cannot run
against a flag that is false everywhere. Follow `/runtime-feature-flags` for the
registry, profile and frontend completeness tests.

The nav entry is gated on this flag alone. It must not be placed inside the
`{inOffice && ...}` block, and must not read the Office feature flag.

## Components and responsibilities

**Backend.** New HTTP handlers in the clarification package: one read, one
sidecar upsert, one sidecar delete, one hidden-bundle list. The read resolves
identity and workspace access per [Security](needs-you-inbox-02.md#security) and reuses the existing
bundle lister with the sidecar filter applied inside the same query. It adds no
new SQL predicate for answerability.

Reuse stops at the lister. These handlers do NOT call the MCP response builder
`buildListPendingQuestionsResponse`, which emits the `QuestionStatus`
projection; this endpoint emits durable message rows instead, for the reason in
[Data and contracts](#data-and-contracts). The handler calls
`ListUnresolvedClarificationBundles` then `FindMessagesByPendingID` per bundle —
the same pair the MCP builder calls — and assembles its own envelope.

**Sidecar store.** A new per-user table plus repository methods for dismiss,
snooze and restore.

**Frontend.** A route component, a bordered container of flat divided rows, a
row component with a 32px muted icon chip and plain muted relative time, an
expander mounting the shipped `ClarificationPanelSection`, a nav entry, a new
store slice holding `needsYouCountByWorkspaceId`, and its selector.

**One shared component IS modified, and that scope is declared here rather
than discovered at Build (AC .39).** `ClarificationPanelSection` cannot be
mounted entirely as-is, because its only outward signal is
`onResolved: () => void` and `useResolveCallback` fires that solely on
`submitState === "ok"`. A `409 not_active` becomes `"expired"` and never calls
it; a `5xx`, a timeout or a network failure becomes `"error"` and never calls
it; and the winner's `claimed` / `status` / `answers`, which
`useClarificationGroup` already parses into `ClarificationRespondResult`, never
leave the hook. So AC .17's notice has no input, AC .35's rollback has no
trigger, and the losing-caller row removal has nothing to fire on. Two changes,
both additive:

1. **An optional outcome callback** threaded
   `useClarificationGroup` -> `ClarificationInputOverlay` ->
   `ClarificationPanelSection`, reporting the four outcomes AC .39 enumerates.
   It carries values the hook already computes; nothing new is derived. See
   [Failure and recovery](needs-you-inbox-02.md#failure-and-recovery) for the mapping.
2. **One string externalized.** The overlay renders the literal
   `{group.answeredCount} of {group.total} answered`
   (`data-testid="clarification-group-progress"`) with no `t()`. That string
   will render inside the Inbox, so AC .29 reaches it. The i18n ratchet judges
   added and changed lines and would never have caught an untouched one, which
   is precisely why it is named here. This is the ONLY copy change to the shared
   component.

`onResolved` keeps its exact signature and firing rule, the new callback is
optional, and the task chat and Quick Chat hosts pass neither, so their
behaviour is unchanged — which AC .39 requires a test to assert. The rejected
alternative was for the Inbox to post `POST /api/v1/clarification/:id/respond`
itself: that reads the outcome directly, and gives one bundle two posting paths
that must stay in step forever.

The count key is deliberately new: `inboxCountByWorkspaceId` has exactly one
writer today, and a second would make the Office and Needs-you badges overwrite
each other per workspace.

v1 renders no tab strip and no page title: the layout starts at the toolbar
with `p-6 space-y-4`, and the app top bar owns the title.

## Data and contracts

`GET /api/v1/clarification-inbox`

Query parameters: `workspace_id` (required), `limit` (optional, default 50,
capped at 200), `cursor` (optional, opaque).

Validation (AC .38), all `400` with no rows and no sidecar effect:

| Input | Rule |
| --- | --- |
| `workspace_id` absent or empty | reject. Never read as "the empty workspace", never defaulted to the caller's first |
| `limit` absent | default 50 |
| `limit` non-numeric or `<= 0` | reject, not clamped: a client sending `0` has a bug worth surfacing |
| `limit > 200` | clamp to 200; a cap, not an error |
| `cursor` absent | first page |
| `cursor` present but undecodable | reject |

Authorization failure is `404` per [Security](needs-you-inbox-02.md#security) (which records why
`403` is unreachable on every endpoint here), not `400`. A
read matching nothing returns `"bundles": []`, never `null`, and renders the
empty state; only a non-2xx renders the error state (AC .21, .31).

The workspace is a query parameter, not a path segment, on purpose:
`/api/v1/workspaces/:x/...` already carries two different parameter names at one
position (`:workspaceID` in the GitLab controller, `:wsId` in Office costs), and
gin panics at registration when sibling routes disagree on a wildcard name. A
query parameter adds no wildcard and matches the MCP tool's own `workspace_id`.

Response body:

```json
{
  "bundles": [
    {
      "pending_id": "...",
      "task_id": "...",
      "session_id": "...",
      "session_state": "WAITING_FOR_INPUT",
      "task_title": "...",
      "created_at": "2026-09-03T04:54:02Z",
      "context": "...",
      "messages": [ { "id": "...", "metadata": { "pending_id": "...", "question_id": "...", "question_index": 0, "question": { "id": "...", "title": "...", "prompt": "...", "options": [] } } } ]
    }
  ],
  "count": 1,
  "hidden_count": 0,
  "next_snooze_expiry": null,
  "next_cursor": "..."
}
```

`count` is `len(bundles)`. Because the sidecar filter runs INSIDE the bundle
query (see [Persistence](needs-you-inbox-02.md#persistence)), every returned row is already a
listable row, so `count == len(bundles)` holds by construction and AC .12 is
structural rather than merely tested. The client reads the list and the badge
from this one response and never computes either separately.

**The count has exactly TWO producers, and no third (AC .40).** A number is
easy to produce in three places and hard to keep equal in three places, so the
set is closed here:

| Producer | Bound | Truncation carried by | Rows alongside |
| --- | --- | --- | --- |
| This read | the request's `limit` (default 50, cap 200) | `next_cursor` present | yes, the same response |
| Boot hydration | the SAME default limit of 50, same workspace scope, same sidecar exclusion for the boot identity | an explicit flag in the boot payload (see [Control flow](needs-you-inbox-02.md#control-flow)) | no, and none are rendered yet |

Everything else that changes the row set produces NO count. In particular the
sidecar writes return no body: they answer `204` and the client re-reads
(below). That is what keeps AC .12's "no rendered state exists in which the
count disagrees with the number of listed rows" true by construction instead of
by three hand-matched computations. A third producer is the defect, not the
optimisation: a workspace-wide count returned from a write would read 59 while
the page it labels holds 49.

Both producers are bounded the same way, so AC .14's capped presentation is
reachable from either: 60 listable bundles read at a limit of 50 give `count`
50 with truncation set, before the route is opened and after, and the badge
never changes value merely because the operator visited.

`next_cursor` is present exactly when `models.ClarificationBundlePage.HasMore`
is true, which the shipped query already computes under the same filters.
Because hidden rows are excluded before `LIMIT` rather than after, a short page
means exhaustion and never "the rest were hidden" — which is what makes AC .11's
truncation indicator and AC .14's capped count honest.

`hidden_count` is how many answerable bundles in this workspace THIS operator's
sidecar is hiding right now. It is workspace-wide, not page-local, so it is a
SECOND query: a bounded `COUNT` over the same visibility and answerability
predicates with the sidecar join inverted. No such field exists in the shipped
tree and the MCP builder computes only a page-local `len()`, which cannot answer
a workspace-wide question. The second query is AC .33's cost, accepted
deliberately.

`next_snooze_expiry` is the earliest `snooze_until` strictly after server time
among the bundles THIS operator's sidecar is hiding in this workspace, or `null`
when none is snoozed. It falls out of the same bounded query that produces
`hidden_count` (a `MIN` beside the `COUNT`), so it costs no third round trip.
It exists because AC .41's timer has to be schedulable from a response the
client already reads: without it the client would have to call the hidden
endpoint purely to learn when to ask again, and the badge would depend on a
route the operator has not opened. A dismissal contributes no expiry, so a
workspace with only dismissals reports `null` and schedules nothing.

**Why this carries `messages` and not the MCP envelope's `questions`.** The MCP
tool projects each bundle to `QuestionStatus`
(`{question_id, title, prompt, status, options}`) for an agent reading data.
This endpoint's consumer is `ClarificationPanelSection` -> `ClarificationInputOverlay`,
which is driven by durable message rows: it reads `metadata.question`,
`metadata.question_id`, `metadata.pending_id` and orders by
`metadata.question_index`
(`apps/web/components/task/chat/clarification-input-overlay.tsx:57-92`). Handing
it the `QuestionStatus` projection would force the client to reconstruct message
rows — a second representation of one bundle, the drift AC .26 forbids
elsewhere. Hence reuse at the lister level, not the envelope level.

Messages travel with the list, so expanding a row needs no second request and
cannot race one.

AC .10's canonical order is `metadata.question_index` ascending, tie-broken by
`metadata.question_id` ascending. `question_index` alone is not a total order:
the shipped overlay
(`apps/web/components/task/chat/clarification-input-overlay.tsx`) coerces an
absent index to `0`, so a bundle with two indexless questions has a tie that
falls back to arrival order and is not reproducible. An index that is absent,
negative, or not a number sorts as `0` and then by `question_id`. The server emits
messages in this order AND rewrites each emitted `metadata.question_index` to its
0-based rank within it. That rewrite is what makes the two surfaces agree, because
the client does NOT preserve server order: `sortMessagesByQuestionIndex` re-sorts
on `question_index ?? 0`, which catches only null and undefined, so a negative
index sorts ahead of zero and a numeric string sorts as its own value, both
contrary to AC .10. Against normalized ranks that re-sort is a no-op. Normalizing
server-side rather than fixing the client is deliberate: changing the shared sort
would alter what the task session and Quick Chat render, which AC .39 forbids. The
rewrite is a response projection and writes nothing durable.

Row presentation: primary text is the first question's `title` when non-empty,
else its `prompt`, truncated; if both are empty the row falls back to the bundle
`context`, and if that is empty too, to a localized "Question from agent"
string. A row is never blank and never unlabelled.

The envelope's bundle-level `context` is the FIRST message's `metadata.context` in
the canonical order above when that is a non-empty string, else `""`; absent or
non-string counts as absent, and later messages are deliberately not consulted, so
the row's fallback text and the expanded panel cannot disagree — the overlay
renders exactly `readSharedContext(sortedMessages[0])`. Do not reuse
`bundleContext` (`mcp/handlers/question_handlers.go`): unexported in another
package, and it scans in STORAGE order.

Secondary text is the task title then the task identifier.
`models.ClarificationBundleSummary` carries no title (Correction 8), so the
handler resolves it per bundle from the task service and returns `task_title` —
bounded by page size (at most 200 lookups), and it spares the client a second
data source it would otherwise invent. A task lookup that FAILS, one that finds
nothing, and an empty title all yield `task_title: ""`, a failure being logged;
the row renders the identifier alone, so neither a deleted task nor a transient
store error hides a question. The title is presentational and the row stays fully
actionable without it, which is why it degrades where `messages` fails the page. Relative time derives from `created_at`.

`session_state` is the owning session's `state`, resolved per bundle by the
handler from the session service and bounded by page size exactly as
`task_title` is. It exists for one reason: AC .26's resolver takes
`Pick<TaskSession, "state"> & {...}`, so `state` is a REQUIRED argument (see
[Status vocabulary](needs-you-inbox-02.md#status-vocabulary)). Without this field the
client would have to fabricate a value or fetch the session separately, and a
fabricated state is a second classifier wearing a disguise. When the session
cannot be read the field is `""`; the resolver short-circuits on
`pending_action` before reading `state`, so the label is unaffected, and the
row still renders.

`messages` is the row's payload, not an ornament, so it does NOT degrade the way
`task_title` and `session_state` do. Two cases, and they differ:

- **The per-bundle message read FAILS.** The whole page fails and the Inbox
  renders the read-error state (AC .21). A transient store error is not a fact
  about that bundle, and dropping one row would hide a blocked agent behind a
  page that looks complete. An error the operator can retry is the honest
  outcome.
- **The read succeeds and returns NO messages.** That bundle is omitted from the
  page and the omission is logged with its `pending_id`. A row with no questions
  cannot satisfy AC .15 and is not actionable, so admitting it would breach
  AC .6's entry gate. This should be unreachable — the bundle query already
  excludes bundles with no resolvable question identifier (AC .8) and
  `resolveIdentity` treats a messageless bundle as not-found — but "unreachable"
  is a claim about today's predicates, and the failure it guards against is a row
  the operator can see and cannot answer.

An omission changes only `count`, which stays `len(bundles)` and therefore stays
equal to the rows listed (AC .12). It does not change `next_cursor`: truncation
describes what the bounded query found, not what enrichment kept.

That is also why the empty state carries a second condition: AC .20's trigger is
the absence of listed rows AND no truncation reported. The second clause is
AC .11's doing, not a rival trigger — it forbids showing a partial list as
complete, and a zero-row page that reports truncation is exactly that. A response listing zero rows
while carrying `next_cursor` means enrichment emptied a page the query had filled,
so the Inbox says the list could not be built in full and offers the AC .21 retry
rather than reporting the operator caught up. Nothing re-reads automatically there
— the omissions are logged, and a retry would refetch the same page and drop the
same rows.

### Sidecar write and restore contracts

AC .22-.25 and AC .32-.33 describe dismiss, snooze and restore; these are their
contracts (AC .36). All three sit under the same new `/api/v1/clarification-inbox`
prefix. A path parameter is safe here where it was not on `/api/v1/workspaces`:
that prefix already carries two disagreeing wildcard names at one position and
gin panics on that, whereas this prefix is new and every route below names the
same `:pendingID`.

**`PUT /api/v1/clarification-inbox/sidecar/:pendingID`** — dismiss or snooze.

```json
{ "state": "dismissed" | "snoozed", "snooze_duration": "1h" | "4h" | "24h" }
```

- `snooze_duration` is read only when `state` is `snoozed`; absent means `4h`
  (AC .32). Any other value, including a well-formed `2h`, is `400`. The set is
  closed deliberately: an open duration would let one entry hide a blocking
  question for longer than the nine-day failure this feature exists to fix.
- `state` absent or outside the two values is `400`.
- Authorized on the workspace owning the addressed bundle by the same rule and
  the same status mapping as the read (AC .36): an unseeable bundle is `404` and
  writes nothing, and an unknown `pendingID` is `404`. **The route carries no
  `workspace_id`, so the mechanism is NAMED here rather than left to Build:**
  `Resolver.AuthorizeBundleAccess(ctx, pendingID)`
  (`apps/backend/internal/clarification/resolver.go`), which resolves the
  bundle's `task_id` from its durable messages and then applies
  `AuthorizeTaskAccess` -> `authorizeTaskScope(ctx, taskID,
  authz.ScopeWorkspaceRead)`. That is the READ's rule reached through the
  bundle's task, not a second rule: it loads the task's workspace and tests the
  same `workspaceDecision(...).CanRead()` predicate at the same scope that
  `AuthorizeWorkspaceScope` tests for the read. Authorizing the bundle's ACTUAL
  workspace rather than a client-named one is also how AC .31's "a
  client-supplied workspace identifier shall not by itself grant access" holds on
  a route that supplies none. Every rejection collapses to one `404`
  (`ErrBundleNotFound`): an unknown `pendingID`, a parent-question record, an
  unresolvable task and an out-of-reach workspace are indistinguishable to the
  caller. Where a transient store error surfaces depends on WHICH step hit it, and
  that split is reused as-is. Inside `resolveIdentity` it propagates unconverted
  and the caller sees a server error. Inside authorization it does NOT:
  `AuthorizeBundleAccess` rewrites ANY error from `AuthorizeTaskAccess` to
  `ErrBundleNotFound`, and `authorizeTaskScope` returns raw errors from `GetTask`
  and from a non-not-found `GetWorkspace`, so a transient failure there is
  indistinguishable from a rejection and answers `404`. Build must NOT make this
  symmetric: that resolver is shared with the shipped clarification read endpoints
  and lies outside this feature's boundary. See
  [Security](needs-you-inbox-02.md#security) for why `403` cannot arise here.
- **The lookup matches ALREADY-RESOLVED bundles, and that is deliberate.**
  `resolveIdentity` reads durable messages, which outlive resolution, so a bundle
  dismissed and later answered elsewhere still resolves and lands in the `DELETE`
  table's "real and visible" rows, not its "unknown" row. Persistence keeps that
  orphan sidecar row and runs no retention job, so this is what keeps it
  deletable. A lister-based lookup would answer `404` instead, because the bundle
  lister is scoped to UNRESOLVED bundles — making an orphan row permanently
  unreachable through the only endpoint that removes it.
- Idempotent by construction: primary key `(user_id, pending_id)` makes it an
  upsert, so repeating leaves one row (AC .25). Re-snoozing REPLACES
  `snooze_until` with a fresh absolute instant from server time; it does not
  extend the old one.
- Concurrency: the primary key serializes writers, so two callers racing on one
  bundle leave one row and the later commit wins. A `PUT` racing a `DELETE`
  therefore ends hidden or visible, never both and never duplicated. Last-write-
  wins is safe here because the row is per-user preference, not a claim.
- **`204 No Content` on success. The write returns no count and no rows**, and
  the client then re-issues the list read with the canonical parameters of
  [Control flow](needs-you-inbox-02.md#control-flow), replacing rows, `count`,
  `hidden_count`, `next_snooze_expiry` and `next_cursor` together from that one
  response. Returning refreshed counts from the write instead is wrong twice
  over: it makes the write a third producer of a number AC .40 binds to one
  bound, and it cannot refill a truncated page — dismiss all 50 rows of a
  50-of-60 page and a counts-only response leaves the client holding zero rows
  over a workspace with 10 listable bundles left, which AC .20's trigger renders
  as caught up. The re-read returns those 10 (AC .13, AC .24). One bounded extra
  round trip per hide changes latency, not behaviour: sidecar writes are already
  non-optimistic and already round-tripping.

**`DELETE /api/v1/clarification-inbox/sidecar/:pendingID`** — restore.

- Deletes the operator's sidecar row, which is the whole of restore: the bundle
  was never mutated (AC .22). `204`, and the client re-reads exactly as it does
  after a `PUT`.
- **`DELETE` resolves existence and visibility FIRST, by the same check as
  `PUT`**, and only then decides. The three cases are distinct and must not be
  collapsed:

  | Addressed `pendingID` | Outcome |
  | --- | --- |
  | unknown, or in a workspace the caller cannot see | `404`, no sidecar write, no disclosure of which of the two it was |
  | real and visible, currently hidden by this operator | `204`, row deleted |
  | real and visible, not hidden by this operator | `204`, nothing changes (AC .36) |

  The middle and last rows are the idempotency AC .36 asks for: the goal state
  is already true, so succeeding is correct and `404` would be wrong. The first
  row is NOT that case, and letting "unknown" fall into the `204` branch would
  turn a bogus-id probe into an existence oracle running the wrong way: a
  caller would learn nothing from `204` but WOULD learn from its absence. Same
  status for all three visible outcomes is what keeps it non-disclosing.

**`GET /api/v1/clarification-inbox/hidden?workspace_id=`** — enumerate hidden.

- Same row shape, ordering, page bounds, validation and authorization as the
  main read, with the sidecar predicate inverted: exactly the answerable bundles
  this operator's dismiss or snooze is hiding. Each row additionally carries
  `state` (`dismissed` or `snoozed`) and `snooze_until` (`null` unless snoozed),
  because restore has to show the operator WHY a bundle is hidden and when it
  would have come back on its own (AC .37).
- **It reports TWO numbers, and they are not the same number.** `count` is
  `len(bundles)`, page-scoped, exactly as on the main read so the two envelopes
  stay one shape. `total` is the workspace-wide number of bundles this
  operator's sidecar is hiding, produced by the SAME bounded query that
  produces `hidden_count` on the main read, and it is what AC .37 binds to:
  `total == hidden_count` for the same operator and workspace, at any page size.
  An earlier draft said this endpoint's `count` equalled `hidden_count`, which
  holds only until an operator hides more bundles than one page returns — 60
  hidden at a limit of 50 makes those 50 and 60. Naming the workspace-wide
  number separately is what makes AC .37 true at every page size instead of
  only on small fixtures.
- "Inverted" is a mode on the same option, not a second option. The field added
  to `ListClarificationBundlesOptions` (see
  [Persistence](needs-you-inbox-02.md#persistence)) carries a user id AND a
  direction, `exclude` or `only`; the main read passes `exclude`, this endpoint
  passes `only`, and everything else about the query is identical. One predicate
  with two directions is what makes the two lists a partition of one answerable
  set: a bundle is in the main read or in this one, never both and never
  neither.
- This makes AC .33's "restore a hidden bundle" constructible; `hidden_count`
  gives the disclosure but not the identities.
- Like the main read, the v1 client sends no `cursor` here. With 60 hidden at a
  limit of 50 the operator sees 50 identities over a `total` of 60; restoring from
  that page shrinks the hidden set, and the next read surfaces the rest. That is
  the same refill argument as AC .13, and it is why `total` rather than `count` is
  what AC .37 binds to.
- **Empty and error.** Zero hidden bundles is `"bundles": []` with `count` and
  `total` both `0`, and the restore surface says nothing is currently hidden
  rather than rendering an error. A failed enumeration renders the read-error
  state of AC .21 and offers to retry. Because `hidden_count` and this
  enumeration are two round trips apart, a snooze expiring between them can put a
  disclosed "3 hidden" over an enumeration of zero; the newer read wins, so the
  restore surface replaces `hidden_count` with this response's `total`. AC .37's
  equality is an identity within one response, not across two.
