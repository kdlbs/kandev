---
status: current
system: cross-workspace-task-handoff
requirements:
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-CLI-ROUTE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-AUTHORIZATION-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROFILES-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-PROVENANCE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-SAME-WORKSPACE-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-IDEMPOTENCY-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-REVERSE-LINK-001
  - REQ-CROSS-WORKSPACE-TASK-HANDOFF-START-001
---

# Failure modes and verification System Design

## Failure modes

Rows are ordered by the pre-create evaluation order and then the post-create
evaluation order defined in [Handoff mechanism](handoff-mechanism.md). `Code`
is the HTTP status the route returns; 200 means the call succeeded and
returned the response object. Every 400/403/500 status is one the
authorization and command-and-route-surface requirements define; 401 comes
from the authentication step ahead of them, and 200 (or, for the
activity-write row, no distinguishable status at all) is an ordinary
successful response, not part of that refusal set.

| Condition | Result | Code |
|---|---|---|
| No/invalid run token, or agent profile unloadable | refuse, no write | 401 |
| Unknown argument, or missing/blank required argument, or blank optional string | refuse, name it | 400 |
| Title over 60 runes | refuse, naming limit and actual length | 400 |
| Target workspace equals source workspace | refuse, name the same-workspace create command | 400 |
| Capability absent, or revoked at any point before the call | refuse, no write | 403 |
| Target workspace not visible to the source task's owner | refuse, indistinguishable from missing | 400 |
| `workflow_id` missing or in another workspace | refuse, one message for both, no owner disclosed | 400 |
| Workflow has no resolvable step, or no steps | refuse | 400 |
| Step listing failed to execute | refuse, retryable | 500 |
| `agent_profile_id` or `executor_profile_id` unresolvable | refuse, name which | 400 |
| `repository_id` not in target workspace, or `base_branch` without it | refuse, name it | 400 |
| Any target-resource lookup fails to execute (workspace, workflow, profile, repository) | refuse, retryable | 500 |
| `external_id` held by a task this source did not hand off | refuse, no reverse link | 400 |
| `external_id` already used by this source in target workspace | return existing task, repair link, report the found outcome | 200 |
| Settlement fails after the task was created | refuse, message carries the task id (the one exception to the fixed generic 500 message); task not deleted | 500 |
| Settlement reports the identity was lost | outcome reported as identity-lost, no launch | 200 |
| Source task gone when the reverse link is written | response object with `reverse_link_recorded: false`, no retry | 200 |
| Delivery task exists, reverse link failed | response object with `reverse_link_recorded: false` and a `reverse_link_error` | 200 |
| Source task's reverse-link data is a non-array, or has a malformed entry | response object with `reverse_link_recorded: false`; nothing overwritten, no retry | 200 |
| Found task's stored handoff timestamp unreadable, reverse-link entry **absent** | response object with `reverse_link_recorded: false` and **empty** handoff timestamp; no write, no substituted clock, no retry | 200 |
| Same, but the reverse-link entry is already **present** | response object with `reverse_link_recorded: true`, no `reverse_link_error`, and **empty** handoff timestamp | 200 |
| Reverse link failed on a created call that asked to start | launch still dispatched; `reverse_link_recorded: false` and `started` decided by the three launch conditions alone | 200 |
| Launch call failed, timed out, or no launcher | response object with `started: false` and a `start_error` | 200 |
| Activity write failed | success; logged, not surfaced | — |

Two corrections from an earlier draft of this table, recorded so neither
regresses:

- The generic "a target-resource lookup fails to execute" row belongs to the
  pre-create target-resource-check step and is ordered immediately after the
  other pre-create refusals and before the idempotency-resolution rows,
  because it can only be reached before a create has happened. An earlier
  draft placed it after the idempotency rows, which both broke the table's
  own stated ordering and read as if a target-resource lookup could still
  fail after the delivery task already existed.
- The 401 status was previously attributed to "the closed set" the
  request-refusal contract defines, but that contract enumerates only
  400/403/500. 401 is defined by the authentication step that runs before
  any of those checks, and 200 is not a refusal status at all; the note
  above states each status's actual source instead of collapsing them into
  one set that does not in fact contain all of them.

## Verification notes

Each bullet names a test obligation and the weaker test it must not become.

- **The route, not only the command.** Every criterion governing the request
  shall be exercised against the route itself with a signed run token; a
  test that only drives the CLI proves flag wiring and nothing about the
  gate, because the command is not a trust boundary. The command-level
  exemptions (see the command-and-route-surface requirements) are each
  verified at their own surface instead — help text and instruction content
  cannot be observed from a route response.
- **The withdrawn MCP surface stays gone.** The Office MCP tool-inventory
  tests shall pass with no edit and no new assertion, because the granted
  and ungranted inventories are now identical; a diff touching either
  signals an incomplete withdrawal. A repository search for the withdrawn
  tool's name, its permission-check helper, and the withdrawn system-prompt
  placeholder's resolution function should return nothing outside this
  specification's own text.
- **The capability grant** needs three agents: a granted CEO, a non-CEO
  role, and a CEO whose per-agent override clears the permission — the third
  is what distinguishes a permission-derived grant from a bare role check.
- **The live-permission-precedence rule** needs a granted call, an
  ungranted call (403), a call with no or an invalid token (401) — each
  asserting no write — and a token minted while granted and replayed after
  the permission was revoked, asserting 403. That last case is the only one
  that fails against an implementation trusting the token's signed snapshot
  over the agent's live permissions; its mirror image (a token minted while
  ungranted, replayed after the permission was granted) is equally required,
  since a naive "either source grants it" implementation would pass the
  first case while failing this one.
- **Activity-entry identity fields** are asserted against the run token's
  own identity fields, plus a token minted with no run id, which must not
  fail the call.
- **Concurrency and the compare-and-set append** need real concurrent calls,
  not two sequential ones: two concurrent handoffs from one source task
  (both appear); a concurrent write to a different metadata key (no
  spurious conflict, and the write touches only the reverse-link key); a
  source task deleted between read and write (a partial-failure result, not
  a retry loop); a reverse-link value that is not an array; and an array
  holding a malformed entry. The last two need both the partial-failure
  result and byte-identical stored metadata — a response-only test passes
  against an implementation that silently drops the bad entry.
- **Workflow validation** needs a non-existent workflow id and a real
  workflow in a third workspace, asserting the two responses are identical.
- **The auto-start-step case** needs a target workflow whose start step
  carries an auto-start `on_enter` action, asserting no session is created
  and `started` is false; asserting only the response field passes against
  an implementation that stamps the auto-start marker anyway.
- **The delivery task's own kanban identity** needs the stored origin, the
  empty identifier, and the unchanged target-workspace task sequence
  asserted directly; the office/non-office read-time projection alone
  passes against an office-triggering origin the requirement forbids.
- **Settlement** needs a create with an `external_id` and then a replay,
  asserting the found-and-settled outcome; checking only the first call
  passes whether or not settlement ran.
- **Profile resolution** needs three agent-profile cases — global (no
  workspace scope), target-scoped, and source-scoped (refused) — plus one
  executor-profile case; the source-scoped refusal is the one an
  existence-only implementation fails.
- **Launch reporting** must show `start_error` is reachable (a nil launcher,
  or a launcher returning an error, each with `started: false`), and needs a
  created call with `start_agent: true` whose reverse-link write fails,
  asserting a session was launched alongside `reverse_link_recorded: false`
  — the case that fails against an implementation that reads the post-create
  ordering as a dependency rather than a sequence.
- **Repair timestamps** need a replay whose delivery task carries a known,
  distinctly older handoff timestamp, asserting the repaired entry and the
  response both carry the stored value; a freshly-created task cannot
  distinguish this from the current time.
- **Unreadable stored timestamps** need three variants — absent, non-string,
  and unparseable — each asserting the partial-failure result, an empty
  response timestamp, and byte-identical stored data, plus two precedence
  cases: a source-id mismatch together with an unreadable timestamp must
  give the ownership refusal; an unreadable timestamp whose reverse-link
  entry already exists must report `reverse_link_recorded: true` with no
  error. An implementation that checks the timestamp before ownership fails
  only the first of these two.
- **The response object on a found outcome** needs a replay against a
  delivery task moved to a different workflow and step since creation,
  asserting the stored/current values rather than the request's; replaying
  the create's own arguments cannot detect this defect.
- **Reverse-link ordering** needs an out-of-order case: append a handoff,
  then repair an older one, and assert the array is sorted with the repair
  not last.
- **Step-selection determinism** needs a workflow with two steps sharing a
  position, asserting the same step across repeated calls, and again with
  the storage order reversed.

## User-visible surfaces touched

- **Office activity log** — two new action verbs, rendered by the existing
  activity surface on the source side; the target-side entry has no reader
  today.
- **Agent CLI surface** — the new subcommand, visible to Office agents only
  through the CEO role's own instructions, not a tool registry. It is
  agent-facing; there is no web rendering of it.
- **Agent permission settings** — one new permission key in the Office agent
  permission editor. Its label and description are English string literals
  delivered by the backend settings endpoint; no permission key exists in
  the web locale catalogs today, so no translation work or `i18n:check`
  obligation arises from adding this one.
- **Target workspace board** — a new card appears; no new rendering.

No new web page or interaction flow is introduced. The one new HTTP route is
agent-facing and token-authenticated, not a UI route.
