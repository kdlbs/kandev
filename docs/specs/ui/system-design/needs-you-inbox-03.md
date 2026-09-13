---
status: draft
system: ui
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
---
# Needs-you Inbox System Design Part 3

Part 1 (boundaries, input inventory, flag, components, data and contracts) is in
[`needs-you-inbox-01.md`](needs-you-inbox-01.md); Part 2 (control flow, failure
and recovery, security) is in [`needs-you-inbox-02.md`](needs-you-inbox-02.md).
This part carries the visual and copy contract.

## Design source

The visual and copy contract is the KAN-41 revision-4 mockup deck, drawn
against the shipped shell and vendored at
[`../assets/needs-you-inbox-mockup-r4.html`](../assets/needs-you-inbox-mockup-r4.html).

It is vendored rather than linked because the first implementation round
received it as prose only. Its eleven frames were inlined into the originating
card as host-relative `/api/attachments/<id>/content` URLs. Resolved against
the Kandev origin, which is the only origin an agent reading that card knows,
they 404: Kandev has its own `/api/attachments/` route over a different
identifier space, so the path looks valid and fails without erroring. The build
that followed satisfied every acceptance criterion in the requirements while
diverging from the deck, because no criterion in the requirements described the
deck.

That is the failure this part exists to prevent, and it generalizes: a design
that is not in the spec canon cannot fail review, because review checks code
against the canon. A copy or control decision that matters is stated here, not
only drawn.

## Fidelity decisions

**D1. Each row offers a control that opens its task.** The row body toggles the
answer panel, so without a distinct control the task a question belongs to is
unreachable from the Inbox. The deck draws it as a `Button size="sm"` on the
right of the row, beside the actions menu; that button is withheld below the
`sm` breakpoint, where its fixed width would squeeze the question title and put
the row at risk of the horizontal overflow AC .30's mobile specification
forbids. The actions menu carries the same destination at every width, so the
task is reachable on a phone without the button.

**D2. The empty state names the active workspace and does not congratulate.**
No completion icon, no exclamation mark, and no claim of being caught up. An
empty queue is a normal state rather than an achievement, and an unscoped
congratulation reads as a claim about the whole instance while other workspaces
hold unanswered questions. Naming the workspace also keeps the sentence true
under AC .20, which already requires the empty state to name what it is not
counting. When the active workspace cannot be resolved from client state, the
sentence falls back to naming no workspace rather than naming the wrong one.

**D3. A row whose bundle holds more than one question states how many.** A
bundle is answered as a unit, so its size changes what answering it costs, and
a row that hides the size understates the work. The count is of questions
carried, not of messages: a bundle's message list can hold context messages
that are not themselves questions, so the count filters on the presence of a
clarification question. A single-question bundle renders no count line, because
the line would then restate the row above it.

## Deck decisions this capability does not adopt

Each is a recorded decision rather than an omission; all are listed in the
requirements under "Out of scope".

- **The tab strip**, and with it the deck's `variant="line"` analysis. v1
  renders one bucket, so there is nothing to switch between.
- **The history bucket and the failed-task bucket**, which the deck draws as
  the second and third tabs.
- **Permission rows** and their amber `IconShieldQuestion` treatment.
- **Re-ask**, cut from v1 on 2026-09-11.
- **The deck's cross-workspace empty state**, which offers to switch to a
  workspace that has a waiting question. It depends on a per-workspace count
  map the client does not hold for workspaces it is not in.

Because the tab strip is gone, the deck's empty-state copy pointing the
operator at "the other two tabs" has nowhere to point; D2 states the copy for
the one-bucket world instead.
