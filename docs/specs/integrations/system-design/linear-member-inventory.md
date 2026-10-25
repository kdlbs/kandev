---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-LINEAR-MEMBERS-001
---

# Linear Member Inventory System Design

## Purpose and boundaries

The integration system owns Linear's team-scoped member inventory and its
projection into the watcher's Creator filter. Keep this correction inside the
existing `Client.ListUsers` contract. No response DTO, credential policy,
persistence, or frontend interaction changes are required.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| `AC-INTEGRATIONS-LINEAR-MEMBERS-001.1` | Traversal; Consumer boundary |
| `AC-INTEGRATIONS-LINEAR-MEMBERS-001.2` | Request and response contract |
| `AC-INTEGRATIONS-LINEAR-MEMBERS-001.3` | Failure behavior |
| `AC-INTEGRATIONS-LINEAR-MEMBERS-001.4` | Traversal; Failure behavior |

## Request and response contract

`GraphQLClient.ListUsers(ctx, teamKey)`, `teamMembersQuery`, and `usersData` live
in `apps/backend/internal/linear/graphql_users.go`. This focused member-client
file keeps `graphql_client.go` within its 800-effective-line limit. The shared
`linearMaxPageSize` remains in the package without a policy of member truncation.

The query declares optional `$after: String`, passes it as `users(after: $after)`,
and requests `pageInfo { hasNextPage endCursor }`. Every request retains
`first: 250`, `orderBy: updatedAt`, and
`teamMemberships.some.team.key.eq: teamKey`. Use the same receiver's endpoint,
HTTP client, credential, and caller context through the existing `do` helper.

Keep the member projection `id`, `name`, `displayName`, `email`, and `avatarUrl`
and the public `[]LinearUser` response unchanged. Decode page metadata with
presence information: distinguish absent/null `pageInfo` or `hasNextPage` from
an explicitly terminal page. `endCursor` may be absent/null/empty on a terminal
page. No other client query or DTO changes.

## Traversal

1. Retain empty-team validation before HTTP admission. Allocate an empty,
   non-nil result slice and invocation-local cursor tracking.
2. Start without an `after` value. Check the caller context before each request;
   decode each response into a fresh page value rather than reusing prior data.
3. Validate completion metadata and append the page's members in provider order,
   preserving every projected field. The result remains private until success.
4. When `hasNextPage` is false, check cancellation before returning the aggregate.
5. Otherwise require a nonempty `endCursor` never previously used as `after` in
   this traversal, remember it, and use its exact opaque value for the next page.

An empty page with a fresh cursor and `hasNextPage: true` continues normally.
A repeated cursor includes both immediate nonadvancement and longer cycles.
Only continuation cursors are cycle-checked; a terminal cursor need not advance.
Do not invent a maximum page/member count, deduplication, sorting, retry, or
snapshot policy. Finite valid inventories complete; caller cancellation and the
existing HTTP request timeout continue to bound interrupted requests. This does
not claim a snapshot across concurrent upstream membership updates.

[Linear's pagination reference](https://linear.app/developers/pagination)
defines `first`/`after`, completion metadata, and continuation by `endCursor`.
The existing implementation already selects `updatedAt` ordering.

## Failure behavior

Return `nil, err` on any `do` failure, including later-page HTTP, GraphQL,
transport, and decoding failures. Preserve the original error so existing
`errors.As(*APIError)` status handling and `errors.Is` context classification
continue to work. Return the context error directly when a boundary context
check fails. Never return accumulated members alongside an error.

For malformed completion metadata or an invalid continuation cursor, return an
ordinary descriptive error using the package's `fmt.Errorf` convention. Do not
include credentials, member data, or opaque cursor values in diagnostics, and
do not classify an invalid provider response as a caller/authentication error.
`Controller.writeClientError` already maps ordinary errors to HTTP 500 and logs
them; retain that path without new error codes, metrics, or UI.

## Consumer boundary and mobile parity

`Controller.httpListUsers` obtains the workspace-scoped client and publishes
`{ users: [...] }` only after success. `listLinearUsers` in
`apps/web/lib/api/domains/linear-api.ts` passes that response to
`useTeamsAndStates` in `linear-issue-watch-fields.tsx`.
`AssigneeAndCreatorRow` in `linear-issue-watch-filter-fields.tsx` maps every user
to a Creator item with `userOptionLabel`; there is no additional member cap.
Its assignee items are independent sentinels.

Desktop and phone consume the same returned inventory. Apply the
`mobile-parity` state/data exception: this backend correction changes no
rendered geometry, overlay, touch behavior, scrolling, navigation, or breakpoint
logic. The real-client HTTP tests prove the changed boundary; the inspected
unchanged consumer mapping establishes the Creator effect. No frontend or mobile
Playwright suite is needed for this scope, and no UI preview is applicable.

## Security and documentation

Preserve [workspace-scoped integration ownership](../../../decisions/0030-workspace-scoped-integration-settings.md).
No credential, scope, or persistence decisions change, so no new ADR is needed.
The public [Linear integration guide](../../../public/integrations.md#linear)
already describes Creator filters and states no member cap. Its separate
watcher-poll bound remains accurate. No public copy, screenshot, or API-shape
amendment is needed for this correction.

## Verification boundary

Use real `GraphQLClient` requests against `httptest.Server` in a focused
`graphql_users_test.go`. Assert full 250+1 inventory identity and fields,
continuation, scope/authentication on every request, empty and single-page
controls, later-page failures/cancellation, and invalid metadata/cursor
termination. Retain request-count bounds in invalid-response fixtures so a
broken implementation cannot hang the suite. Exact scenarios and commands live
in the [implementation plan](../../../plans/linear-creator-member-pagination/plan.md).
