---
created: 2026-10-06
status: implemented
requirements:
  - REQ-UI-SESSION-SEARCH-OWNERSHIP-001
system_design:
  - ../../specs/ui/system-design/session-search-ownership.md
legacy_specs: []
---

# Implementation Plan: Current Chat Search Results

## Overview

One sequential work order corrects local search request ownership and proves it
through the rendered production hook and real protocol client. This four-file
package was reviewed by ROOT before a later explicit implementation INTERRUPT in
the same primary session and an exclusive global local-heavy grant. The one local
work order is complete; publication, hosted verification and merge remain separate
delivery gates. The original design checkpoint remains recorded in the task plan.

## Scope

Own the focused UI [requirement](../../specs/ui/requirements/session-search-ownership.md)
and [design](../../specs/ui/system-design/session-search-ownership.md), hook-local
request admission/settlement and meaningful transport-only deferred regressions.
Preserve the accepted debounce, wire payload, errors, result shape, navigation,
bounded raw backfill, independent instances and usable current searches.

Exclude backend writes, index/schema, WebSocket protocol/transport/abort, global
coordinators/stores, transcript loaded-data identity, pagination policy, composer
Ctrl+R history search, layout/copy/touch/navigation/breakpoint changes and extra
documentation artifacts without an authoritative-owner scope checkpoint.

## Technical approach

Implement only the local ownership boundary in `use-session-search.ts`, with
generation retirement at query edit/close, committed session lifecycle and unmount;
guard timer admission and all success/catch/finally state effects. Audit retained
callbacks and React commit/effect replay without a generic async abstraction.
The design inventories actual TaskChatPanel/overlay/search-bar wiring and real API
and WebSocket request resolution. No consumer production edit is planned.

## Evidence and assumptions

Initial clean checkout HEAD: `b84e4add2b7a76ef709ba8797d62665d06916e8a`;
branch `feature/keep-chat-search-res-sj6`; hook blob
`44383efe8aa775d3e0e3e3af9a5d55a2ac956fa2` independently confirmed here.
The request-ID advances only for a started nonempty debounced request. Query
clear/replacement/close can therefore leave an old settlement eligible.

Historical ROOT execution on `5010081464673fb41c163c06a64926308cb15d43`,
handle 63114, actually joined exit1 after 5.078s: three causal retired-hit failures
and two current/exact-wire/newer-request controls passed. Receipt/classification
are `/tmp/kandev-root-session-search-proof-{receipt,classification}.json`; original
read-only source `/tmp/kandev-session-search-retirement-repro.test.tsx` SHA256
`75026c553058036f8d3ec9942e52109bb584ccc50ef25ceecd7da33bc7aaadf7`
was independently checked here. ROOT's audit
`/tmp/kandev-root-ready-candidates-main-audit-20261006T0457Z.json` reports unchanged
current-main hook, immediate consumer/API/transport sources. These are historical
ROOT receipts, not child test execution. Never rerun or delete the original proof.

Confirmed assumption check: this is a current-search ownership correction with
no backend defect claim. UI owns independent client reply ownership. Catalog and
adjacent transcript-motion/pagination/composer-focus searches found no focused
current-session-search pair; use this coherent pair rather than an incident spec.
No unresolved material product choice or separate ADR is needed.

## Tests

| Acceptance | Targeted evidence after release |
| --- | --- |
| `.1` | `use-session-search-ownership.test.tsx`: clear/whitespace, query replacement before debounce, retired success/error/finalizer, current pending loading |
| `.2` | Same suite: close/reopen success/error, cancelled queued work and usable fresh searches |
| `.3` | Same suite: committed session/null/A-B-A, old retained callbacks, unmount, StrictMode and independent instances |
| `.4` | Same suite: exact payload/180ms/current result/error/newer order; existing `use-session-search.test.ts` debounce/null/close/navigation controls and focused MAX40/cancellation controls |

Full IDs use prefix `AC-UI-SESSION-SEARCH-OWNERSHIP-001`.
Tests use the real rendered hook/API/protocol with only the wire transport deferred.
No mocked ownership predicate, exported private helper, or fake full chat panel.

## Mobile and end-to-end boundary

Apply the explicit `/mobile-parity` exception for pure state/data normalization
inside the existing component, with no layout/touch/scroll/navigation/viewport
change. Shared desktop/phone semantics are proven at the rendered production-hook
and real API/protocol boundary. No browser/E2E/build or ASCII layout preview is
scheduled. An observed composition or glue defect requires a scope checkpoint.

## Work orders

- [x] [Task 01: Retire obsolete chat searches](task-01-retire-obsolete-searches.md)

## Resource and delivery barriers

Do not install or run product checks during design. After ROOT releases the
sole global-heavy lease, run the work order's exact bounded commands sequentially
in explicit Bash with login=false and existing Node24. Log every command, start,
deadline, owned PID/process group, returned handle, terminal exit and cleanup;
actually join each before the next. Preserve foreign worktrees/deps/proofs/refs/
caches/processes and paused work. Routine causal fixture/lint fixes rerun only
affected checks. Timeout/resource/transport/unknown/out-of-scope conditions return
a ROOT checkpoint without automatic recovery or larger budgets.

After checks, normal active hooks/new Conventional Commit/no amend or bypass,
normal push/ready PR are standing authorized only after implementation release.
Preserve published SHA unless correcting an actual finding. Return the heavy
lease after all local/publication joins, owned processes gone, clean exact remote
head, before one retained 45-minute all-terminal collector with owned watchdog.
No duplicate collectors/timer GitHub queries; join and prove gone before a
ROOT-bounded replacement. CI reruns need ROOT's named workflow/job grant.

Inspect one sufficient automatic substantive CodeRabbit App347564 full exact-head
all-file review during CI. Progress/ACK is insufficient; one necessary full request
only for actual skip/gap. Disposition actual findings; reacquire heavy for local
fixup. Require six actual required contexts and successful Backend/Frontend/E2E
parents, fresh exact-head policy/review/governance/resolver/nohidden/actionable/
changesrequested/errors/human gates and all handles joined before MERGE-ready.
Separate ROOT serial MERGE lease precedes normal expected-head squash/noadmin and
verification of actual merged SHA/tree/all blobs/remote-main inclusion/cleanup.
ROOT then owns independent verification/archive/checksum-only proofrelease/refill.
Plan completion, checker timeout and publication are not delivery completion.

## Verification results

Design validation passed: catalog validation (351 decisions, 1371 specifications),
focused catalog discovers both new UI files, full specification lint, public-doc
source validator (47 pages), and exported PR-documentation coverage preflight
(`covered`, no errors) using the prospective bounded implementation inventory.
That inventory is validation input, not a claim of source/test changes. The actual
four-document-only diff is documentation-exempt in the same checker.

At DESIGN END, whitespace/diff/status checks confirmed exactly four new
unstaged/uncommitted Markdown artifacts with no implementation or heavy commands.
ROOT later reviewed their exact hashes and released implementation in this same
primary session. That historical checkpoint is preserved in the Kandev task plan.

After release, the single pinned frozen install passed. Clean causal RED on the
original hook produced 14 assertion failures and eight passing controls in the
new 22-test suite. The first RED run also exposed an unawaited navigation `act`
in the owned fixture; it was corrected before the clean causal run, without
weakening assertions or timing. Production edits followed the clean RED.

Final combined Vitest passed 29/29 (22 real-protocol and seven existing controls),
changed-file ESLint passed with zero warnings, one normal typecheck passed, and
i18n check plus ratchet passed. ESLint's sole initial warning was the new suite's
outer describe length; splitting into ordinary focused groups and formatting
changed inputs, so both affected ESLint and targeted tests were rerun once.

Catalog, full spec lint, public-doc source validation (47 pages), actual six-file
PR documentation coverage (`covered`, no errors), and tracked/untracked whitespace
passed. The existing search test file required no changes. No consumer/API/WS/
backend/layout/copy or public documentation change occurred. The narrow mobile
state/data exception applies; no browser, E2E or build was run.

All started local commands actually joined, without timeouts; owned process
groups are gone. Exact commands, starts, deadlines, handles, PIDs, logs and results
are recorded in the Kandev task plan and owned receipts under
`/tmp/kandev-child44-search-tpc2n89y/`. Managed dependencies and original ROOT
evidence are retained. Task 01 is done and the verified specifications are active/
current. Delivery is pending; no merge lease has been granted.

## Risks

- A session-string-only guard admits A-to-B-to-A work; ownership must distinguish
  committed lifetimes without retiring current callbacks on ordinary rerenders.
- Timer clearing alone does not retire already issued wire requests.
- Retired finally/error paths can silently corrupt a new loading/results state.
- StrictMode replay must restore live ownership; render-time ref writes must not
  retire work for abandoned renders.
- Shared PanelSearchBar queues even at debounceMs=0; owned hook admission must
  reject stale/closed callbacks without changing that shared component.
