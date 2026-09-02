---
status: draft
system: agents
created: 2026-09-01
owners:
  - nova28
---

# Plan injection budget Requirements Part 1

## Overview

Kandev pastes a task's implementation plan into the prompt of a launching agent session
at two independent sites. Neither bounds the plan against a stated budget, and both
reduce it — when they reduce it at all — in a way that discards the newest part of the
running record without telling the agent anything was lost. This capability introduces
one shared, deterministic plan reducer, applies it at both sites, and requires that any
reduction be declared in the injected text.

The agent system owns this contract: both consumers are agent-runtime session-launch
surfaces, and [dynamic agent routing](../system-design/dynamic-agent-routing-01.md)
already requires "a bounded continuation package" without defining the bound. The task
and workflow system keeps the plan artifact, its write API, and its revision model;
this capability only reads.

Rationale, worked examples, and the algorithm's derivation live in the
[system design](../system-design/plan-injection-budget-01.md); its measurement,
prior-art receipts, and per-criterion notes — Appendices A, B and C, named throughout
this file — are in
[system design Part 2](../system-design/plan-injection-budget-02.md). This file states
the contract. Criteria are cross-referenced as `AC-001.n` / `AC-002.n`.

This part carries the reducer's own contract (REQ-001), the terminology every criterion
in both parts judges against, and the measurement. The anti-drift requirement (REQ-002)
and the exclusions common to both parts (`## Out of scope`) are in
[Part 2](plan-injection-budget-02.md).

## Terminology

- **Plan document / composed document:** the exact text a call site would inject
  today for a given plan, composed PER SITE and never normalised across the two: the
  handover site takes `plan.Content` VERBATIM and does NOT trim it; the dynamic site
  takes `TrimSpace(plan.Title + "\n" + plan.Content)`, as it does today. The reducer is
  defined over this text — not over the database row, and not over the frame a site
  wraps around the reducer's output (AC-002.5).
- **Section:** a run of the plan document beginning at a line starting with `## ` and
  continuing to the line before the next such line, or to the end. Content before the
  first such line is the **preamble section**, and ONLY when there is such content: a
  document whose first line already begins with `## ` has NO preamble section and SHALL
  NOT be given a zero-length one, which would inflate `{total}` by one. Every other
  section begins at a `## ` line, so the preamble is the only section that could ever be
  empty. A document with no `## ` line is one.
- **Budget:** the maximum size in bytes of the reducer's output, including any
  omission notice, cut marker, and separator bytes.
- **Line:** a run of a section up to AND INCLUDING the next `\n`, or — for a final line
  carrying no terminator — up to the section's end. Both are COMPLETE lines, and a
  line's length counts its terminator when it has one. Cutting at a **line boundary**
  therefore leaves text ending with `\n` or at the section's end; the latter is normal,
  not a corner, the dynamic site's composed document being trimmed.
- **Omission notice:** the fixed template appended when, and only when, the reducer
  dropped content, carrying two independent facts:

  ```text
  [Kandev: plan reduced to fit the injection budget; {omitted} of {total} sections omitted{shortened}. Call get_task_plan_kandev for the full plan.]
  ```

  `{omitted}` and `{total}` are decimal integers. `{shortened}` is the literal
  `, and the retained section was shortened` when the intra-section path of AC-001.7
  ran, and empty otherwise.
- **Inline cut marker:** the fixed literal `[Kandev: section truncated here]`, emitted
  only on the intra-section path.
- **Notice reservation:** `len(notice)` rendered at its widest for the document in hand
  — `{omitted}` and `{total}` both set to its total section count, `{shortened}` present
  — PLUS ONE BYTE for the separator AC-001.6 may place before it. An upper bound on both
  counts, computed once after splitting and before any section is retained.
- **Marker reservation:** `len(marker)` PLUS ONE BYTE for the separator AC-001.6 may
  place before it. Also an upper bound: each separator is conditional at assembly time,
  so reserving it unconditionally can leave the output under budget but never over it.
  Reserving the notice and marker WITHOUT their separators does not satisfy AC-001.2 —
  worked in the design's Appendix C.
- **Containment:** the transformation of AC-001.14. Not part of the reducer.

## Measurement

Live Kandev SQLite database, read-only, 2026-09-01: **178 plans** — the corpus the
criteria cite. The full table, and what each row decides, is in the design's
Appendix A.

## Requirements

### REQ-AGENTS-PLAN-INJECTION-BUDGET-001: Bounded, deterministic plan injection

**Intent:** Bound what a launching session pays for plan context, keep the parts that
describe both the plan's framing and its current state, and never let the agent believe
a reduced plan is the whole plan.

**User story:** As an agent starting a new session on a task that already has a plan, I
want a bounded excerpt that tells me when it is an excerpt, so that I neither exhaust my
context on a document I can fetch nor act on a silently truncated record as complete.

#### Acceptance criteria

- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.1:** WHEN a plan document is at most the
  budget in bytes, THEN the reducer SHALL return it byte-identical and SHALL NOT append
  an omission notice. Equality with the budget is within budget. Byte-identity is
  relative to the document the reducer RECEIVES, which at the handover site is the
  composed document after AC-001.14 containment. AC-002.3 is the complete register of
  the respects in which injected bytes may differ from today.
  PRECEDENCE: this is the GENERAL case and it yields to the two special ones, which are
  evaluated first. AC-001.12 governs an empty or whitespace-only composed document — it
  injects nothing, even though it is trivially within budget, so byte-identity does NOT
  apply to it — and AC-001.13 governs a budget too small to emit anything at all. Where
  either applies it wins; everywhere else this criterion holds.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.2:** WHEN a plan document exceeds the budget,
  THEN the reducer's output SHALL be at most the budget in bytes, counting the omission
  notice and any inline cut marker. The bound SHALL hold for every input, the criterion
  being the measured size of the final output. On the whole-section path it SHALL be
  achieved by subtracting the notice reservation ([Terminology](#terminology)), which
  THERE carries every generated byte assembly can add, separators included. On the
  intra-section path of AC-001.7 the marker reservation SHALL be subtracted IN ADDITION:
  that path also emits the cut marker, which the notice reservation does not carry, so
  subtracting the notice reservation alone would exceed the budget by the marker's
  length. Both reservations being upper bounds, no re-selection, eviction, or restart is
  required or permitted.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.3:** WHEN the reducer drops any content, THEN
  the output SHALL end with the omission notice of [Terminology](#terminology). Its two
  facts are reported independently. `{omitted}` SHALL be the number of whole sections
  not represented in the output AT ALL and `{total}` the document's section count, a
  section present but shortened by AC-001.7 NOT counting as omitted. Zero is therefore
  correct ONLY when the document has one section: when AC-001.7's path runs on a
  document of `{total}` sections it retains a shortened first section and drops the
  rest, so `{omitted}` SHALL be `{total} - 1`. The notice is tied to content having been
  dropped, never to a non-zero section count. This criterion yields to AC-001.13: when
  the reducer can emit no plan content at all it SHALL return nothing, the notice
  included, even though content was dropped. That is the ONLY case in which dropped
  content is not followed by a notice.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.4:** WHEN the reducer drops no content, THEN
  its output SHALL NOT contain an omission notice.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.5:** WHEN the reducer drops content, THEN it
  SHALL retain a contiguous run of whole sections from the start of the document and a
  contiguous run from the end, and SHALL drop only whole sections from the middle,
  except as permitted by AC-001.7. Either run MAY be empty; retaining only a tail run is
  expected when the first section does not fit.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.6:** WHEN the reducer selects which sections
  to retain, THEN it SHALL grow two runs — one from the end of the document and one
  from the start — considering candidates alternately, tail run first. A candidate
  SHALL be retained when it fits the budget remaining after subtracting the notice
  reservation and the sections already retained. WHEN a candidate does not fit, THEN
  that run SHALL be closed and no further section taken from it, keeping each run
  contiguous per AC-001.5. Selection SHALL stop when both runs are closed or
  the runs meet, and each section SHALL be considered at most once.
  THE TRAVERSAL SHALL BE FULLY DETERMINED, so that two implementations cannot retain
  different sections for the same input. Number the sections in document order; the head
  run's next candidate is the lowest not yet considered, the tail run's the highest. THE
  RUNS MEET when no unconsidered section remains between them, and only then. Turns
  alternate, tail run first, a closed run forfeiting its turns to the other. WHILE A RUN
  IS OPEN AND AN UNCONSIDERED SECTION REMAINS, that run SHALL consider its next candidate
  on its turn: consideration is REQUIRED, not merely permitted, so no section may be
  skipped by declaring the runs met while it is still unconsidered. A considered section
  is consumed whether or not it was retained — that is what makes "at most once" exact —
  and SHALL NOT be revisited by the other run; it could not be retained if it were, the
  remaining budget only ever shrinking. Retained sections
  SHALL be emitted in original document order; the reducer SHALL NOT reorder, merge,
  deduplicate, or rewrite them.
  ASSEMBLY SHALL BE EXACT: retained sections SHALL be concatenated with NO inserted
  separator — every section but the document's LAST ends at a `## ` line and so already
  carries its trailing newline, and the last one, which may not, can only land at the
  end of the output where the next rule covers it. The notice — and, on the AC-001.7
  path, the marker before it — SHALL each be preceded by a single `\n` only when the
  text so far does not already end with `\n`; those two conditional bytes are what
  [Terminology](#terminology)'s reservations hold room for. The output SHALL NOT end
  with a trailing newline.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.7:** WHEN the selection of AC-001.6 retains
  no whole section, THEN and only then the reducer SHALL reduce within the document's
  **first** section: it SHALL keep that section's leading whole lines while they fit the
  budget remaining after reserving BOTH the notice reservation AND the marker
  reservation ([Terminology](#terminology)), SHALL cut only at a line boundary, and
  SHALL mark the cut with that marker. Each candidate line SHALL be measured as
  Terminology defines a line, its terminator included. It
  SHALL NOT cut mid-line, and SHALL NOT reduce intra-section in any other case. WHEN
  not even one complete line fits after those reservations, THEN the reducer SHALL
  return no plan text at all, per AC-001.13. The trigger is that nothing whole fits, NOT that
  the first section is large: a document with an oversized first section but a small
  last one retains that last section under AC-001.6 and never reaches this path
  (design `#why-the-fallback-reduces-the-first-section`).
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.8:** The reducer's output SHALL be valid
  UTF-8 for every input that is valid UTF-8. A cut SHALL NOT split a rune.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.9:** WHEN the reducer is called twice with
  the same plan document and budget, THEN it SHALL return byte-identical output,
  depending only on those two inputs — not on the workflow step, session, task,
  agent, clock, map iteration order, or any heading's text.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.10:** WHEN the reducer is applied to its own
  output under the same budget, THEN the result SHALL be byte-identical to that
  output.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.11:** WHEN the plan document contains no
  `## ` line, THEN it SHALL be treated as one section; WHEN such a document also exceeds
  the budget it is governed by AC-001.7, one that fits returning unchanged under
  AC-001.1. WHEN
  it contains content before its first `## ` line, THEN that preamble SHALL be the
  first section, eligible for retention on the same terms as any other.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.12:** WHEN the plan is absent, or the site's
  COMPOSED DOCUMENT is empty or only whitespace, THEN no plan text and no omission
  notice SHALL be injected, and the launch SHALL proceed unchanged. Emptiness SHALL
  be judged on the composed document — after containment, where it applies — and never
  on the raw `content` column. One input splits the two sites: at the DYNAMIC site a
  plan with a title and whitespace-only content composes to a NON-empty document and
  SHALL still inject the bare title; at the HANDOVER site the composed document IS the
  content, so that plan is empty here and no plan section SHALL be injected, changing
  today's behaviour. That second case is a named exception in AC-002.3; both are worked
  in the design (Failure and recovery, Appendix C).
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.13:** WHEN the reducer cannot emit any plan
  content within the budget, THEN it SHALL return no plan text at all — never a
  notice with nothing attached, never a marker with nothing attached, never an output
  over budget. This SHALL cover every such case, so the function is total: a
  non-positive budget; a budget smaller than the notice reservation; a budget holding
  the notice reservation but not it and the marker reservation together on the
  intra-section path; and a budget holding both but not one complete line of the first
  section. Every band is judged against [Terminology](#terminology)'s reservations,
  separators included.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.14:** The injected plan text SHALL NOT be able
  to terminate or forge the enclosing `<kandev-system>` block. Plan content is
  agent-authored and untrusted here; one stored plan already carries a start tag, and
  `StripTags` removes only the end tag (design `#security`).
  MECHANISM: containment SHALL remove every occurrence of the exact literals
  `<kandev-system>` and `</kandev-system>`. IT SHALL BE ONE COMBINED LOOP: every pass
  SHALL remove occurrences of BOTH literals, and the loop SHALL terminate only when a
  full scan finds NEITHER. A single pass is not enough — one pass over
  `<kandev<kandev-system>-system>` reconstitutes a live tag — and TWO INDEPENDENT
  PER-LITERAL LOOPS SHALL NOT BE USED, because removing one literal can construct the
  other while a loop that rescans only for its own literal never sees it. NEITHER
  ORDERING ESCAPES, so running the two loops the other way round is not a fix: given
  `<kandev` + `</kandev-system>` + `-system>` a start-then-end pair leaves a live
  `<kandev-system>`, and given the mirror input `</kandev` + `<kandev-system>` +
  `-system>` an end-then-start pair leaves a live `</kandev-system>`. In each case the
  first loop finds nothing to do and the second constructs the literal the first was
  looking for, after it has stopped looking. Matching SHALL be case-sensitive and
  exact-literal; no other text SHALL change.
  SCOPE: containment SHALL apply at the HANDOVER SITE ONLY, before the reducer is
  called. The dynamic site does not wrap plan text in that block, so containing there
  would change injected bytes for no security gain.
  ORDER: containment precedes reduction, so the reducer's guarantees hold over the
  contained document and emptiness (AC-001.12) is judged after it — a plan of only tag
  literals contains to empty and injects nothing.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.15:** The reducer SHALL hold no mutable shared
  state, and concurrent calls for different tasks or the same task SHALL NOT affect each
  other's output. Each call SHALL operate on the snapshot its caller already read; the
  reducer SHALL NOT read the database.
- **AC-AGENTS-PLAN-INJECTION-BUDGET-001.16:** WHEN a plan is mutated between two
  session launches, THEN each launch SHALL reflect the snapshot its own caller read.
  The reducer SHALL NOT cache output across calls.
