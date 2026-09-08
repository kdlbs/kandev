---
status: draft
system: office
requirements:
  - REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001
created: 2026-08-19
updated: 2026-08-20
owners:
  - tbd
---
# Automations YAML Export System Design Part 6

## Purpose and boundaries

This design preserves the technical source detail for `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` during migration.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-AUTOMATIONS-YAML-EXPORT-001` | [Migrated source detail](#migrated-source-detail) |

## Migrated source detail

### Consistency, permissions, failure

**AC-29** — When an export is served, the system shall read the automations, their triggers,
their repository links, and every row it resolves a portable descriptor from, within a **single
read transaction opened on the store's reader handle**, so a concurrent create, update, or delete
cannot produce a document containing a trigger whose automation is absent, an automation missing
triggers it holds, or a descriptor resolved against a different snapshot than the row referencing it.

**The export shall establish that transaction's snapshot immediately upon opening it**, by issuing
its first read inside the transaction before performing any other work. This is what makes
`## Definitions`' *start* a single well-defined moment rather than an interval, and it is what
AC-13 and AC-30 are stated against.

*(SQLite's deferred transaction takes its snapshot at the first read, not at `BEGIN`. Left to the
builder, the gap between the two is unbounded — descriptor-lookup wiring, slug precomputation and
warning-buffer setup could all run inside it — and any commit landing in that gap is visible to an
export that had already "started". AC-13 was falsifiable that way by an implementation doing
nothing wrong; see `## Definitions` for the trace. Closing the gap costs one read.)*

Satisfying this **requires new read methods on the automation store** that accept the transaction,
and that work is in scope for this card. `Store` today splits `db` (writer) and `ro` (reader),
every read path takes only a `ctx` and issues against `s.ro` directly, `BeginTxx` is called only on
the writer, and no read transaction on the reader handle exists anywhere in the backend — so there
is nothing to reuse. See `## Decisions` › "Export is additive and read-only" for why adding them
does not contradict that decision.

**Descriptor rows are covered by this AC too, and the existing lookup seams cannot carry them.**
The automation package's four injected lookups return the wrong data for AC-18 and none accepts a
transaction:

| Existing seam | Returns | Why it cannot serve the export |
|---|---|---|
| `AgentProfileLookup.AgentProfileExists(ctx, id)` | `(bool, error)` | AC-18 needs `{agent_name, model, mode}` |
| `WorkflowLocator.WorkflowWorkspaceID(ctx, id)` | `(string, error)` | AC-18 needs the workflow **name** and the **step name** |
| `RepositoryLookup.GetRepository(ctx, id)` | `(workspaceID, defaultBranch string, ok bool)` | AC-18 needs the repository **name**; also two-valued, which AC-19 forbids |
| *(none)* | — | **No executor-profile lookup exists on the automation service at all** |

The export shall therefore define **new, transaction-accepting descriptor lookups** — one each for
agent profile, executor profile, workflow-and-step, and repository — following the established
`SetWorkflowLocator` / `SetRepositoryLookup` injection pattern: the interfaces are declared in the
automation package, each method takes the export's read transaction — the `*sqlx.Tx` opened on the
store's reader handle — as its first argument after `ctx`, and each reports the three outcomes
AC-19 requires.

**Where the SQL lives is part of this contract.** Each new lookup shall be satisfied by a **new
exported, transaction-accepting read method on the store that owns the table** — the
agent-settings store, the workflow repository, and the repository store respectively — and the
adapter constructed in `backendapp` shall do nothing but call that method, passing the transaction
through. **No SQL text shall be written in `backendapp`.**

*(This is stated because the AC is otherwise satisfiable two ways and only one of them is
consistent with the codebase. Verified: `internal/backendapp` today contains no raw SQL and no
`*sqlx.Tx` usage, every existing `Set*Lookup` adapter delegates to the owning package's exported
method, and the only `*sqlx.Tx` parameters in `internal/workflow/repository` and
`internal/agent/settings/store` are unexported helpers operating inside those packages' own
transactions. An adapter issuing its own `SELECT` against `agent_profiles`, `workflows`,
`workflow_steps` or `repositories` would be the first of its kind in the tree, and would put query
text for four tables in a package that owns none of them — with no compile-time signal when an
owning package renames a column. That is precisely the silent drift AC-22 and AC-43 exist to
prevent for this export's own fields, reintroduced one layer out. Delegation keeps each query
beside the schema it reads and keeps the seam doing what every other seam here does.*

*The seams exist so the automation package does not import the task, agent-settings or repository
services and create a cyclic import — AC-44 step 2 relies on the same rationale — so moving the
SQL into the automation package is not available either. Passing the transaction across the seam
costs nothing new: the automation package already depends on `sqlx` for its own `Store`, every one
of these tables lives in the same SQLite database, and one read transaction can read all of them.*

*The alternative considered and rejected was to exempt descriptor rows from this AC, which would
mean a descriptor resolved against a different snapshot than the automation referencing it —
exactly the inconsistency the AC exists to prevent, and the one a git-committed artifact would
preserve forever.)*

This AC is observed by a unit test, not by a race: the export path shall obtain its snapshot once
and pass it to every read — store reads and descriptor lookups alike — and the test asserts, with a
store double and four lookup doubles that each record the handle they were given, that every
recorded handle is the same one and that no read arrived with no handle.

*(Stated because a race is not deterministically testable, and an AC whose only verification is
"run it and hope" is one a builder can satisfy by doing nothing. Making the shared handle the
observable turns a timing property into a structural one. The lookup doubles are named explicitly
because a test that instruments only the store passes while every descriptor read runs outside the
transaction, which was true of an earlier draft of this AC.)*

**AC-30** — While an export is in flight, a concurrent mutation of the same workspace's
automations shall neither block nor be blocked by it; the export reflects the snapshot
open at its *start*, as `## Definitions` fixes that term — the moment its read transaction
establishes its snapshot, which AC-29 requires to happen immediately upon opening the
transaction.

*(Probed, not reasoned, because an earlier round cleared this claim on general grounds that turned
out to be incomplete. The probe replicates both DSNs from `internal/db/sqlite.go` exactly — writer
`_foreign_keys=on&_mode=rwc&_busy_timeout=5000&_journal_mode=WAL&_synchronous=NORMAL&_cache=shared`,
reader `_foreign_keys=on&_mode=ro&_busy_timeout=5000&_cache=shared` — opens a read transaction on
the reader pool, establishes its snapshot with a first read, and then writes the same table on the
writer pool. Result: `journal_mode = wal`; the `UPDATE` and a subsequent `INSERT` each **succeeded
immediately with no block**; and the still-open read transaction continued to see the pre-write
value, so snapshot isolation holds. A read transaction opened after the write also did not block.
AC-30 stands, with evidence.*

*One dependency is worth naming, because it is not what it appears to be. Both DSNs carry
`_cache=shared`, and SQLite shared-cache mode uses table-level locks under which this AC would
fail. It does not fail, because **`_cache=shared` is inert**: SQLite's URI parameter is `cache`,
and the driver's own parameters are `_busy_timeout`, `_journal_mode` and friends — neither honours
`_cache`, so shared cache is never enabled. Probed side by side against the same code path:
`_cache=shared` and no cache parameter at all behave identically (write succeeds), while a true
`cache=shared` fails the write immediately with `database table is locked: automations` — and
immediately, because `busy_timeout` does not retry `SQLITE_LOCKED`. **If that DSN is ever
"corrected" to `cache=shared`, this AC breaks at once and hard.** That is a pre-existing
observation about the database configuration, not a defect this card introduces or fixes; it is
recorded here so the dependency is visible rather than accidental.)*

**AC-31** — When a user without access to the target workspace requests an export, the system
shall deny the request through the same workspace authorizer used by every other automation
operation (`Service.authorizeWorkspace`, wired to `taskSvc.AuthorizeWorkspaceAccess`), return
`404`, and return no automation data and no error text distinguishing denial from absence.

*(`404` and not `403`, deliberately. `ErrAutomationNotFound` is documented as covering "both a
missing automation and one in a workspace the caller cannot reach, so authorization never leaks
which of the two it is". A `403`/`404` split on this endpoint would reintroduce exactly the
existence leak that sentinel exists to prevent: a caller could enumerate which workspace IDs are
real by reading the status code.)*

*(Testing note, because the obvious fixture observes the opposite: the authorizer returns `nil`
without checking anything when the caller context carries no user scope, which is Kandev's default
single-user mode. A test for this AC must supply a **scoped** caller context; run unscoped it
observes `200` and the AC looks wrong.)*

**AC-44** — The system shall evaluate the request in this order, and shall emit no automation
data unless every step passes:

1. **Authorize** the workspace through `Service.authorizeWorkspace`. Classify the result by
   sentinel, not by position: `nil` → continue; an error satisfying
   `errors.Is(err, repoerrors.ErrWorkspaceNotFound)` → `404` (AC-31); **any other non-nil error**
   → `500` (AC-45).
2. **Resolve the workspace** through a workspace lookup injected into the automation service at
   construction, following the existing `SetWorkflowLocator` / `SetRepositoryLookup` injection
   precedent — the seam exists so the package does not import the task service and create a cyclic
   import. `repoerrors.ErrWorkspaceNotFound` → `404` (AC-35); any other non-nil error → `500`
   (AC-45); lookup not wired → `500`, since the endpoint cannot honour AC-35 without it and an
   unwired lookup is a construction error rather than a deployment mode.
3. **Export.** An authorized, existing workspace holding no automations → `200` with an empty list
   (AC-32).

*(The sentinel rule in step 1 is spelled out because the wired authorizer makes the naive reading
unsafe. `taskSvc.AuthorizeWorkspaceAccess` returns **the same `repoerrors.ErrWorkspaceNotFound`
for a denied workspace and for a workspace that does not exist**, and returns any infrastructure
error from its own workspace read bare on the same path. A builder who reads "denial" as "the
authorizer said no" and treats a not-found error as "a failure that is not a denial" returns `500`
where AC-31 and AC-35 both mandate `404` — which re-opens, with the status code as the oracle,
precisely the enumeration leak AC-31 exists to close. Classifying on the sentinel makes denial and
absence indistinguishable, which is the required behaviour, and leaves `500` for the errors that
genuinely are failures.)*

*(Why step 2 exists, stated precisely because the general claim is false: for a **scoped** caller
step 1 already answers existence, since the authorizer's own workspace read produces
`ErrWorkspaceNotFound` for a workspace that is not there. Step 2 is the path that carries AC-35
for an **unscoped** caller — Kandev's default mode — where step 1 returns `nil` without reading
anything and `Service.ListAutomations` cannot help: the store returns an empty slice for a
workspace ID that never existed, indistinguishable from a real but empty workspace. The lookup is
therefore required and is not dead code, even though a scoped-context test will never reach it.
The automation package holds no workspace lookup of any kind today. Office's config export, which
this endpoint otherwise mirrors, never returns `404` at all — it returns `500` on any error — so
the precedent is not usable here.)*

*(Step 2 and step 3 read from different snapshots and the gap between them is not closed. If the
workspace is deleted after step 2 succeeds, the export returns `200` with whatever the AC-29
snapshot holds, which for a cascaded delete is an empty list. This is an accepted race: the
existence check is a gate against a workspace ID that was never real, not a guarantee that the
workspace outlives the request, and paying for the stronger guarantee would mean holding the
workspace row in the export's own transaction across a service boundary the injection seam exists
to avoid.)*

**AC-32** — When a workspace has no automations, the system shall return a valid document
with `version`, `type`, and an empty `automations` list, and the zip form shall be a valid
archive containing no automation files. This is not an error.

**AC-33** — When an automation's stored `name` is the empty string, the system shall emit
`name: ""` and derive its filename through the AC-26 fallback. *(`Service.CreateAutomation`
rejects an empty name, but the `automations.name` column permits one, so a row written by
any other path is representable.)*

**AC-34** — When an automation's stored `max_concurrent_runs` is zero or negative, the
system shall emit the stored value unchanged rather than substituting the service's
`1` default. *(Export reports what is in the database; it is not a validation pass.)*

**AC-35** — If the target workspace does not exist, then the system shall return `404` and
no partial document, indistinguishable in status and body from the AC-31 denial. The
existence answer comes from the lookup named in AC-44 step 2.

**AC-36** — If serialization of any automation fails, then the system shall return `500`
and no partial document. A partially-written body is never emitted.

**AC-45** — If the read transaction cannot be opened, any query fails, the workspace lookup
fails for a reason other than not-found, the authorizer fails for a reason other than
denial, or the zip archive cannot be written, then the system shall return `500` with no
automation data and no partial body.

**AC-48** — The system shall build each response body completely in memory before writing
any of it to the response, and shall write the status line only once the body is known to
be complete.

*(AC-36 and AC-45 both promise "no partial body", and that promise is unimplementable in
the streaming shape Office uses: `exportConfigZip` sets `200` and the `Content-Disposition`
header and then `io.Copy`s the archive to the writer, so a failure mid-copy has already sent
`200` and cannot retract it — the client receives a truncated archive under a success
status. Buffering is what makes the guarantee real. The measured corpus is ~78 KB of prompt
across 7 automations, so the memory cost of buffering the whole artifact is not a concern at
this scale.)*

### User surface

**AC-37** — When a user opens a workspace's automations settings page, the system shall offer an
export control that downloads the zip form. The control shall be present when the page itself is
(a user who can open the page has passed the same workspace check the export authorizes against),
shall be disabled from the moment the request is issued until the download has been handed to the
browser or an error has been surfaced, and shall surface a non-blocking error message if the
response status is not `200`, leaving the page otherwise unchanged. A workspace with no automations
still shows the control enabled — AC-32 makes that a valid export, not an error.

The control shall issue the request with `fetch`, check the response status, read the body to a
`Blob`, and trigger the download from an object URL with an explicit filename of
`kandev-automations.zip`. It shall **not** navigate to the endpoint with `window.open` or an anchor
`href`.

*(The mechanism is pinned because the two halves of this AC are unimplementable without it, and the
precedent this spec otherwise mirrors gets it wrong. Office's equivalent control is
`window.open(officeApi.exportConfigZipUrl(activeWorkspaceId), "_blank")`: a raw navigation gives the
calling code no completion signal and no access to the response status, so there is nothing to
disable on and nothing to test for. Worse, a navigation to a non-`200` renders the error body in a
blank tab, which is the opposite of "leaving the page otherwise unchanged". Under `fetch` the
filename comes from the object URL's `download` attribute rather than from `Content-Disposition`;
the header stays on the response for anyone hitting the URL directly, but it is not what names this
download. "In flight" is defined above as the whole span from issue to hand-off precisely because
"until headers arrive" and "until the body is read" are different windows and only the wider one
prevents the second click AC-37 is guarding against.)*

**AC-38** — All copy introduced by the export control shall be resolved through `t()` or
`<Trans>` and shall be present in `en`, `pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`.

## Failure modes

Every row cites the AC that makes it testable. A row without one is a shadow path.

| Condition | Behaviour |
|---|---|
| Workspace has no automations | `200`, empty `automations` list (AC-32) |
| Workspace does not exist | `404`, body indistinguishable from denial (AC-35, AC-44) |
| Caller lacks workspace access | `404`, no data, no distinguishing text (AC-31, AC-44) |
| Workspace lookup not wired | `500` (AC-44 step 2) |
| Agent/executor profile or repository row is **absent** | Reference omitted, warning emitted, export succeeds (AC-19) |
| A descriptor lookup **fails** rather than reporting absence | `500`, no partial document, **no** warning — absence and failure are distinct outcomes (AC-19, AC-45) |
| Workflow resolves but its step does not | `workflow.name` emitted without `step`, warning emitted (AC-19) |
| Some repositories resolve, others do not | Unresolved members dropped, resolved members kept in order, warning emitted (AC-19) |
| Trigger `config` is not valid UTF-8 | Empty `config` mapping, `config is not valid UTF-8` warning; checked on raw bytes before decoding and takes precedence over the next two rows, so exactly one warning is emitted (AC-39) |
| Trigger `config` is not valid JSON | Empty `config` mapping, warning naming automation + trigger type + condition; export succeeds (AC-39) |
| Trigger `config` is valid JSON but not an object | Same as above (AC-39) |
| Trigger `config` holds a number | Emitted unquoted and untagged, character-identical to storage; never reformatted, rounded, quoted, or given an explicit `!!int`/`!!float` tag (AC-41) |
| Trigger `config` keys contain digits or punctuation | Sorted byte-wise via an explicit `MappingNode`, not by yaml.v3's map sorter (AC-8) |
| Trigger `config` holds a **string** whose text would resolve to another YAML type (`"true"`, `"null"`, `"1.0"`, `"0755"`, `"12"`, `"~"`) | Emitted with an explicit `!!str` tag, so yaml.v3 quotes it and it re-parses as a string; never retyped to bool, null, int or float (AC-8, AC-23) |
| Multi-line valid-UTF-8 prompt, emitted scalar **is** a literal block scalar (any chomping indicator) | Emitted as-is, **no** warning (AC-16) |
| Multi-line valid-UTF-8 prompt, emitted scalar is **not** a literal block scalar | Emitted faithfully in whatever form yaml.v3 chose, exactly one `prompt not emitted as a block scalar` warning; export succeeds. The condition is read from the emitted node's style, never from the prompt's characters (AC-17) |
| Multi-line prompt contains a TAB | Block scalar retained, **no** warning — a TAB does not clear `block_allowed`, and this row is a consequence of the style check rather than a special case in it (AC-16) |
| Prompt contains invalid UTF-8 | Emitted as `!!binary`, `invalid UTF-8` warning added, `200` — not a serialization failure; wins over AC-46 and AC-17 whether or not the prompt has a newline, and is the only reason emitted (AC-47) |
| Single-line non-empty prompt, valid UTF-8 | Emitted as YAML requires, **no** warning — a block scalar is unreachable without a newline, so this is not a degradation (AC-46) |
| Prompt is the empty string | `prompt` omitted entirely; no prompt AC applies and no warning is emitted (AC-4, AC-46) |
| Valid-UTF-8 prompt whose default emission does **not** decode back byte-for-byte (a prompt beginning with a newline is the reachable case) | Re-emitted as a double-quoted `!!str` scalar, which is probed to round-trip; exactly one `prompt re-quoted to preserve bytes` warning; AC-17 suppressed for that prompt so only one prompt warning is emitted; `200` (AC-49, AC-15, AC-16, AC-17) |
| A prompt that round-trips under neither the default nor the double-quoted form | `500`, no partial document — corrupt bytes are never committed. No probed input reaches this (AC-49, AC-36) |
| Prompt is invalid UTF-8 **and** would fail the fidelity test | AC-47 wins and is evaluated first; fidelity comes from base64-decoding, not from comparing the re-parsed scalar, whose value is the base64 text (AC-47, AC-49) |
| Multi-line prompt whose only line break is CR, U+0085, U+2028 or U+2029 | Treated as multi-line, since `## Definitions` fixes *newline* as YAML's five-character break set; emits non-literal, so exactly one `prompt not emitted as a block scalar` warning (AC-17, AC-46) |
| `workflow_id` set, `workflow_step_id` empty | `workflow` emitted with `name` only, **no** warning — nothing was referenced, so nothing is unresolved (AC-19, AC-4) |
| Two automations derive the same slug | Suffixed with `id` prefix, lengthened until unique (AC-27) |
| Authorizer returns a non-`ErrWorkspaceNotFound` error | `500` — not `404`; the sentinel is what distinguishes denial (AC-44 step 1, AC-45) |
| Workspace deleted between the existence check and the snapshot | `200` with an empty list; accepted race (AC-44) |
| Two same-named automations emit an identical warning | Both kept — dedup is never global (AC-42, AC-19) |
| Two same-type triggers on one automation both have bad `config` | Both warnings kept — AC-39 messages dedup per trigger, not per automation, so the count is not lost (AC-42, AC-39) |
| Automation name **or trigger type** contains a newline or control character | Escaped before interpolation so the warning stays one line. "Newline" is the five-character set, so U+0085 escapes as `\x85` and U+2028/U+2029 as `\u2028`/`\u2029`; without this a raw U+2028 survives into both the YAML scalar and `WARNINGS.txt`, where a reader honouring YAML's break set sees two lines (AC-42) |
| Automation name **or trigger type** contains an invalid UTF-8 byte | Escaped as `\xNN` before interpolation, so the warning stays a UTF-8 string scalar and does not become `!!binary` (AC-42) |
| A concurrent mutation lands while an export is in flight | Neither blocks the other; the export keeps its snapshot. Probed against both real DSNs (AC-30) |
| A mutation commits between two exports opening their transactions | Each export's *start* is its snapshot establishment, and AC-29 requires that to happen immediately on opening, so the window is one read wide and AC-13 is stated against a defined moment (AC-13, AC-29, AC-30) |
| Read transaction, query, or zip write failure | `500`, no partial body (AC-45, AC-48) |
| Serialization error | `500`, no partial body (AC-36, AC-48) |

## Persistence guarantees

The export performs no writes. No table is modified, no `last_evaluated_at` is touched,
no run row is created. Exporting an automation has no effect on when it next fires.

## Out of scope

Named exclusions, not omissions:

- **The applier (Phase 2).** Reading exported YAML back into the database is not
  specified here. It is additionally gated on an unresolved upstream question — whether
  `kdlbs/kandev` considers automations a git-syncable entity at all — tracked in Office
  discovery task `ba70553d`. If the answer is no, Phase 2 is a carried topic branch
  rather than an upstream contribution.
- **Two-way sync** (Kandev writing back to git).
- **A poller.** Nothing is fetched from a repository on an interval.
- **Migrating agent or executor profiles into git.** They stay database-resident; the
  export references them by descriptor, which is why AC-19 needs a warning path.
- **`automation_runs`.** Run history is observability, not definition, and is never
  exported.
- **Importing Office `ConfigBundle` routines.** A different entity in a different package.
- **Fixing the Office `reports_to` import defect.** Referenced as a caution; it has its
  own card.
- **Cross-workspace export.** Every endpoint is scoped to one workspace.
- **Secret rotation or a secret-bearing export variant.** `webhook_secret` has a
  dedicated reveal endpoint and stays there.
