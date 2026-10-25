---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PLAN-INJECTION-BUDGET-001
  - REQ-AGENTS-PLAN-INJECTION-BUDGET-002
---

# Plan injection budget System Design Part 2

Appendices to [Part 1](plan-injection-budget-01.md), which carries the design itself.
Part 1 is normative-adjacent prose about how the reducer works; this part is the
evidence behind it — the measurement the criteria cite, the prior art the shape was
taken from, and the per-criterion arithmetic that would otherwise crowd the
requirement.

## Appendix A: measurement

From the live Kandev SQLite database at `~/.kandev/data/kandev.db`, read-only,
2026-09-01. The requirement carries the rows the criteria cite; this is the full
table and what each row decides.

| Quantity | Value | What it decides |
| --- | --- | --- |
| Plans stored (`task_plans`, `UNIQUE(task_id)`) | 178 | Corpus size; one plan per task |
| Plan content: min / median / max characters | 1,209 / 17,448 / 122,215 | The handover budget's ceiling |
| Plans exceeding 4,000 bytes composed | 160 of 178 (90%) | The dynamic bound engages on almost every plan |
| Plans with one section larger than 4,000 characters | 127 of 178 (71%) | Whole-section granularity cannot always fit, so AC-001.7 must exist |
| Share of plan surviving today's 4,000-byte cut | median 18.2%, min 3.3% | How much is lost today, and from which end |
| Plans containing multibyte runes, among those cut | 160 of 160 (100%) | Byte slicing is not rune-safe, so AC-001.8 is not theoretical |
| `## ` heading occurrences across the corpus | 2,348 | Heading text is uncontrolled, so it is never interpreted |
| Distinct workflow step names | 39 | Step names are installation-defined, so they cannot key selection |
| Plans containing a `<kandev-system>` literal | 1 | Containment is not a no-op; hence the AC-002.3 carve-out |

Three consequences drive the criteria. Today's dynamic bound keeps a median 18.2%
of the plan and all of it from the head, while a plan's newest material is at its
tail — hence head *and* tail. 71% of plans hold a section larger than the dynamic
budget, so a whole-section-only reducer would frequently return nothing — hence the
intra-section fallback. And every cut plan contains multibyte runes, so a byte
slice would corrupt output on real data, not only in principle.

The handover site applies no bound at all, re-injecting on the order of 16M
characters (roughly 4M tokens) across 421 eligible launches.

The heading row counts occurrences, not distinct strings; a re-measurement on
2026-09-02 (181 plans, the corpus having grown by three) gives 2,320 occurrences
against 1,846 distinct, so headings do repeat across plans. The conclusion is
unchanged either way — roughly ten distinct headings per plan is uncontrolled
vocabulary, and AC-001.9 locates headings as cut boundaries without reading them.
Every other row in this table is the 2026-09-01 snapshot the criteria cite and is
not re-baselined here.

## Appendix B: prior art, in full

**Receipt (our own prior reasoning).** The QMD `wiki` collection (443 documents),
searched semantically — the vector index was present and current, so this was not
the silent keyword fallback. Two queries: context-injection budgeting and omission
signalling; preloading versus fetching on demand.

The governing hit is `concepts/artifact-write-api.md` (`lifecycle: draft`,
2026-09-01), which measures the same production database and reaches the same
live-content total. Two of its positions bind this design. **"Control flow parsing
the artifact" is an anti-pattern** — "a text-derived state machine does not merely
cost tokens; it returns wrong answers silently" — which is why headings here are
located as cut boundaries and never read. **"Heuristic truncation guard as a
stand-in" is an anti-pattern**, because it "cannot tell deliberately smaller from
read stale"; this design does not guess whether a plan is complete, it states
in-band when it dropped something.

**Receipt (what others shipped).** `saas-kb` `search_fsm_docs`, `category:
"ai_sdlc"`, two queries covering session-handover injection budgets and
context-window compaction. Best relevance across both was 0.016 — the flat, low
distribution that corpus produces when nothing matches, so this is a genuine
negative result rather than a failed search. No platform in that slice documents
bounding a **durable artifact** re-injected into a new session; they document
*conversation* compaction, which is a different problem and is not adopted: a
conversation is consumed once and may be summarised lossily, whereas a plan is a
durable record the agent can re-fetch in full.

One primitive transfers. OpenHands' `maybe_truncate()` "keeps the head and tail of
the content to preserve context at both ends" and inserts a caller-supplied
`truncate_notice` at the cut. That shape is adopted — head and tail, plus an
in-band notice — minus its saved-file reference, since `get_task_plan_kandev`
already returns the full plan and no file needs writing.

## Appendix C: notes on individual criteria

Rationale that would otherwise crowd the requirement:

- **AC-001.12, on reachability.** The whitespace-content row is reachable, not
  hypothetical: `content` defaults to the empty string and no constraint prevents
  it. Judging emptiness on the `content` column rather than on the composed
  document would require a call site to carry its own plan logic, which AC-002.1
  forbids; the site asymmetry it produces instead is stated under Failure and
  recovery.
- **AC-001.13, on the shipped constants.** The four bands the criterion covers — a
  non-positive budget, one below the notice reservation, one holding the notice but
  not notice-plus-marker, and one holding both but not a single line — are all
  unreachable at 12,000 and 4,000 bytes. The criterion exists so the function is
  total and so a future budget change cannot produce an undefined case.
- **AC-002.5, on the handover value.** 12,000 bytes caps that site's worst observed
  injection of 122,215 characters at roughly a tenth, while leaving 22% of stored
  plans untouched. The dynamic value is unchanged at 4,000, so this capability
  loosens no bound that exists today.
- **AC-002.5, on what the budget measures.** Each budget bounds the reducer's
  output — the plan document alone — not the text a site wraps around it. The
  handover frame (`"\nThe task has an implementation plan:\n\n%s\n"`) and the
  `<kandev-system>` wrapper lie outside the 12,000; the dynamic `"Plan summary: "`
  prefix lies outside the 4,000. Tests must assert against the reducer's output,
  not the wrapped prompt, or they will measure the frame and fail for the wrong
  reason.
- **AC-001.2, the reservation arithmetic, worked.** With the pinned literals the
  notice is 161 bytes when both counts are single-digit and the `{shortened}`
  clause is present, and the marker is 32 bytes. Take the dynamic budget of 4,000
  and a three-section document on which nothing whole fits, so the intra-section
  path runs.

  Reserving the notice and marker *without* their separators leaves
  `4000 - 161 - 32 = 3807` bytes for lines. Fill them and assemble. The retained
  text ends at a line boundary strictly inside the section — step 7 is only reached
  because the section does not fit whole, so the last retained line carries its
  terminator — and therefore ends in `\n`, so no separator precedes the marker. The
  marker does not end in `\n`, so one precedes the notice. The output is
  `3807 + 32 + 1 + 161 = 4001`: one byte over, on a path the criterion says can
  never exceed the budget.

  Reserving with the separators leaves `4000 - 162 - 33 = 3805`, and the same
  assembly yields `3805 + 32 + 1 + 161 = 3999`. Under the bound.

  The marker's reserved byte is not spent here, and on the only path that emits a
  marker it never is. It is reserved because AC-001.6 states both separators
  conditionally, and a reservation that is an upper bound can leave the output
  under budget but can never let it past — which is the property AC-001.2 needs and
  the reason neither reservation is computed from what assembly will actually
  do.

  What makes this survive a careless test is that the over-budget form has no
  slack to absorb it. The reserved notice sets both counts to `{total}`, the
  rendered one carries `{total} - 1`, and those two decimal renderings are the same
  width unless `{total}` is a power of ten — so the reservation is exact, not
  generous, for almost every document. The `{shortened}` clause gives no slack
  either: it is reserved as present and, on this path, is present. On the
  whole-section path the same clause is reserved but never rendered, which is why
  that path is 39 bytes clear and does not expose the defect at all.
- **AC-002.4, on test placement.** The assertion lives in package
  `internal/agent/runtime/dynamic` because `continuationFieldLimit` is unexported
  and only visible there. It imports the reducer package for the exported budget,
  which is cycle-free: `internal/agent/**` does not import `internal/orchestrator`.
  Neither exporting the constant nor modifying `bounded()` is required, so that
  package's production code is untouched.
