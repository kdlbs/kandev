---
status: current
system: office
requirements:
  - REQ-OFFICE-COORDINATOR-AUTHORITY-001
  - REQ-OFFICE-COORDINATOR-AUTHORITY-002
created: 2026-09-06
owners:
  - kandev
---

# Taskless Coordinator Authority System Design Part 2

## Purpose and boundaries

This part owns the board-read wire contract: the runtime route, its query parameters and
response shape, the ordering and paging rules, the error mapping, and the `agentctl`
surface that reaches it. The capability vocabulary that gates it, the task-scope
derivation and the annotation predicate are
[part 1](taskless-coordinator-authority-01.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-001` | [Board read](#board-read), [CLI](#cli) |
| `REQ-OFFICE-COORDINATOR-AUTHORITY-002` | [Request and response contract](#request-and-response-contract), [Ordering and the descending default](#ordering-and-the-descending-default) |

## Board read

New route, mounted in `runtime.RegisterRoutes` beside the existing group:

```
GET /api/v1/office/runtime/tasks
```

`/runtime/` is already on `officeWorkspacelessPrefixes`
(`internal/backendapp/office_scope.go`) with the reason "every handler calls
`contextFromRequest`, which ... derives workspace/task/run from its claims." The new
handler honors that: it reads `runCtx.WorkspaceID` and ignores any caller-supplied
workspace.

**Evaluation order**, which decides which refusal wins when a request is wrong in more
than one way:

1. `Allows(CapabilityListTasks)` — else `ErrCapabilityDenied` (`403`) + `runtime.denied`.
2. Non-empty workspace claim — else `ErrWorkspaceOutOfScope` (`403`).
3. Parse and validate **every** query parameter — any failure is `ErrInvalidListParams`
   (`400`), with no query issued and no partially-parsed page.
4. Delegate to `ListTasksFiltered` — always, with no unfiltered branch.

Authorization therefore always precedes parameter validation, so a run lacking the
capability learns nothing about which parameters would have been accepted.

Step 4 is the substance of AC-002.9: the dashboard handler forks on
`hasTaskFilterParams` and serves an unfiltered read from a query ordered
`ORDER BY t.updated_at DESC` with no tiebreak, which is unstable across pages. The
runtime read never takes that fork.

### Request and response contract

| Parameter | Cardinality | Maps to | Rule |
|---|---|---|---|
| `status` | repeatable | `ListTasksOptions.Status` | matches any supplied value |
| `priority` | repeatable | `.Priority` | matches any supplied value |
| `assignee` | single-valued | `.AssigneeID` | agent profile id, exact match |
| `project` | single-valued | `.ProjectID` | exact match |
| `sort` | single-valued | `.SortField` | `updated_at` \| `created_at` \| `priority`; absent → `updated_at` |
| `order` | single-valued | `.SortDesc` | `asc` \| `desc`; absent → `desc`, for every sort column |
| `limit` | single-valued | `.Limit` | integer; absent, `<= 0` or `> 500` → 100 |
| `cursor` | single-valued | `.CursorValue` | must be supplied with `cursor_id` |
| `cursor_id` | single-valued | `.CursorID` | must be supplied with `cursor` |
| `include_system` | single-valued | `.IncludeSystem` | boolean; absent → `false` |

Repeating a single-valued parameter is `ErrInvalidListParams`, never "last one wins" —
positional resolution is exactly the silent behavior change AC-002.13 exists to prevent.

#### Normalization

The table above says what each parameter means; these rules say how its raw value is read,
and they are strict on purpose. AC-002.13's whole point is that a malformed request fails
loudly instead of quietly becoming a different query, and every lenient rule here would be
a way to get a page you did not ask for.

- **Empty means absent.** A parameter present with an empty value is treated as not
  supplied. So `?limit=` takes the default, and `?cursor=&cursor_id=x` is *one* cursor
  component supplied, which is the `400` above — not a first page.
- **Enumerated values match exactly.** `sort`, `order` and `include_system` are compared
  byte-for-byte against their allowed spellings: `updated_at` / `created_at` / `priority`,
  `asc` / `desc`, `true` / `false`. No case folding and no trimming, so `DESC`, `True`,
  `1` and `" desc"` are each `ErrInvalidListParams`. A caller that has to guess the casing
  is a caller that will silently get the wrong order.
- **`limit` parses as a base-10 integer.** A value that does not parse is `400`; one that
  parses is then clamped by AC-002.4 (`<= 0` or `> 500` → 100). Parsing and clamping are
  different steps: `abc` is an error, `0` is not.
- **No trimming anywhere.** `status`, `priority`, `assignee`, `project`, `cursor` and
  `cursor_id` are passed through as given. A filter value with stray whitespace matches
  nothing, which is a visibly empty page rather than a silently different one.

Response body:

```json
{ "tasks": [ /* task rows */ ], "next_cursor": "", "next_id": "" }
```

`next_cursor` and `next_id` are separate fields rather than one opaque composite, so a
caller resumes by echoing both back (AC-002.5, AC-002.12). Both are empty on the final
page. A cursor is only meaningful under the sort column and direction that produced it;
the endpoint does not encode the sort into the cursor, so a caller that changes `sort` or
`order` mid-walk gets a well-ordered but differently-anchored page. That is stated rather
than defended against, because detecting it would mean encoding sort state into the
cursor, which the existing keyset shape does not carry.

### Ordering and the descending default

`ListTasksFiltered` gives a total order on `(<sort column>, tasks.id)` with both legs in
the same direction, an out-of-allow-list sort rejected by `resolveListTasksOptions`, a
limit clamped by `limit <= 0 || limit > 500 → 100`, archived, ephemeral and
automation-origin rows excluded in `buildTaskWhereClause`, and system-workflow rows
excluded unless `IncludeSystem`.

One thing it does **not** give is a descending default. `resolveListTasksOptions`
computes `dir := "DESC"; if !opts.SortDesc { dir = "ASC" }`, so the zero value yields
**ascending**; only the dashboard caller sets `SortDesc: true`. The `ListTasksOptions`
struct comment claiming a DESC default for `updated_at`/`created_at` describes callers,
not the function. The runtime handler therefore sets `SortDesc` explicitly from the
`order` parameter, defaulting it to `true` (AC-002.2). Relying on the repository default
would ship the contract inverted.

#### Four exclusions, one opt-in

`ListTasksFiltered` drops four classes of row, and only one of them is negotiable by the
caller. Three are emitted by `buildTaskWhereClause`; the fourth is appended by
`ListTasksFiltered` itself, after that helper returns:

| Excluded | Predicate | Emitted by | Opt-in |
|---|---|---|---|
| Archived | `t.archived_at IS NULL` | `buildTaskWhereClause` | none |
| Ephemeral | `t.is_ephemeral = 0` | `buildTaskWhereClause` | none |
| Automation-origin | `COALESCE(t.origin,'') != 'automation_run'` (`notAutomationOriginT`) | `buildTaskWhereClause` | none |
| System-workflow | `COALESCE(w.workflow_template_id,'') NOT IN (...)` | `ListTasksFiltered` | `include_system` |

The runtime handler calls `ListTasksFiltered`, so it gets all four; a caller reaching
`buildTaskWhereClause` directly would get three. The system-workflow clause is further
guarded by `!opts.IncludeSystem && len(sysArgs) > 0`, and `systemTasksPlaceholders()`
returns nil args when `SystemWorkflowTemplateIDs` is empty, so with no system template ids
configured the clause is omitted whatever `include_system` says. That branch is defensive
and currently unreachable, but it is why the opt-in column reads as a toggle on a clause
that exists only when there is a system-workflow list to exclude against.

AC-002.8 names all four rather than only the two an implementer would notice from the
route. The ephemeral and automation-origin predicates are inherited from the shared
query, not chosen here, and they are invisible in the request: a coordinator that cannot
see an automation-origin task should be reading a contract that says so, instead of
inferring it from a `WHERE` clause it never sees. The spec already names the
automation-origin clause on the write side — it is one of the four the runner set
inherits from `CountActionableTasksForAgent` — so leaving it unstated on the read side
would have been an asymmetry rather than a uniform silence.

Widening any of the three unconditional exclusions is a separate contract change, not a
new parameter. `include_system` stays the only opt-in this endpoint offers.

### Error mapping

`respondRuntimeError` today maps `errTaskTitleRequired` and `ErrProjectRequired` to
`400`, `shared.ErrForbidden` to `403` + `runtime.denied`, and everything else to `500`.
`resolveListTasksOptions` returns a bare `fmt.Errorf("invalid sort field: %s")`, which
under that mapping would surface an out-of-allow-list sort as `500`.

Two new sentinels in `internal/office/runtime/errors.go`, both added to
`respondRuntimeError`'s `400` branch beside the existing two:

- `ErrInvalidListParams` — any board-read parameter that fails validation. The handler
  validates parameters itself and returns this sentinel; it does not depend on the
  repository's untyped error text.
- `ErrCommentBodyRequired` — an empty or whitespace-only comment body (AC-004.6).
  `PostComment` performs no body validation today, so this is new behavior, not a
  re-mapping.

`ErrCapabilityDenied`, `ErrTaskOutOfScope` and `ErrWorkspaceOutOfScope` already wrap
`shared.ErrForbidden` and keep their `403`.

### CLI

`cmd/agentctl/kandev_tasks.go`'s `tasksList` is re-pointed from
`/api/v1/office/workspaces/{ws}/tasks` to `/api/v1/office/runtime/tasks`, and stops
reading `KANDEV_WORKSPACE_ID` to build its path — the token is the authority.

**It must also stop *requiring* that variable.** `getWithParams` takes a
`requiredEnvName` / `requiredEnvVal` pair and aborts with `"<NAME> must be set"` when the
value is empty, before issuing any request. Re-pointing the path while keeping that guard
would leave the command hard-failing on an environment variable the contract has just
declared non-authoritative. `tasksList` therefore calls a variant with no required-env
check, and succeeds with `KANDEV_WORKSPACE_ID` unset (AC-001.8).

Its three existing flags (`--status`, `--assignee`, `--project`) keep working, and it
gains one flag per remaining parameter: `--priority`, `--sort`, `--order`, `--limit`,
`--cursor`, `--cursor-id`, `--include-system` (AC-001.10). Without these the paging
contract of REQ-002 would be specified but unreachable, since this CLI is the only
sanctioned agent path to the endpoint.

**`--status` and `--priority` must be repeatable**, because the parameter table marks
those two repeatable and AC-002.13 gives them union semantics. Today they cannot be:
`tasksList` declares `--status` with `flag.String`, and `getWithParams` takes a
`map[string]string` and calls `url.Values.Set`, so a map cannot hold two values for one
key and `Set` would overwrite rather than append. Both change together — a repeatable
`flag.Var` collecting into a slice, and a request helper carrying `url.Values` (built with
`Add`) instead of a `map[string]string`. Left as-is, the union half of AC-002.13 is
specified behavior with no way for an agent to reach it, which is the gap AC-001.10
exists to close.

Board-read coverage for the exclusion set: an ephemeral task and an automation-origin
task are both absent from a board read, and stay absent with `include_system=true`, which
is what pins them as unconditional rather than merely default-off.

The dashboard route is left in place: it serves the browser, and the scope middleware
plus `AgentAuthMiddleware` already constrain it. This design removes it as the *agent's*
path, not as a route.
