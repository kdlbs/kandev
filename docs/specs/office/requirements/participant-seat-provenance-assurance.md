---
status: draft
system: office
created: 2026-09-08
owners:
  - kandev
---

# Office Participant Seat Provenance Assurance Requirements

## Overview

`participant-seat-provenance.md` is shipped and its criteria are met by the
code that satisfies it. Three independent review legs and an outside-voice
pass found no production defect. What they did find is a gap of a different
kind: several of that contract's criteria hold *by inspection* and are not
held in place by anything, and one guard the contract relies on exists at
only one layer.

Both are the same failure mode seen from two sides. A criterion nothing can
falsify is a criterion the next change can break silently, and a guard at
one layer is a guard the next caller can walk around. This document owns
the two repairs.

The first is behavioural and small: the seat store itself refuses a
registration that names no agent. Today that is unreachable, because the
only two production callers are HTTP routes that reject it first
(`AC-OFFICE-SEAT-PROVENANCE-005.1`). It is worth having anyway, because the
store's write is what makes the seat, and a future internal caller — an
agent tool, a scheduler, a fixup path — reaching it directly would write a
seat naming nobody, which no reader can interpret and no fan-out can wake.

The second is a verification contract. It names the specific regressions
the seat-provenance suite must be able to fail on, in terms of the
behaviour that would change, not the code that would change. This is not
a style preference: this system already treats detectability as a
first-class property, because a suite that stays green through a
reintroduced defect is worse than no suite, having spent the cost and
bought the confidence without the coverage.

Office owns both. The registration surface, the fallback casting and the
seat writer are Office's; the contract they implement is
`participant-seat-provenance.md`, also Office's. Nothing here changes that
document, which is frozen. Everything here either adds a guard beneath it
or pins down how it is proven.

## Terminology

- **Seat**, **role slate**, **seat provenance**, **claim**, **automatic
  casting**, **manual registration**, **unclaimed seat**, **decided seat**
  and **supported dialect** are used exactly as
  `participant-seat-provenance.md` defines them.
- **Seat store** — the in-process writer that creates, claims and removes
  seats. Reached today only through the registration surface; the surface
  and the store are different layers and this document distinguishes them
  deliberately.
- **Registration surface** — the operator-facing per-task registration
  entry point, reachable for the `reviewer` and `approver` roles, which
  the seat store sits behind.
- **The suite** — the automated tests that run in CI for this repository,
  taken as a whole. A criterion satisfied by a test that never runs in CI
  is not satisfied.
- **Named regression** — a described change in behaviour, stated as
  behaviour rather than as an edit to code, that would violate a criterion
  of `participant-seat-provenance.md`.

## Prior art

Each leg opens with what it searched.

**Our own prior reasoning — leg unavailable on this runner, and it is
named rather than skipped.** Searched this machine for the `wiki-query`
skill: `~/.claude/skills` holds 57 entries and none is `wiki-query`;
`which wiki-query` reports not found; there is no `~/.obsidian-wiki`
config symlink and no vault directory under `~/Documents`. No
`OBSIDIAN_VAULT_PATH` resolved, so no QMD collection was queried and no
grep fallback ran either. This card was moved onto the `neo` SSH runner,
which does not carry the vault; the leg is unavailable here, not empty.

What partly stands in for it is that this exact concept was already put
through that leg on the primary box, and the result is recorded in the
shipped contract's own *Prior art*: the vault at
`/Users/henry/Documents/henry/wiki`, QMD collection `wiki`, three queries,
nothing on seat provenance or slates. One adjacent page,
`concepts/optimistic-vs-pessimistic-concurrency`, was load-bearing there
and stays load-bearing here: it is why the two writers block on a shared
exclusion rather than repair a duplicate afterwards, which is in turn why
`REQ-OFFICE-SEAT-ASSURANCE-002` insists the mid-write windows be entered
deliberately rather than hoped for.

**What others shipped — leg unavailable.** The `saas-kb` MCP server is not
among this session's tools, so `search_fsm_docs` could not be called with
`category: "ai_sdlc"` or any other filter. The shipped contract records
that leg's earlier result for this concept: two queries, top relevance
0.0139, nothing describing a placeholder-versus-preference relation. No
vendor behaviour is copied here.

**In-repo prior positions — this leg ran.** Searched
`apps/backend/AGENTS.md` and `docs/decisions/INDEX.md` for testing
conventions bearing on determinism and detectability. Three positions
found, all followed rather than re-derived:

- A test-only failpoint injected mid-operation is an established idiom
  here, not a novelty to justify: destructive cutover migrations "inject a
  test-only failpoint after each cutover step" to make a window
  deterministic.
- Detectability is already treated as first class. The PR-sync guidance
  requires a call-count test in as many words, because "an assertion on
  the resulting `PRStatus` passes with the whole regression restored" —
  the same argument this document generalises.
- Revive's 800-effective-line file limit applies to test files, with new
  tests going in a new file rather than being appended to a large one.
  Three of the four files this work touches are already between 438 and
  788 lines, so this constrains where the new cases can live.

**What we are doing differently.** The failpoint idiom above is available
but is not the default chosen here. Where a mid-write window can be
entered from outside the process — by holding a lock the writer must wait
on — that is preferred, because it proves the window on the shipped code
path rather than on a path that only exists when a test is running. The
failpoint stays as the named fallback for a window with no external
handle.

## Requirements

### REQ-OFFICE-SEAT-ASSURANCE-001: The seat store refuses a registration naming no agent

**Intent:** A seat exists to name an agent. A seat naming nobody satisfies
the store's natural key, is admitted silently, is counted by the quorum
guard, and can never be woken or decided — a permanent stall written in
one row. The registration surface already refuses it; the store should
refuse it too, so that the guarantee belongs to the writer rather than to
the caller that happens to be in front of it today.

#### Acceptance criteria

- **AC-OFFICE-SEAT-ASSURANCE-001.1:** When a registration reaches the seat
  store naming an empty agent profile identifier, the store shall reject
  it, writing no seat, claiming no seat and promoting no seat's
  provenance.
- **AC-OFFICE-SEAT-ASSURANCE-001.2:** That rejection shall be reported to
  the caller as a failure, and shall be distinguishable from the
  no-change outcome the store reports for a task that stands at no step or
  does not exist (`AC-OFFICE-SEAT-PROVENANCE-005.2`, `-005.6`). A caller
  shall be able to tell the two apart without matching on a message
  string.
- **AC-OFFICE-SEAT-ASSURANCE-001.3:** The rejection shall occur before the
  store begins a transaction and before it acquires the shared exclusion
  of `AC-OFFICE-SEAT-PROVENANCE-004.2`. A rejected call shall therefore
  contend with no other writer, leave nothing to roll back, and not delay
  a concurrent automatic casting or registration for the same task and
  role.
- **AC-OFFICE-SEAT-ASSURANCE-001.4:** The rejection shall be deterministic
  and carry no state. The same call rejected twice yields the same
  failure, the store shall not retry it, and no caller shall be required
  to.
- **AC-OFFICE-SEAT-ASSURANCE-001.5:** Only the empty identifier is
  rejected by this guard. An identifier that is non-empty but names no
  agent profile — including one that is only whitespace — stays governed
  by `AC-OFFICE-SEAT-PROVENANCE-005.8`: it claims no seat and displaces no
  `auto` seat, and whether it writes one of its own is unchanged. The
  boundary is stated so the two layers cannot drift into disagreeing about
  what "empty" means.
- **AC-OFFICE-SEAT-ASSURANCE-001.6:** No behaviour observable through the
  shipped registration surface shall change. Every request that reaches
  the store through that surface already carries a non-empty identifier,
  so no status code, activity entry, notification, seat or provenance
  differs before and after this guard exists.
- **AC-OFFICE-SEAT-ASSURANCE-001.7:** The guard shall hold on both
  supported dialects, and shall reach the same decision on both without
  consulting the store, since it inspects only its own argument.

### REQ-OFFICE-SEAT-ASSURANCE-002: Named seat-provenance regressions fail the suite

**Intent:** Every criterion below is already satisfied by shipped code.
What is missing is that nothing would notice if it stopped being. Each
criterion names one behaviour that must be asserted and, where the sharp
edge is a specific wrong implementation rather than a missing scenario,
the behaviour change that must turn the suite red.

**User story:** As an engineer changing seat registration, I want the
suite to fail when I reintroduce a defect this contract already fixed, so
that I learn it from CI rather than from a stalled quorum guard in
production.

#### Acceptance criteria

- **AC-OFFICE-SEAT-ASSURANCE-002.1:** The suite shall cover a role slate
  at the task's current step holding a `manual` seat for one agent and
  exactly one undecided `auto` seat for a different agent, with the first
  agent registered again in that role, asserting that the registration
  reports no change and that the second agent's seat still names that
  agent with provenance `auto`. An implementation that searches for a
  claimable seat before checking whether the named agent already holds one
  shall fail this case (`AC-OFFICE-SEAT-PROVENANCE-002.4`).
- **AC-OFFICE-SEAT-ASSURANCE-002.2:** The suite shall cover a claim that
  displaces one agent in favour of another, followed by the displaced
  agent being registered again in the same role at the same step,
  asserting the slate then holds exactly two seats, both `manual`, naming
  those two agents (`AC-OFFICE-SEAT-PROVENANCE-002.10`).
- **AC-OFFICE-SEAT-ASSURANCE-002.3:** The suite shall cover a registration
  whose read-only view of the task names a different current step than
  the view its write goes through, asserting the seat lands at the step
  the write view names. An implementation that resolves the task's step
  outside its own exclusion shall fail this case
  (`AC-OFFICE-SEAT-PROVENANCE-004.10`).
- **AC-OFFICE-SEAT-ASSURANCE-002.4:** The suite shall cover a registration
  request naming an empty agent profile identifier, asserting the surface
  rejects it with a client error and that no seat is written for that task
  and role (`AC-OFFICE-SEAT-PROVENANCE-005.1`).
- **AC-OFFICE-SEAT-ASSURANCE-002.5:** The suite shall cover a participant
  registration naming a role other than `reviewer` or `approver`,
  asserting it is refused and that no seat is written or claimed
  (`AC-OFFICE-SEAT-PROVENANCE-005.7`).
- **AC-OFFICE-SEAT-ASSURANCE-002.6:** The end-to-end quorum case that
  walks an automatic cast followed by a manual registration naming a
  different agent shall assert the resulting guard requires exactly one
  decision, not only that the guard names the expected role. A regression
  to two seats in that slate shall fail it
  (`AC-OFFICE-SEAT-PROVENANCE-006.1`, `-006.2`).
- **AC-OFFICE-SEAT-ASSURANCE-002.7:** The suite shall cover a claim whose
  cancellation of the displaced agent's queued run fails, asserting that
  the registration still reports success, that the claimed slate is
  exactly as a successful claim leaves it, that the failure is recorded,
  and that the displaced agent's run is still runnable afterwards
  (`AC-OFFICE-SEAT-PROVENANCE-006.6`, second sentence).
- **AC-OFFICE-SEAT-ASSURANCE-002.8:** The claim's activity entry shall be
  asserted by content, not by count: the recorded detail shall be checked
  to name the task, the step, the role, the displaced agent profile and
  the claiming agent profile, each with its correct value. An entry
  written with an empty or wrong payload shall fail the case
  (`AC-OFFICE-SEAT-PROVENANCE-002.9`).
- **AC-OFFICE-SEAT-ASSURANCE-002.9:** A claim shall be asserted to leave
  the claimed seat's identifier, decision-required flag, position and
  creation time unchanged, in addition to the agent profile and
  provenance it changes (`AC-OFFICE-SEAT-PROVENANCE-002.2`).
- **AC-OFFICE-SEAT-ASSURANCE-002.10:** The case covering a seat removed
  after a registration selected it and before the claim was applied shall
  enter that window deliberately rather than by scheduling chance. It
  shall establish that the seat was selected and then removed, and assert
  the registration writes a new `manual` seat for its named agent. An
  implementation that reports a claim when its conditional write matched
  no seat shall fail it
  (`AC-OFFICE-SEAT-PROVENANCE-004.8`).
- **AC-OFFICE-SEAT-ASSURANCE-002.11:** A case that can only be entered on
  one supported dialect shall skip explicitly when that dialect is
  unavailable, and shall never report success without having run. A
  criterion whose only case skipped is not covered, and the suite shall
  make that visible rather than green.
- **AC-OFFICE-SEAT-ASSURANCE-002.12:** No case added under this
  requirement shall change production behaviour, weaken an existing
  assertion, or relax a criterion of `participant-seat-provenance.md`. A
  case that cannot be made to pass against the shipped implementation is
  evidence about this document, which is then wrong and routes back here,
  not licence to edit the code it tests.

## Out of scope

- **Rejecting an empty participant role at the seat store.** The store's
  own `role` column admits only the five defined roles, so an empty role
  is already refused structurally and cannot produce the silent malformed
  seat an empty agent identifier can. Which of those five the
  registration surface exposes is that surface's policy
  (`AC-OFFICE-SEAT-PROVENANCE-005.7`); moving it down a layer would
  duplicate the decision in two places that can then disagree.
- **Guarding participant *removal*.** This guard is on the registration
  write only. Removal naming an empty agent profile identifier deletes by
  exact match on that identifier, so it can only ever match a seat that is
  already malformed and never a well-formed one; it is not a destructive
  wildcard and needs no guard. Removal's scope stays exactly as
  `AC-OFFICE-SEAT-PROVENANCE-003.4` defines it.
- **Rejecting an empty task identifier at the seat store.** A
  registration naming no task resolves no current step, and
  `AC-OFFICE-SEAT-PROVENANCE-005.6` already contracts that as writing
  nothing and raising no failure. Turning it into a rejection would change
  contracted, shipped silence, which is a different decision from adding a
  guard beneath an existing one.
- **Mapping `REQ-OFFICE-SEAT-ASSURANCE-001`'s failure to an HTTP status.**
  No shipped route can reach it, so a status mapping would be a contract
  written for a caller that does not exist. The first caller that needs
  one brings its own requirement.
- **Changing any criterion of `participant-seat-provenance.md`.** It is
  frozen. This document adds a guard beneath it and pins how it is proven;
  where the two ever conflict, that document wins and this one is wrong.
- **Introducing a mutation-testing tool or CI gate.**
  `REQ-OFFICE-SEAT-ASSURANCE-002` names specific regressions in prose and
  asks for cases that catch them. It does not ask for a harness that
  generates mutants, nor for a coverage threshold.
- **Coverage for criteria not named above.** The criteria listed are the
  ones a review pass found unheld. Silence here about another criterion
  means it was not examined, not that it is proven.
- **Waking the claiming agent, surfacing provenance in the API, and
  collapsing multiple `manual` seats.** All three are excluded by
  `participant-seat-provenance.md` and stay excluded; nothing here reopens
  them.

## Dependencies

- `participant-seat-provenance.md` owns every criterion referenced above.
  This document adds no seating behaviour of its own beyond
  `REQ-OFFICE-SEAT-ASSURANCE-001`, and changes none of that document's.
- `review-participant-seats.md` owns automatic casting, which is one of
  the two writers every concurrency case here exercises. No criterion in
  it changes.
- `step-entry-sequence-execution.md` owns whether a step's entry actions
  run at all, and therefore whether an `auto` seat exists to be claimed in
  the end-to-end case.
