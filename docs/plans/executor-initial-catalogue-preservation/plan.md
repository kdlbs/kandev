---
created: 2026-10-10
status: implemented
requirements:
  - REQ-EXECUTORS-INITIAL-CATALOGUE-001
system_design:
  - ../../specs/executors/system-design/initial-catalogue-loading.md
legacy_specs: []
---

# Implementation plan: Preserve executor choices during initial loading

## Overview

Prepare one independently verifiable correction to executor initial-read
publication, with real selected-profile regression evidence. The reviewed design checkpoint was
`DESIGN_READY`; the single work order is now implemented and locally validated.
The owning [requirement](../../specs/executors/requirements/initial-catalogue-loading.md)
and [design](../../specs/executors/system-design/initial-catalogue-loading.md)
define the outcome. Initial loading has a separate lifetime from the existing
[profile-editor contract](../../specs/executors/system-design/profile-editor.md);
its already-corrected profile-card refresh is a pattern, not implementation work.

## Evidence and assumptions

Confirmed by ROOT at `df73598e7c1a06ad7a5fcf11cbf0f0b1d1b76fef`: a held real
successful initial GET overwrites registered executor.created and profile.created
updates. Real task fallback/options/selector evidence shows a selected live
profile becoming unavailable and its trigger reverting to the placeholder.
ONE causal RED and TWO passing controls (ordinary initial GET and warm catalogue)
had zero unhandled/setup errors. Readable metadata:
`/tmp/kandev-root-executor-bootstrap-discovery-20261010/qualified-proof.json`,
`qualifier.json` and `qualifier.log`. Original native 99871 actually joined as
1177a0, exit 1; wrapper 3498895/group 3498896 absent; GLOBAL heavy RETURN
06:57:55 UTC. This is supplied discovery, not our permanent-test RED or GREEN.

Protected `candidate.test.tsx` (0400), SHA256
`678087ed978585af9b8911b9c31d039abb55930b4116d1754c1c5b598fb93593`,
must never be read, copied, deleted or reused. Independently author future tests.

Current inspected local HEAD and exact remote `refs/heads/main` both equal
`53a00c274a677e579695c8d90052755372787c95`; candidate branch is
`feature/preserve-live-execut-so8`, currently unpublished. Current source
retains the qualified unconditional publication. Task/session boot and live
task/subtask callers confirm the actor impact. Before implementation re-resolve
exact main/candidate remote refs and inspect relevant source for intervening
changes; do not trust shared remote-tracking refs or rewrite foreign work.

Assumption check: intent, owner and bounded scope are settled by ROOT. No
material unanswered question blocks design. Chosen routine design is a local
sticky fence for reads admitted against empty state; it preserves complete
current state rather than imposing server freshness. Catch preservation is
necessary in this same read and needs independent affected-path evidence.

## Scope

In scope: AC .1 through .6, only executor initial-read admission/publication,
loaded compatibility, invocation cleanup, real task-profile selection evidence.
No global resource framework, versions/timestamps, backend, event, agent-list,
auth/lifecycle or other-writer redesign. No refresh remigration. No new UI
interaction/copy or broad hardening. No product changes in the design turn.

## Technical approach

Use the captured owning AppStore in `useSettingsData` for fresh admission.
Observe the empty-to-nonempty transition before the existing no-store GET;
retain a sticky observation until that read settles. Uncontested responses
populate; contested reads and failures retain current state. Existing loaded
finalization/fast path, gates, agent discovery and consumers retain behavior.
Use a small private adjacent helper only if extraction is needed for local
function limits; it is not a reusable resource abstraction.

| Boundary | Intended behavior | Evidence |
| --- | --- | --- |
| Empty, enabled, unsettled store / real GET | Populate if uncontested, including empty result | Ordinary/empty controls |
| Registered owner/profile events during held GET | Retain whole current inventory and selected usable choice | Independent causal RED/GREEN and mixed-state case |
| Populated or loaded / disabled gate | Preserve existing skip and completion behavior | Warm/loaded/disabled controls |
| Failure / effect rerender | Retain live state; dispose only at settlement | Independent failure and cleanup cases |
| Existing provider eligibility | Current disabled reasons remain effective | Actual options and selector, no eligibility override |

No new unsupported-shape or provider fallback is introduced; supported profiles
continue through existing options/eligibility. Selection defaults and task
submission remain outside publication ownership.

## Tests and acceptance coverage

One new real integration suite, with helper only if necessary, covers:

| Named case | AC |
| --- | --- |
| keeps a live selected profile after a stale successful initial GET | .2, .3 |
| populates usable choices from an ordinary initial GET | .1 |
| skips GET and preserves usable prepopulated choices | .4 |
| preserves current mixed owner/profile values, membership and order | .2, .3 |
| remembers live arrival followed by removal back to empty | .2 |
| preserves live choices after initial GET rejection | .5 |
| settles uncontested empty success and failure | .1, .5 |
| gates disabled, re-enabled and already-loaded empty consumers | .4 |
| ignores unrelated settings and empty-array replacement | .6 |
| keeps the observation through a live effect rerender and disposes at settlement | .2, .5, .6 |

Test names may be parameterized without weakening the evidence. Inventory
assertions quantify every current entry, not merely one surviving row.
Cleanup must settle/reject all held fetches, unmount providers and restore
stubs/subscriptions with zero unhandled errors. Catch companion evidence must
prove the loss before changing catch; do not claim discovery qualified it.

## UI and E2E boundary

Data-only mobile exception: actual options/selector and selected draft prove
the common state outcome. Existing desktop/phone composition, touch, scrolling,
navigation, markup and copy are unchanged. No ASCII redesign, mobile/browser
suite or product build is needed. This is component integration across real
hook/provider/HTTP/events/options/selector, not a live backend or execution claim.

## Work orders

- [x] [Task 01: Preserve initial catalogue publication](task-01-preserve-initial-publication.md)

One work order, sequential, no delegation. No implementation dependency on the
sibling hosted PR 4395 or protected paused task a6032d95.

## Operational and delivery gates

Task 24418c94-207d-4a48-8c73-49a4789ba702; session
e7305d5a-ec97-4292-a673-d8c157b5741d. Keep this primary/model. No native
delegates, recursive tasks, new sessions or model switches. Preserve foreign
edits, worktrees, dependencies, caches, refs, processes, paused task a6032d95
and anonymous volume 2c48e791. Never resume/archive/clean them.

DESIGN ONLY until ROOT reviews actual four files and sends a LATER explicit
implementation INTERRUPT; heavy commands additionally require GLOBAL exclusive
heavy admission. No install, permanent test, lint, typecheck, build, hooks,
commit or push in this turn. Final design action is the DESIGN_READY handoff.
No user approval question or task-completion signal that advances implementation.

Later reviewed delivery is conditionally authorized: frozen install once only
if dependencies are missing, targeted checks, normal hooks without bypass,
ordinary commit/push and ready PR. Stage new TS helpers before the final i18n
ratchet. Preserve full native exec results, every returned session_id and actual
original joins. A directly completed command needs its actual exit only.
Record current owned absence and RETURN before releasing heavy work; never
kill foreign processes or infer joins from a duplicate invocation.

Keep one original 90-minute hosted observer, 60-second cadence, GNU 91-minute
timeout and kill-after 10 seconds, across interrupts/main advances/head changes.
Read automatic feedback while CI runs. Do not replay full suites per push/SHA;
rerun only affected checks for substantive changes. Optional style/docstring
polish neither pushes nor gates. Focused semantic review only for concrete
material correctness, security or architecture needs.

Final later acceptance requires actual current same-turn hosted publication
and checks: known six plus actual Backend/Frontend/E2E successes, fresh complete
errors=[], zero actionable visible/hidden/human findings, clean adequately
reviewed head, original observer join and current owned absence. ROOT stays
active/directly supervises; callback-queue-full is optional/non-gating. Requested
ROOT messages use INTERRUPT only where platform authorization permits it;
children cannot interrupt parents. No END/WFI/extra publication ceremony.
No merge before ROOT's one static check and SEPARATE serial normal expected-head
squash grant. Exact refs/heads/main and candidate ref are authoritative.

## Verification results

`DESIGN_READY`. Catalogue validation (369 decisions, 1513 specifications),
specification lint, Markdown links/anchors and whitespace checks passed.
Unfiltered tracked+cached+untracked NUL inventory contains exactly the four
new design files, unstaged/uncommitted, with no unexpected/foreign paths.
Independent ONE-order/ALL-six-AC/design/manifest references passed. Actual
docs-only coverage is exempt, errors=[]; separate planned-trigger validation
passes with ONE order and errors=[], not actual production coverage.
See the [work-order receipt](task-01-preserve-initial-publication.md#results).
ROOT completed full actual-file review and later released implementation under
exclusive lease 110. Implementation and local task checks now pass: 13 new
integration cases and 6 existing hook controls, scoped ESLint, typecheck, full
i18n and the final staged ratchet. Actual production coverage includes both
source modules, status=covered, ONE order, ALL six ACs, errors=[]. Normal hooks,
publication and current hosted evidence remain separate delivery gates.

Public documentation audit: `docs/public/executors.md` (Create and select a
profile), root README and `docs/screenshots.md` remain accurate. This local
state correction changes no instruction, API, label, screenshot, configuration
or interaction. Internal specs/plans are the only documentation changes. The
existing executor system boundary is unchanged; no README catalogue row or ADR
is needed.

## Risks

- A conservative skipped response may defer response-only entries. No retry or
  global freshness promise is added.
- Disposal on effect cleanup would lose the fence after live arrival; keep
  cleanup invocation-owned and prove rerender plus settlement.
- Adding shared deduplication/auth/lifetime policy would exceed the qualified
  defect. Reassess with ROOT if another writer or boundary proves necessary.
- The selected ID alone can hide the bug. Require actual enabled option,
  owner fallback metadata and rendered selected trigger after held settlement.
