---
status: draft
system: typed-workflow-state
created: 2026-09-01
owners:
  - kandev
---

# Review findings access requirements

Part 2 of the typed workflow review state system: MCP read and resolve tools over
`task_review_findings`, which is write-only today. System-wide terminology,
non-functional constraints and exclusions are in [../README.md](../README.md).

## Requirements

### REQ-TWS-003: MCP read access to a task's review findings

An agent can read the findings published for a task through MCP, under the same
reach rule that governs publishing.

- **AC-TWS-003.1:** A tool named `list_review_findings_kandev` shall be
  registered in the same `review` tool group as the publisher, and shall
  therefore be available on exactly the same surface.
- **AC-TWS-003.2:** `task_id` shall be optional and shall default to the calling
  session's task, matching `publish_review_findings_kandev`.
- **AC-TWS-003.3:** The tool shall call the same task authorizer
  `PublishFindings` calls, with the resolved `task_id`, before reading anything. A
  task outside the caller's reach shall be **rejected with an authorisation
  error**, never silently redirected to the caller's own task. The backend action
  shall use the existing `ws.ErrorCodeForbidden`, and the tool shall surface that
  error's message. The publisher maps an authoriser failure to an internal error
  today; the new actions are not required to reproduce that, and this card does not
  change the publisher.
- **AC-TWS-003.4:** The response shall be a JSON object carrying `findings` (the
  array), `total_matched` (matches before the cap) and `truncated` (boolean),
  rendered into the tool's text result per AC-TWS-003.13. All three keys shall
  always be present, so a caller never infers truncation from a length. `findings`
  shall be rendered as an empty JSON array `[]` when nothing matched, never as
  `null`. Go marshals a nil slice as `null` and `ListTaskReviewFindings` returns a
  nil slice for zero rows, so the direct implementation emits `null` and forces
  every caller to handle two encodings of "no findings"; AC-TWS-003.9 requires an
  empty list, and this pins what empty looks like on the wire. For each returned
  finding the response shall carry `id`,
  `run_id`, `repository_name`, `file_path`, `start_line`, `end_line`, `side`,
  `severity`, `category`, `title`, `body`, `suggestion`, `status`, `resolved_at`,
  `created_at`. A finding published through `publish_review_findings_kandev` shall
  round-trip every field it set. A `line_end` the publisher defaulted to the start
  line round-trips as that start line, not as `0`. Every listed key shall be present
  on every returned finding, including when its value is empty. The response is a
  **spec-defined shape, not `models.TaskReviewFinding` marshalled directly**: that
  model tags `ResolvedAt` as `json:"resolved_at,omitempty"`, which drops the key
  entirely for an unresolved finding and would therefore omit `resolved_at` from
  every result under the AC-TWS-003.5 default of `open`. `resolved_at` shall be
  rendered as JSON `null` when the finding carries no resolution timestamp, never
  omitted, **so that every finding carries the same key set whatever its status**.
  Stable shape is the whole reason, and it is sufficient on its own: a caller reads
  `resolved_at` and compares it to `null` rather than testing whether the key exists
  and branching on a schema that shifts under it. It is explicitly **not** a
  distinguishability rule — a finding reopened under AC-TWS-004.5 has its
  `resolved_at` cleared, so it is indistinguishable here from one never resolved, and
  `updated_at` is deliberately outside this field set. Telling those two apart is not
  a capability this tool offers and no AC requires it. `created_at`, and
  `resolved_at` when non-null, shall be
  rendered as RFC 3339 strings — the encoding Go's `encoding/json` already gives
  `time.Time`, named here because declaring a spec-defined shape otherwise leaves the
  format to the implementer.
- **AC-TWS-003.5:** An optional `status` argument shall accept `open`,
  `resolved`, `dismissed`, or `all`. It shall default to **`open`**, because the
  consuming caller asks "what is still outstanding".
- **AC-TWS-003.6:** An optional `severity` argument shall accept `blocker`,
  `major`, `minor`, or `nit` and shall restrict results to that severity. Omitted,
  it shall not restrict.
- **AC-TWS-003.7:** `status` and `severity` shall be normalised before matching by
  trimming surrounding whitespace and lower-casing, matching
  `review.NormalizeFindingInput`. A value absent, JSON `null`, or empty after
  trimming shall be treated as **omitted**, so the AC-TWS-003.5 default applies. A
  non-empty value unrecognised after normalisation shall be **rejected with a
  validation error naming the accepted values**. Such a value shall not be ignored
  and shall not fall back to the default: a typo that silently widened the result
  set would return resolved findings to a caller asking for open ones. This does not
  override the omitted-value rule above.
- **AC-TWS-003.8:** Findings shall be returned ordered by `repository_name`, then
  `file_path`, then `start_line`, then `id` — every component a named column,
  matching `ListTaskReviewFindings`. `id` is a UUID, so rows identical on the first
  three tie-break in an arbitrary but **stable** order. This is the existing
  repository ordering and is deliberately not changed here.
- **AC-TWS-003.9:** A task with no findings, or none matching the filters, shall
  return an **empty list and a success result** — not an error and not a
  not-found.
- **AC-TWS-003.10:** The response shall be capped at **100 findings**, retaining
  the **first 100 in the AC-TWS-003.8 order** so that a truncated response is
  itself deterministic. When the cap truncates, the response shall state the total
  number that matched in `total_matched` and set `truncated` to true. When the cap
  does **not** truncate — including when nothing matched, and including the exact
  boundary case of exactly 100 matches, which does not truncate — `truncated` shall be
  **false** and `total_matched` shall be that same match count, which absent a
  concurrent publish is the length of `findings`. AC-TWS-005.4 accepts a skew
  between the total and the page under a publish committing mid-call and is not
  weakened here. Both directions are stated because AC-TWS-003.4 promises that a
  caller never infers truncation from a length: that promise holds only if
  `truncated` is authoritative when false as well as when true. Left one-way, an
  implementation that set `truncated` unconditionally would violate no rule while
  breaking exactly the guarantee the key exists to give. Truncation shall never be
  silent.
- **AC-TWS-003.12:** An explicitly empty `task_id` shall be treated as omitted and
  resolved to the calling session's task, matching `publish_review_findings_kandev`
  (`resolveTaskID`). With no `task_id` and no session task, the call shall fail with
  the same `task_id is required` validation error the publisher returns.
- **AC-TWS-003.11:** Findings for the whole task shall be returned, across every
  review run — the store is task-scoped and a caller wants all of them, not one
  run's.
- **AC-TWS-003.13:** The **successful** text result of **both** tools in this group
  shall be the JSON document alone, with **no leading prose and no trailing prose**,
  so that the whole text result parses as JSON. Rendering shall otherwise match
  `publish_review_findings_kandev` — indented JSON in a text result — but the
  absence of a prefix is a deliberate **departure** from it: that tool prefixes
  `"Review findings published:\n"` because it returns a two-key acknowledgement
  nobody parses, whereas these two return the data the consuming step acts on, and a
  prefix would make every caller strip a fixed string before decoding. This AC exists
  because "rendered as the publisher renders its own" is ambiguous between the
  mechanism and the prefix, and the two readings produce different tests. It governs
  **success results only**: the error results required by AC-TWS-003.3,
  AC-TWS-003.7, AC-TWS-003.14, AC-TWS-004.8 and AC-TWS-004.12 are human-readable
  messages and shall not be forced into JSON.
- **AC-TWS-003.14:** When reading the findings fails — `ListTaskReviewFindings`
  returning its wrapped query or row-iteration error, or any other persistence
  failure — the tool shall return an **error result naming the failure**. It shall
  **not** return a success carrying an empty `findings` array. AC-TWS-003.9's empty
  success is reserved for a query that ran and matched nothing; a read that did not
  run has not established that anything is absent. Reporting "no outstanding
  findings" to a review step because the store was unavailable is the same
  silent-wrong-answer failure REQ-TWS-002 exists to prevent on the Part 1 side, and
  Part 2 shall degrade as visibly. A persistence failure shall be distinguishable by
  the caller from both the empty result and the AC-TWS-003.3 authorisation error.

### REQ-TWS-004: MCP resolution of a review finding

A finding an agent has addressed can be closed through MCP. Without this, the
ledger has no close path and a caller must track dispositions in prose — the
substrate this card exists to leave.

- **AC-TWS-004.1:** A tool named `resolve_review_finding_kandev` shall be
  registered in the same `review` group, taking a required `finding_id` and a
  required `status` whose accepted values are exactly `open`, `resolved` and
  `dismissed`. The tool's schema enum and AC-TWS-004.8's validation error shall
  name those three and no others.
- **AC-TWS-004.2:** `open` shall be accepted, so a finding closed in error can be
  reopened; refusing it would make an incorrect resolution unrecoverable through
  MCP.
- **AC-TWS-004.3:** Authorisation shall be performed against **the owning task
  read from the finding row**, not against any caller-supplied task identifier. A
  caller shall not be able to resolve a finding on a task outside its reach by
  naming a task it can reach.
- **AC-TWS-004.4:** An unknown `finding_id`, and a `finding_id` that exists on a
  task outside the caller's reach, shall return the **same** not-found error, so
  the tool cannot be used to test whether an identifier exists. This differs
  deliberately from AC-TWS-003.3, where `list` returns an authorisation error: there
  the caller names a `task_id` it already holds, so refusing discloses nothing,
  whereas `finding_id` is an opaque global identifier an authorisation error would
  confirm the existence of.
- **AC-TWS-004.5:** **Changing** a finding to `resolved` or `dismissed` shall stamp
  `resolved_at`; changing it to `open` shall clear `resolved_at`. When the submitted
  status equals the stored one, AC-TWS-004.6 governs instead and this AC does not
  apply.
- **AC-TWS-004.6:** Re-submitting the status a finding **already has** shall
  succeed, shall return the finding, and shall **leave `resolved_at` unchanged**.
  `resolved_at` records when the disposition was made; a retried call is not a new
  disposition, and a retry must not move a timestamp.
- **AC-TWS-004.7:** The tool shall resolve exactly one finding per call.
- **AC-TWS-004.8:** An empty `finding_id`, a missing `status`, or a `status`
  outside `open` / `resolved` / `dismissed` shall be rejected with a validation
  error naming the accepted values, mirroring AC-TWS-003.7. No such call shall
  reach persistence.
- **AC-TWS-004.9:** A successful resolution shall publish the existing
  `TaskReviewFindingUpdated` event, exactly as the UI update path does, so the
  Review panel converges without a reload. The tool shall go through
  `ReviewService`; it shall not write the repository directly. This includes the
  idempotent no-op of AC-TWS-004.6: the event carries the whole finding as a state
  snapshot rather than a delta, so a redundant publish sets every subscriber to the
  state it already holds, and publishing unconditionally keeps one code path.
- **AC-TWS-004.10:** The AC-TWS-004.6 comparison shall be added to the existing
  shared `ReviewService.UpdateFindingStatus`, not to a parallel method. That method
  stamps `resolved_at` on every call today, so this **deliberately changes the
  browser action `task.review.finding.update` as well**: a second UI resolve of an
  already-resolved finding stops moving the timestamp. Re-stamping was wrong on both
  surfaces, and forking the method would leave two stamping rules to keep in step.
  This authorises no change to that action's authorisation (Out of scope 8).
- **AC-TWS-004.11:** A successful resolve shall return the finding using the
  AC-TWS-003.4 field set **and its representation rules**, `resolved_at` included as
  `null` rather than omitted, so one shape describes a finding across this tool
  group. The result shall be a JSON object whose single key `finding` holds that
  object — the same envelope the existing `task.review.finding.update` action already
  returns, rather than the finding bare — and shall be rendered into the tool's text
  result under AC-TWS-003.13, which governs both tools in this group. Naming the key
  leaves room for a sibling key later; a bare object does not, and the two shapes are
  both already present in the tree, so leaving the choice open would decide it
  arbitrarily.
- **AC-TWS-004.12:** A **persistence failure** shall be reported as such and shall
  **not** be folded into AC-TWS-004.4's not-found. The not-found result is reserved
  for two conditions and no others: the finding row does not exist, or it exists on
  a task the caller cannot reach. The two are recognised differently and only then
  converge on one response. A missing row shall be recognised by matching the
  existing sentinel `models.ErrTaskReviewFindingNotFound` (re-exported as
  `service.ErrReviewFindingNotFound`) with `errors.Is` — the repository already
  wraps that sentinel only for `sql.ErrNoRows` on the read and only for a zero-rows
  result on the write, returning every other failure unwrapped, so the sentinel is a
  reliable discriminator and a catch-all on error is not. An unreachable task shall
  be recognised by the authorizer denying it, and is then **mapped** to the same
  not-found response by AC-TWS-004.4. A query, write or read-back error is neither
  condition and shall surface as a distinct error result, never as not-found.
  Without this rule, AC-TWS-004.4's requirement that unknown and unreachable
  identifiers be indistinguishable pushes the implementation toward a catch-all on
  any read failure, which would tell an agent a finding does not exist whenever the
  store is unavailable — and an agent acting on that would treat a live finding as
  gone. The catch-all is the natural way to satisfy AC-TWS-004.4, which is exactly
  why the narrower rule has to be stated. This rule covers the **repository** only.
  The task authorizer is a `func(ctx, taskID) error` seam that returns one
  undifferentiated error for a denial and for its own infrastructure failure, so any
  error from it shall continue to be treated as a reach denial and mapped to
  AC-TWS-004.4's not-found. Separating those two would mean changing that seam's
  signature, which this card does not do; the boundary is stated so a builder does
  not read "persistence failure" as reaching the authorizer.


## Verification

One line per case; the cited AC is authoritative for the expected value.


1. Publish then list round-trips every 003.4 field, a defaulted `line_end`
   returning as the start line, and `resolved_at` present as JSON `null` on an
   `open` finding rather than absent from the object. The whole text result is
   decoded with a strict JSON parser and no prefix is stripped first, so a
   reintroduced prose prefix fails the case (003.4, 003.13).
2. `status` filters return the right subsets for `open`, `resolved`, `dismissed`
   and `all`, on a task holding all three (003.5).
3. `severity` filters correctly; `status` + `severity` intersect (003.6).
4. `"OPEN"` and `" open "` are accepted as `open`; `""` and JSON `null` are treated
   as omitted and take the default; a non-empty unrecognised value is rejected with
   an error naming the accepted values. The same three assertions are made
   **independently for `severity`**: `"BLOCKER"` and `" blocker "` accepted as
   `blocker`; `""` and JSON `null` treated as omitted, which for `severity` means
   the filter does not restrict rather than applying a default, since 003.6 gives it
   none; and a non-empty unrecognised severity rejected with an error naming
   `blocker` / `major` / `minor` / `nit`. 003.7 governs both arguments, so an
   implementation that normalises `status` only must fail this case (003.7, 003.6).
5. An out-of-reach task is rejected on both tools — `ErrorCodeForbidden` on list —
   and the response carries no finding from the caller's own task. The list tool's
   error text shall carry the message the task authorizer returned, asserted against
   an authorizer fake denying with a distinctive message, so a hardcoded or generic
   denial string fails the case (003.3, 004.3).
6. No findings → success result whose `findings` is `[]`, asserted against the
   raw JSON so `null` fails the case, carrying `total_matched: 0` and
   `truncated: false` (003.9, 003.4, 003.10).
7. 101 findings return 100, `truncated: true`, `total_matched: 101`, the 100 being
   the first in 003.8 order (003.10).
8. Ordering matches 003.8 for a set differing on each of the four columns.
9. An untruncated response carries `truncated: false` and a `total_matched` equal
   to the number of findings returned — the VALUES asserted, not merely the keys
   present, so an implementation that sets `truncated` unconditionally fails. Run
   against a quiescent store so AC-TWS-005.4's accepted skew does not apply, and
   covering the exact boundary of 100 matches as well as a short result
   (003.10, 003.4).
10. Resolve stamps `resolved_at`; reopening clears it (004.5).
11. Re-resolving with the same status succeeds, leaves `resolved_at`
    byte-identical, and still publishes `TaskReviewFindingUpdated` (004.6, .9).
12. An unknown `finding_id` and one on an unreachable task return the **same**
    not-found error, compared byte-for-byte so no oracle reappears (004.4).
13. Empty `finding_id`, missing `status`, garbage `status` — each rejected before
    any write (004.8).
14. A successful resolve returns the 003.4 field set under a single `finding` key,
    its whole text result parsing as JSON with no prefix stripped, with
    `resolved_at` `null` after a reopen (004.11, 004.5, 003.13).
15. Omitting `task_id` with no session task fails with `task_id is required`
    (003.12).
16. `open` is accepted by the schema and the validation error names all three
    statuses (004.1, .2).
17. The UI action `task.review.finding.update` also stops re-stamping on a repeated
    identical status (004.10).
18. A list call whose repository read fails returns an **error result**, not a
    success with an empty `findings` — asserted against a repository fake that
    returns an error, and distinguishable from both the case-6 empty success and
    the case-5 authorisation error. The message shall name the failed read rather
    than being a bare failure string, so a generic `"failed"` fails the case
    (003.14).
19. A resolve whose read, write or read-back fails returns an error that is **not**
    the case-12 not-found string, asserted against a fake failing at each of the
    three points; and an unknown id still returns not-found, so the two paths are
    shown to be separated rather than merely renamed (004.12, 004.4).
20. A task whose findings span **two distinct `run_id`s** returns findings from both
    runs in one response, asserted on the `run_id` values present rather than on the
    count alone, so an implementation scoped to the latest run — or to any single
    run — fails the case (003.11).

