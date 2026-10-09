---
created: 2026-10-08
status: implemented
requirements:
  - REQ-TASKS-COMMENT-DRAFT-001
system_design:
  - ../../specs/tasks/system-design/comment-draft-preservation.md
legacy_specs: []
---

# Implementation plan: Preserve the next task comment while sending

## Overview

Keep later task instructions when an older comment send succeeds. One sequential
work order adds real rendered transport regressions and changes only the
acknowledgement-time clearing decision. ROOT reviewed the full design package
and later authorized implementation in this same primary session.

## Identity and source checkpoint

- External identity: `root-task-comment-draft-preservation-20261008-85`.
- Task: `6d88c606-1f2d-40e8-9e72-9efa169861b8`.
- Primary session: `94cbc674-dd0b-4a9d-ac5d-2c6cee47a570`.
- Worktree: `/home/jcfs/.kandev/tasks/preserve-the-next-ta_dbgpz8pr/kandev`.
- Branch: `feature/preserve-the-next-ta-dls`.
- Base/head: `c2c243b2ac1ad4068f0062c823d7c0bd2597e677`; actual remote main read
  once on 2026-10-08 matches it. Initial worktree was clean.
- Title remains ROOT-owned: **Preserve the next task comment while sending**.

ROOT's accepted read-only evidence at
`/tmp/kandev-root-task-comment-draft-discovery-20261008/candidate.test.tsx`
is regular0400 with SHA256
`ad6bfff53c4200fc9f4a72e2963cfe3f82229631ba136afbefc6afeeea6479c3`.
Its qualified proof/receipt/log record `native12656/startd496fa/ACTUALJOIN3e82df`,
`actualVitest1`, one causal expected RED and two PASS controls. Actual TaskChat
and providers were rendered; only fetch transport was mocked. Actual trimmed
POST/user attribution and callback once preceded loss of B. ROOT reports the
temporary test removed, owned group518437 absent, and clean source. The proof
source base was `5638725e5d22d8089269fc53a5b79ed61d9b9445`; ROOT verified current
TaskChat/API byte-identical on `c2c243b2`. Accept this evidence without replay,
copy, import, edit, chmod, or deletion. New permanent tests must be authored
independently after the implementation grant.

## Scope

### In scope

- [Requirement](../../specs/tasks/requirements/comment-draft-preservation.md)
  `REQ-TASKS-COMMENT-DRAFT-001`, acceptance `.1` through `.8`.
- Exact raw snapshot comparison on successful comment send.
- Real rendered transport tests and affected-path validation.
- Same-value changes from asynchronous file insertion; existing synchronized
  prompt-delivery integration controls.

### Out of scope

- Navigation/session ownership, unmount feedback, draft persistence,
  comment transport/API/backend/events, and other chat inputs.
- Layout, copy, touch sizing, scroll, responsive/navigation changes; file or
  utility delivery redesign; shared draft manager/framework.
- New flags, infrastructure, generic QA/review, broad suites, browser runs,
  builds, synthetic merge tests, and moving-main rebases.

## Technical approach

Follow the [system design](../../specs/tasks/system-design/comment-draft-preservation.md).
`ChatInput.handleSubmit` already captures raw `inputValueRef.current`, admits
only eligible non-pending sends, and posts a trimmed body. After success,
replace unconditional `setInputAndSync("")` with the synchronized functional
equality rule. Preserve the callback, catch feedback, and finally settlement.

Do not modify `synchronizeInputValue` or `usePromptResultDelivery`: inspection
confirmed the existing helper evaluates its functional update against the
latest ref and synchronizes it before scheduling state. The component's shared
setter already covers typing, file insertion, and prompt application.

## ASCII UI preview

**UI-01: Existing task comment composer, pending and acknowledged.** Entry:
task Chat tab through `ChatActivityTabs`. Shared desktop/phone composition.
Illustrative brackets describe existing controls, not new product labels.

```text
Pending A, after typing B        A acknowledged
+-------------------------+     +-------------------------+
| B (editable draft)      | --> | B (editable draft)      |
| [attach] [enhance]      |     | [attach] [enhance]      |
|        [send disabled]  |     |        [send available] |
+-------------------------+     +-------------------------+
```

Current causal defect: the right-hand text becomes empty. Proposed behavior:
B stays exactly as typed. Unchanged A, or exact A restored before success,
clears. Empty/whitespace-only B remains empty/whitespace-only with sending
disabled; failure retains B and existing error feedback. A successful send
refreshes comments regardless of whether the text clears.

No control order, grouping, fixed/scrolling region, touch geometry, copy, or
navigation changes are required. The surrounding surface retains its scroll
owner. Phone uses the same textarea and completion rule; there is no separate
phone composition to design. AC `.1` through `.8` map to the rendered test
matrix below. Spacing is illustrative, not a pixel specification.

## Tests

The [work order](task-01-preserve-comment-draft.md#rendered-regression-matrix)
owns the exact named scenarios in
`apps/web/components/task/simple/task-chat.comment-send.test.tsx`. It tests the
actual rendered TaskChat/provider/store/API chain with only fetch mocked.
Existing `task-chat.test.tsx` and `use-prompt-result-delivery.test.ts` remain
focused controls. The work order lists one worker/4 GiB Node commands and
actual changed-path coverage, lint, typecheck, i18n, and documentation checks.

## E2E evidence and mobile exception

End-to-end component-to-fetch evidence covers the reported race without a
backend, browser, or mocked composer. Deferred transport holds A pending while
the real textarea changes to B, then acknowledges A and observes B. This is a
pure state/data change within an existing component. The mobile-parity exception
permits these targeted rendered tests because layout, touch, scrolling,
navigation, and viewport-dependent interaction remain untouched. No new
Playwright file/project or visual check is planned. Any later change to those
boundaries requires ROOT scope reconciliation before adopting this exception.

## Work orders

- [x] [Task 01: Preserve the next task comment](task-01-preserve-comment-draft.md)

One wave, sequential, no dependencies. No delegation is authorized.

## Execution and delivery gates

ROOT reviewed all four files after DESIGN END and sent a LATER explicit
implementation INTERRUPT to this actual primary, granting EXCLUSIVE GLOBAL
LOCAL-HEAVY85 after independently qualifying Child84's release. Scope and eight
ACs remain unchanged. Review receipt:
`/tmp/kandev-root-child85-design-review-20261008.json`. Browser, build, and broad
suites remain excluded. Do not ask
operator approval/model-switch questions or create agents/tasks/tabs/sessions.
Persist progress in the version-safe Kandev task plan, preserving its system
marker, identity, user edits, title ownership, question barriers, and final-action
rules. ROOT reads directly; no parent callback/notification/ACK gate.

Child84 exclusively holds GLOBAL LOCAL-HEAVY at design time. Later local-heavy
work requires a serial ROOT grant after that lease is released. If dependencies
remain absent, run one pinned pnpm9.15.9 frozen install from `apps/`, only then.
Retain and ACTUALLYJOIN every original handle; prove owned groups gone before
explicit RETURN. Routine causal fixture/lint repairs are affected-only;
resource/timeout/transport/unknown/outside-scope failures checkpoint ROOT before
retry. A parent question, if critically necessary, ends the turn immediately.

After later delivery authorization, use normal active hooks and new commits,
without bypass/amend. Freeze head except real corrections. Retain one 90-minute
all-terminal observer (GNU timeout91m, kill-after10s, cadence60s); ACTUALLYJOIN
before any ROOT-authorized replacement. All six required checks and actual
Backend/Frontend/E2E parent workflows must succeed. Findings must be fresh,
complete/error-free, exact-current-head and human-gate clear. Accept authenticated
configured App347564 substantive FULL exact-head/all-files automatic review;
request review once only for an actual gap, with no redundant review wait.
Hosted retry and MERGE need separate ROOT grants. Normal expected-head squash
completion requires actual merged SHA/tree/blobs/remote verification and joined
only-owned cleanup. Preserve clean managed worktree/deps/caches for ROOT archive.

## Verification results

DESIGN END on 2026-10-08. All four files remain unstaged/uncommitted;
requirement/design/plan are `draft` and the sole work order is `pending`.

- Catalog validation: exit 0, 364 decisions and 1464 specifications.
- Specification linter tests: exit 0, 36 tests passed.
- Full specification lint: exit 0, all specification files passed. Original
  native session94922 was actually joined to terminal exit 0, chunk84ff92.
- Actual changed-path PR documentation preflight: exit 0, four documentation
  paths correctly classified `exempt`, with no errors. Since that exemption
  does not validate package references, the additional requirement/AC/design/
  manifest/work-order assertions also passed using the installed Node24 binary.
  The login-shell PATH lacked Node; no tool installation was performed.
- Both new specs appear in the Tasks catalog; all eight AC references resolve.
- Whitespace/status audit: passed; only the four package files are untracked,
  no staged changes and no `apps/` changes.

At DESIGN END, product verification was deliberately not run and no local-heavy
lease was held. The later reviewed implementation grant and results follow.

### Reviewed implementation results

ROOT accepted the eight ACs and single work order, then granted implementation
and EXCLUSIVE GLOBAL LOCAL-HEAVY85 after Child84's independently qualified
release. The permanent suite was independently authored; protected proof was
never accessed, copied, imported, modified, deleted, or replayed.

- Frozen install: pnpm9.15.9, one conditional install from `apps/`, exit 0.
  Native17427/start11d92d/ACTUALJOINde8c71; PG746132 gone.
- New causal RED on unchanged production: native44815/start7d1862/
  ACTUALJOIN3c9a09, expected exit 1: one lost-NEXT assertion, five PASS controls,
  15 unselected cases; PG753343 gone. Actual rendered TaskChat/providers/store,
  actual POST/body/user attribution, fetch-only transport mock.
- Minimal production correction: one conditional synchronized clear. Scoped
  GREEN native88739/start91d38e/ACTUALJOIN910a02, exit 0, 3 files/51 tests PASS;
  PG754828 gone.
- Formatter/changed lint: native36463/start0749c7/ACTUALJOIN3ff437, exit 1 only
  for a duplicated own fixture string; PG756874 gone. Affected-only fixture
  constant repair, final new suite21 PASS and both changed TS/TSX lint clean:
  native43872/start5a596d/ACTUALJOIN7cc9af exit 0; PG758355 gone.
- Web typecheck: native22980/startf31ecb/ACTUALJOINf46f81 exit 0, PG759210 gone.
  Pretypecheck generators produced no tracked drift.
- i18n check and base-pinned ratchet: native30807/start7f303c/ACTUALJOIN706702
  exit 0; PG764865 gone. No copy/catalog changes. Existing 436 orphan-key
  warnings were informational; completeness gates passed in all locales.
- Final docs/catalog/spec lint and actual six-path coverage:
  native29915/startab8f0e/ACTUALJOIN35d780 exit 0; PG769843 gone. Coverage is
  `covered`, with accepted requirement/design/work-order references and no errors.
- Normal initial commit hook reformatted four fixture indentation lines and
  stopped the attempt, native73010/startb50b37/ACTUALJOIN5de64d exit 1,
  PG772328 gone. No bypass or amend. All applicable lint/doc/architecture/copy
  hooks otherwise passed. Final formatted new21 cases passed:
  native25251/startb230fa/ACTUALJOINd3df7b exit 0, PG776171 gone. Re-stage and
  create a new normal commit; final hook/publication receipts stay in task plan.

Full original native exec/join responses and raw command logs are retained in
`/tmp/kandev-child85-local/native-receipts.json` and its sibling label logs/JSON
verdicts. All product commands ran serially with one Vitest worker/4 GiB Node.
Existing TaskChat/prompt-delivery tests stayed unchanged. Public docs, helpers,
API, backend, other composers, layout/copy/touch/navigation stayed unchanged.
No browser/build/broad suite or synthetic merge test was run. Normal hook,
publication, explicit lease RETURN, hosted observer, and merge gates remain in
the platform task plan; no merge authorization is implied by implementation.

## Risks

- Trimming comparison would still erase raw whitespace edits.
- Bypassing the synchronized setter would desynchronize utility prompt reads.
- Mocked composer/API helpers would fail to prove the real transport boundary.
- Future unrelated ownership or presentation work must not expand this fix.

## Public documentation

No public-doc change needed. The audit found separate session and attachment
contracts but no documentation of this reset race or procedure requiring change.
Internal requirements/design/plan record the correction.
