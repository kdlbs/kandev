---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PLAN-INJECTION-BUDGET-001
  - REQ-AGENTS-PLAN-INJECTION-BUDGET-002
---

# Plan injection budget System Design Part 1

## Purpose and boundaries

The agent system owns the context a launching agent session receives. Two of its
surfaces inject a task's plan, and this design places a single reducer between
the plan and both of them.

Appendices A (measurement), B (prior art) and C (per-criterion notes) are in
[Part 2](plan-injection-budget-02.md); every bare "Appendix" reference below means one
of those.

Contracts this design uses but does not own:

- **The plan artifact** (`task_plans`, one row per task, whole-document replace)
  belongs to the task and workflow system. This design reads `Title` and
  `Content` through the existing `GetTaskPlan` and writes nothing.
- **`get_task_plan_kandev`** belongs to the MCP handler surface. This design
  only names it in the omission notice.
- **The `<kandev-system>` wrapper and prompt templates** belong to
  `internal/sysprompt`. This design adds no template and changes no prompt.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PLAN-INJECTION-BUDGET-001` | [Components and responsibilities](#components-and-responsibilities), [Control flow](#control-flow), [The reduction algorithm](#the-reduction-algorithm) |
| `REQ-AGENTS-PLAN-INJECTION-BUDGET-002` | [Call sites](#call-sites), [Preventing drift](#preventing-drift) |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| **Plan reducer** (new, backend-only) | Split a composed plan document into sections, drop from the middle until it fits, append the omission notice when it dropped anything. Pure function; no I/O, no clock, no shared state. |
| `injectHandoverIfNeeded` (`internal/orchestrator/executor/executor_execute.go`) | Compose the handover plan document from `plan.Content` VERBATIM (this site does not trim), apply containment, call the reducer with the handover budget, then wrap the reducer's output in the site's EXISTING frame — `"\nThe task has an implementation plan:\n\n%s\n"` — and pass that framed string as `planSection`, as it does today. The frame is outside the budget (AC-002.5). When the reducer returns nothing, `planSection` stays empty and no frame is emitted (AC-001.12). |
| `addDynamicPlan` (`internal/orchestrator/dynamic_launch.go`) | Compose `strings.TrimSpace(plan.Title + "\n" + plan.Content)`, call the reducer with the dynamic budget, assign to `ContinuationInput.PlanSummary`. |
| `bounded()` (`internal/agent/runtime/dynamic/conductor.go`) | Unchanged. Continues to bound every continuation field. For `PlanSummary` it is a no-op *given* two conditions the design must maintain — budget ≤ `continuationFieldLimit`, and no reducer-introduced surrounding whitespace — both asserted by the AC-002.4 test. |
| **Tag containment** (new, alongside the reducer) | Remove `<kandev-system>` and `</kandev-system>` literals until stable (AC-001.14). Called by the handover site only, before the reducer. |

No frontend component implements this outcome. There is no user-visible surface;
the only observable artifacts are the agent's injected prompt and the backend
log.

### Placement

The reducer is a new package under `internal/agent/` — it is agent-runtime
behaviour, and both consumers already depend on that subtree. It must not live
in `internal/sysprompt` (the dynamic site does not use system prompts) nor in
`internal/agent/runtime/dynamic` (the handover site must not depend on the
dynamic routing runtime to reduce a plan).

## Data and contracts

The reducer takes a composed document and a budget in bytes and returns the
reduced text plus exactly two facts: whether it reduced at all, and the number of
whole sections not represented in the output — AC-001.3's count, on every path.
Both are needed and neither follows from the other: a single-section document cut
by AC-001.7 reports a count of zero while having dropped content, so the boolean,
not the count, is what AC-002.6 keys its log on. Input and output sizes are NOT
returned — a caller measures those from the string it passed and the string it got
back — and a caller must never re-derive the count by parsing the notice. It
exposes no other surface: no configuration struct, no options, no interface, no
registry.

Two named constants carry the budgets. `continuationFieldLimit` is unexported in
`internal/agent/runtime/dynamic`, so the dynamic constant cannot be *defined* as
it from the reducer package; AC-002.4 therefore asserts the relationship from a
test inside that package, which is the only place both names are visible.

The reducer exposes one companion function, the containment of AC-001.14. It is
separate because it is not budgeting: it is applied by the handover site alone,
before the reducer runs, and the dynamic site never calls it.

The omission notice is a fixed English sentence carrying two independent facts —
a count of whole sections omitted out of the document's total, and a clause
present only when the retained section was shortened. It names
`get_task_plan_kandev`. It is backend prompt text for an agent, not product copy,
so it is not localized and does not enter the i18n catalogs.

The intra-section marker of AC-001.7 is a distinct fixed string, so a reader can
tell "whole sections were dropped from the middle" from "this one section was
cut short".

## Control flow

Both paths are synchronous and local to an already-running launch.

```text
launch → site reads plan (GetTaskPlan) → site composes plan document
       → reducer(document, site budget) → reduced text + stats
       → site wraps the reduced text in its own frame/prefix (outside the budget)
       → site injects → site logs stats when anything was dropped
```

Handover: `Execute` → `injectHandoverIfNeeded` → reducer →
`sysprompt.InjectSessionHandover(previousCount, planSection, prompt)` → the
plan text is placed inside the `<kandev-system>` block by the existing template.

Dynamic: `buildDynamicContinuation` → `addDynamicPlan` → reducer →
`ContinuationInput.PlanSummary` → `BuildBoundedContinuation` → `ContinuationPrompt`
→ downstream launch.

`bounded()` is left unmodified and becomes a no-op for this field only under two
conditions, both of which AC-002.4 makes explicit rather than assuming. Its length
check cannot fire, because the dynamic budget is at most `continuationFieldLimit`.
Its unconditional `strings.TrimSpace` cannot change the value either, because
AC-001.6's assembly rule forbids the reducer from emitting leading or trailing
whitespace that its input did not have. Without that second condition the call
would not be a no-op at all; it would silently re-trim the reducer's output.

### Call sites

Each site keeps its own composition step. This is deliberate and is what makes
AC-002.3 hold: because the reducer returns its input unchanged when it fits, an
under-budget plan produces byte-identical output to today at both sites, without
either site having to adopt the other's title handling.

The handover site inserts one extra step between composing and reducing —
containment (AC-001.14) — and the dynamic site does not. That asymmetry is one
of two documented exceptions to byte-identity, and is why AC-002.3 carries an
explicit carve-out rather than an unqualified guarantee. It is placed before the
reducer so that every guarantee the reducer makes is stated over the text that is
actually injected, and so that emptiness (AC-001.12) is judged after it.

### Preventing drift

REQ-002 exists because the defect being fixed is duplicated, divergent handling of
one artifact. Three things keep it from returning.

The reducer is the only place budgeting, section selection, or truncation is
implemented; neither call site carries any of the three (AC-002.1). Containment is
the one function that lives beside it rather than inside it, and it is shared code
called from one site, not inline logic at that site — so there is still exactly one
implementation of each behaviour.

The anti-bypass test (AC-002.2) exercises each site's injection path with an
over-budget plan and asserts the notice appears in what that site injects. It is
deliberately not a test of the reducer: asserting the reducer's behaviour proves
nothing about whether a site calls it, and a site that stopped calling it would
still pass a reducer-only suite.

The reducer holds no mutable shared state and reads no database (AC-001.15), so it
is safe under the concurrent launches the orchestrator already performs, and its
output for a given task cannot be perturbed by another task reducing at the same
moment. Each caller passes the snapshot it already read; the reducer neither
re-reads nor caches (AC-001.16). The symmetric guarantee is AC-001.4: when nothing
was dropped, no notice is emitted, so a plan that fits is never decorated with a
claim that something is missing.

### The reduction algorithm

Given a document and a budget:

The implementation reserves space for generated text before it selects source
sections. The notice reservation uses the widest notice for the document's section
count and one possible preceding newline. The marker reservation uses the marker
length and one possible preceding newline. These are upper bounds, so the final
assembly remains within the budget even when a conditional newline is not needed.

1. If the document is empty or only whitespace, return nothing at all — no text,
   no notice, no stats (AC-001.12).
2. If `len(document) <= budget`, return it unchanged with a zero drop count
   (AC-001.1). This is the only path most callers take for a small plan, and it
   allocates nothing.
3. Split into sections at lines beginning `## `, keeping any preamble as the
   first section (AC-001.11). Section order is fixed here and never changed
   (AC-001.6).
4. Compute the notice reservation once, now that the section count is known: the
   notice rendered at its widest for this document — both numbers set to the total
   section count, the `shortened` clause present — PLUS ONE BYTE for the conditional
   `\n` step 6 may place before it. Both parts are upper bounds, which is what makes
   AC-001.2 hold for every input without a second pass: the notice finally rendered can
   only be shorter and the separator can only be absent, so the finished output can only
   come in under the bound, never over it. Two ways to get this wrong, both producing an
   over-budget output the algorithm cannot recover from, there being no eviction or
   restart step: reserving a fixed-width count and then formatting a wider one, and
   reserving the notice WITHOUT its separator byte. Appendix C works both, and Testing
   notes says which fixture catches which.
5. Grow a tail run and a head run, considering candidates alternately with the
   tail run first. Retain a candidate that fits the budget remaining after the
   reservation and the sections already retained; when one does not fit, close
   that run permanently rather than skipping past it, so both runs stay contiguous
   (AC-001.5, AC-001.6). Stop when both runs are closed or the runs meet — which
   AC-001.6 defines as no unconsidered section remaining between them, and which is the
   ONLY thing that word means here. Closing rather than skipping is deliberate: skipping
   a section that does not fit and taking a smaller one behind it would use the budget
   slightly better and produce a patchwork whose holes the reader cannot locate.
   One trace exposes a sloppy loop and must hold. TWO sections whose last does not fit:
   the tail closes, and the head, still open, considers and retains S1. An implementation
   that stops because ONE run closed retains nothing, falls through to step 7, and
   shortens a section that never needed shortening.
   There is deliberately NO trace in which every section is retained, and none should be
   added. Step 5 runs only when the document exceeds the budget, so retaining every
   section would require `len(document) <= budget - reservation`; the output would then be
   the whole over-budget document carrying no notice, violating AC-001.2 and AC-001.4. The
   same arithmetic covers the last section still unconsidered when the runs meet — it is
   retainable only if every section is, so it never is. AC-001.6 still REQUIRES that it be
   considered, consideration being what makes the traversal fully determined and keeps it
   correct under a future budget or reservation change, but no trace can show a conforming
   loop retaining it.
6. If step 5 retained at least one whole section, emit them in original document order
   and append the notice, reporting the number of whole sections omitted out of the
   total (AC-001.3), then return. Assembly is exact (AC-001.6): sections are
   concatenated with no inserted separator: every section EXCEPT the document's last
   ends at a `## ` line and so already carries its trailing newline. The last section
   is the one that may not — the dynamic site's `TrimSpace` guarantees it does not, the
   handover site passes content verbatim — but it cannot strand a join, because it can
   only land at the END of the output, which the next rule covers. Tail-first retains
   it FIRST on every reduction, so that is the common case, not an edge one. The notice
   is preceded by a single `\n` only when the text so far does not already end in one:
   the byte step 4 reserved. The output carries no trailing newline; that rule is not
   cosmetic, it is the condition that keeps `bounded()` a no-op.
7. If step 5 retained nothing, fall back to the single intra-section path: take leading
   whole lines of the **first** section while they fit the budget less BOTH the notice
   reservation AND the marker reservation — each carrying its own conditional separator
   byte — then append the marker, then the notice, and return (AC-001.7). A line is what
   Terminology says it is, its terminator included. The retained prefix is always
   strictly shorter than the section: step 7 runs only because step 5 could not retain
   that section whole against a LARGER remaining budget, so its final line is never
   among the lines taken and the retained text always ends with `\n`. The separator
   before the marker is therefore never spent on this path; the one before the notice
   always is, the marker not ending in a newline. Both are reserved regardless — see
   Appendix C for why an upper bound is what AC-001.2 needs.
   The notice reports both facts independently. `{omitted}` is the number of whole
   sections NOT REPRESENTED AT ALL, which on this path is `{total} - 1`: the first
   section is present but shortened, and every other section was dropped whole. It is
   zero ONLY when the document had one section to begin with. Reporting a blanket zero
   here would tell the agent nothing was omitted while `{total} - 1` sections are gone,
   which is the failure REQ-001's Intent exists to prevent. The `shortened` clause
   carries the separate fact that the retained section was cut.
8. If not even one complete line fits after those two reservations — separators
   included — return no plan text at all (AC-001.13): never a notice with nothing
   attached, never a marker with nothing attached, never an over-budget output. With the
   shipped constants this band is unreachable; it is defined so the function is total.

Determinism (AC-001.9) follows from the algorithm reading only the document and
the budget: no map iteration, no randomness, no clock, no heading text
interpretation. Headings are located, never read.

Idempotency (AC-001.10) needs care and is the one place a naive implementation
will fail the criteria. Feeding the reducer's output back in presents a document
whose last section is the notice. The implementation must return such an input
unchanged when it fits, which step 2 already does, since the output is by
construction within budget. The test for AC-001.10 must therefore use an input
that actually reduced, not merely a short one.

Rune safety (AC-001.8) follows from cutting only at line boundaries and section
boundaries, both of which fall on rune boundaries in valid UTF-8. No code path
slices a string at an arbitrary byte offset. This is the specific defect in
today's `bounded()`, which does `value[:continuationFieldLimit]`.

### Why the fallback reduces the first section

When nothing whole fits, the opening is the only position guaranteed to carry the
document's framing; leading lines of a later section would arrive detached from
their heading. The trigger is *nothing whole fits*, never *some section is
oversized*; Testing notes works that trap and the fixture that catches it.

### Why head and tail rather than head alone

A plan is appended to over a task's life, so its tail holds current state — the
latest verdict, open findings, the PR number — its head holds framing, and its
middle is where superseded working notes accumulate. Keeping both ends costs the
same budget as keeping one and loses the least useful part. Today's bound keeps
the head alone (Appendix A). The tail-first tie-break in AC-001.6 follows: when
only one more section fits, current state is worth more than more framing.

## Failure and recovery

- **Plan read fails.** Each site keeps its existing policy unchanged (an
  explicit exclusion in the requirement). The reducer is not reached.
- **Plan missing, empty, or whitespace.** The reducer returns nothing when the
  document it receives is empty or whitespace-only, and the launch proceeds
  (AC-001.12). Because each site composes its own document, one input splits them:
  a plan with a set title and whitespace content composes to a non-empty document
  at the dynamic site, which still injects the bare title as today, and to a
  whitespace-only one at the handover site, which injects no plan section. The
  latter is a deliberate change — today's `plan.Content != ""` guard passes on
  whitespace and frames it — and is the second of AC-002.3's named exceptions.
- **Budget too small for the notice, or non-positive.** Return no plan text
  rather than an over-budget output or a bare notice (AC-001.13). This cannot
  arise from the shipped constants; it is defined so the function is total.
- **Malformed markdown.** There is no failure mode. Unbalanced fences, headings
  inside code fences, and `##` at a line start within a fenced block are all
  treated as section boundaries. This is a deliberate simplification: a
  false-positive boundary can only change where a cut lands, never whether the
  output is valid or within budget. Tracking fence state would add a parser to
  buy nothing the criteria require.
- **The reducer cannot fail.** It returns no error. A launch is never blocked,
  degraded, or retried because of plan reduction.

## Persistence

None. This design stores nothing, caches nothing (AC-001.16), and runs no
migration. It reads `task_plans` through the existing `GetTaskPlan` and holds
the result only for the duration of one launch.

## Security

Plan content is authored by agents and is untrusted as prompt input. It is
injected inside the `<kandev-system>` block on the handover path, so text that
closes or forges that block would let plan content escape the system context and
present itself as trusted framing.

`sysprompt.StripTags` removes only `</kandev-system>`; one stored plan already
contains a `<kandev-system>` start tag and none contains an end tag. AC-001.14
requires containment of both directions, and requires ONE combined replace-until-stable
loop rather than two independent per-literal ones. A single pass is not enough: one pass
over `<kandev<kandev-system>-system>` reconstitutes a live start tag. Two per-literal
loops of the shape `StripTags` uses are not enough either, and that is the sharper trap —
`StripTags` loops over one literal because it has only one to remove, whereas containment
has two that can build each other. Given `<kandev` + `</kandev-system>` + `-system>`, a
start-only pass finds no start literal and changes nothing, and the end-only pass removes
the end literal and leaves a live start tag its own stop condition never rechecks.
Reversing the two loops does not help — it only moves the hole, as the mirror input
`</kandev` + `<kandev-system>` + `-system>` shows, leaving a live end tag by the same
argument. Every pass must therefore rescan for BOTH literals, stopping only when neither
is found.

Containment is **not** a no-op on the measured corpus: it changes the injected
bytes of exactly the one plan that carries the start tag. That is a deliberate
carve-out, decided rather than assumed away. AC-002.3 records it as one of two
exceptions to byte-identity under budget, and AC-001.1 states byte-identity
relative to the document the reducer receives, which on the handover path is the
contained document.

The carve-out is scoped to the handover site only. The dynamic site does not wrap
plan text in a `<kandev-system>` block — nothing in `internal/agent/runtime/dynamic`
references the tag — so containing there would change injected bytes for no
security gain. Matching is case-sensitive and exact-literal, so `<KANDEV-SYSTEM>`
and `< kandev-system >` are not removed; that limit is stated in the requirement's
Out of scope (Part 2), because broadening the match would rewrite legitimate plan prose
that discusses the tag.

The omission notice is generated, never derived from plan content, so a plan
cannot forge or suppress it. Sizes and counts are the only plan-derived values
that reach the log; no plan text is logged.

## Observability

AC-002.6 requires a log on reduction, carrying the site, task, input size, output
size, and sections omitted. The two sites are not symmetric here, and AC-002.6 is
therefore written per site rather than as one sentence about existing behaviour.
The handover site already emits an `Info` record at injection ("injecting session
handover context") and extends it with these fields. The dynamic site emits no log
at injection at all — `addDynamicPlan` contains no logging — so it gains a new
`Info` record. Treating both as "already logging" would have left the dynamic
site's reductions invisible.

No expvar counter is added. The existing `routing_*` and `workflow_*` families
answer different questions, and there is no requester for a metric here; the log
is sufficient to confirm the bound is engaging and to see how often plans exceed
it.

## Testing notes

The criteria that will be got wrong without deliberate tests:

- **AC-001.1 / AC-002.3** — byte-identical output under budget, asserted per
  site with each site's own composition, not just on the reducer.
- **AC-001.10** — idempotency asserted on a document that actually reduced.
- **AC-002.4** — the dynamic budget's relationship to `continuationFieldLimit`
  asserted directly, so raising one without the other fails the build.
- **AC-002.2** — the anti-bypass test. Asserting the reducer's own behaviour
  does not show that a site calls it; the test must exercise each site's
  injection path with an over-budget plan and assert the notice appears.
- **AC-001.14's containment loop** — THREE fixtures, and all three are needed. The
  single-literal `<kandev<kandev-system>-system>` catches an implementation that does not
  repeat at all. The other two catch the likelier defect, two independent per-literal
  loops, and ONE OF THEM IS NOT ENOUGH because each ordering fails on a different input:
  `<kandev` + `</kandev-system>` + `-system>` survives a start-then-end pair as a live
  `<kandev-system>` but is cleared by end-then-start, and the mirror
  `</kandev` + `<kandev-system>` + `-system>` survives end-then-start as a live
  `</kandev-system>` but is cleared by start-then-end. A suite carrying only one of the
  pair therefore passes a broken implementation half the time, depending on which order
  the builder happened to write. The combined loop returns the empty string for all
  three; assert that, not merely that no literal remains.
- **AC-001.13** — a budget smaller than the notice, which the shipped constants
  never produce and a hand-written test must construct.
- **AC-001.2 needs BOTH a narrow and a wide section count**, because the two ways
  to exceed the budget are caught at opposite ends of the range and a suite that
  tests only one ships the other green. Assert `len(output) <= budget` on both.
  - *Wide count.* The notice's length varies with the number it carries, so an
    implementation that reserves room for a fixed-width count and then formats a
    wider one goes over. A document with enough sections to force a three-digit
    count catches this; a two-section document does not.
  - *Narrow count, on the intra-section path.* A reservation that omits the
    separator byte of AC-001.6 goes over by exactly one byte, but only when the
    reserved notice is no wider than the rendered one. On this path the reserved
    notice carries `{total}` where the rendered one carries `{total} - 1`, so the
    slack is one byte when the section count is a power of ten and zero for every
    other count. The defect is therefore invisible at N=10 and N=100 and visible at
    N=2, N=3, N=11, N=101. Do not leave this to the wide fixture above: N=100 is the
    roundest way to force three digits and is exactly a count that hides it. Require
    a small-N case explicitly. Appendix C works the arithmetic. The fixture needs no
    special construction beyond a small section count and nothing whole fitting: the
    separator this catches is the one before the NOTICE, and step 7 always spends it,
    because the marker does not end in a newline. Do not try to build a case whose
    retained text ends without a `\n` — step 7 shows that cannot happen on this path,
    and an implementation that produced it would be cutting mid-line.
- **AC-001.7** — the trigger is that *nothing whole fits*, not that some section
  is oversized, and the two must be tested apart. The positive case is a document
  where no whole section fits, so the intra-section path runs. The negative case is
  the trap: a document whose first section is far larger than the budget but whose
  last section is small must retain that last section under step 5 and must **not**
  reach the fallback. A suite that only asserts "oversized section ⇒ marker appears"
  passes against an implementation that wrongly reduces the first section whenever
  it is oversized, which would discard current state on 71% of stored plans.
- **AC-001.13** on the intra-section path — a budget that holds the notice but not
  the notice and marker together, and one that holds both but not one complete
  line. Both must return nothing rather than a bare notice or marker.
- **AC-001.3's omitted count on the intra-section path.** Nothing else in this
  list reaches it: every other notice assertion runs on the whole-section path,
  where the count is the number of dropped sections and the obvious
  implementation is right. On the fallback the obvious implementation — report
  zero, because no section was dropped *whole* — is wrong. Give the test a
  multi-section document where no whole section fits, and assert the notice
  carries `{total} - 1`, not zero. A single-section document is the degenerate
  case where zero IS correct, so it cannot stand in for this.

Real plan shapes are available from the measurement in the requirement; fixtures
should include a plan with no `## ` line at all, one with a preamble, and one
whose single largest section exceeds the dynamic budget.

## Related decisions

No ADR is required. This design adds one internal pure function and changes no
architecture, public contract, persistence boundary, or security boundary.
