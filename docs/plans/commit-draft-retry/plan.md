---
created: 2026-10-08
status: implemented
requirements:
  - REQ-WORKSPACES-COMMIT-DRAFT-RETRY-001
system_design:
  - ../../specs/workspaces/system-design/commit-draft-retry.md
legacy_specs: []
---

# Implementation plan: Keep failed commit messages available for retry

## Overview

One sequential work order adds an explicit feedback acknowledgement and local
commit draft ownership, proves the real dialog recovery flow, and updates the
existing public how-to. ROOT reviewed the concrete four-file package and sent a
later explicit implementation release in
task `d3fa44a4-db3f-4e3d-9d61-82dbfb0f57e5`, primary session
`ad44d4cc-4a7f-4f81-9f7b-0a405be2d66d`. Local implementation and task-defined
checks passed. A hosted E2E finding requires the narrow test-only correction
below, now verified at all three widths. Publication, hosted evidence and a separately authorized
merge remain external delivery gates. The completed design turn made no product
edits, test/install/build/browser runs, commits or publications.

## Scope

### In scope

- The [owning requirement](../../specs/workspaces/requirements/commit-draft-retry.md)
  and its nine criteria, using unchanged native dialog composition.
- Feature-local state/acknowledgement, exact payloads, bounded pending and scope
  guards, and real-provider rendered integration evidence.
- A short failed-commit retry note in the existing public commit how-to during
  implementation, plus normal task-defined delivery gates after ROOT release.

### Out of scope

- Backend/API/permission changes, amend/reset/other Git operations, rollback,
  abort transport, automatic retries, persistence, per-repository draft caches,
  generic mutation infrastructure and arbitrary session-lifetime redesign.
- New rendered composition, copy, layout, touch behavior or navigation.
- Delegates, recursive tasks, additional sessions/tabs/model switches,
  ungranted hosted retries or merge, synthetic merge tests or main-only rebases.

## Technical approach

Apply the [system design](../../specs/workspaces/system-design/commit-draft-retry.md)
inside `components/vcs/vcs-dialogs.tsx` and `hooks/use-git-with-feedback.ts`.
Extract the commit-only state to `components/vcs/use-commit-dialog-state.ts` to
avoid growing the existing dialog file past its 600-line limit. Return a local
boolean acknowledgement, capture raw draft/choice values at admission, and
settle only the owning generation/revision. Shared hook callers keep ignoring
the return value; useSessionGit fan-out and WebSocket contracts stay intact.

| Path | Scope/outcome | Required evidence |
| --- | --- | --- |
| Single repository | Trimmed payload; false/rejection retains raw draft; true resets | Native provider transport/input assertions |
| Explicit root or named repository | Preserve empty-string versus omitted scope; captured Stage all | Payload and reopening assertions |
| All repositories | Existing aggregate false retains draft after partial success | Real fan-out with one successful and one failed transport result |
| Pending and newer draft | No duplicate admission or late-owner clearing | Deferred responses; edits, reopen, scope transitions |
| Other feedback callers | Ignore boolean; same operations/toasts | Hook toast tests and web typecheck |

## Desktop and phone state semantics

The production change retains the existing `CommitDialog` and `CommitBodyField`
composition, copy, classes, scrolling, touch targets, navigation and breakpoints.
Component tests prove shared state semantics, not browser geometry or live Git.
The actual hosted hook-rejection failure additionally requires browser integration
coverage: dismiss the intentionally retained modal before background chat actions.
The same existing scenario runs at 1280px, 393px and 767px, using shipped phone
Changes/Chat navigation. ROOT authorized this focused chromium viewport selection
instead of a new mobile file/project. No touch-device or geometry claim is made.

## Accepted ROOT evidence

Read-only proof metadata resides at
`/tmp/kandev-root-commit-draft-discovery-20261008/qualified-proof.json`,
`receipt.json`, `draft-bytes-receipt.json` and `draft-bytes-output.log`.
The immutable strengthened candidate is regular mode 0400, SHA256
`8e22101518968c3d009385522d4f0544525e4368255e81e4569b6d20b7a3dd14`.
Do not replay, copy, import, edit or delete either candidate.

- Initial native `50322`, start `b36511`, actual join `c8d9ff`, test exit 1:
  two failure cases proved disappearance; one success control passed. This did
  not itself prove reopened draft bytes. Initial candidate digest was
  `d78ee5896871e961a17c6245f9a484843c30462ba83649ee73a5b21c2890e468`.
- Strengthened native `86390`, start `1e77cd`, actual join `405872`, test exit 1:
  both failures explicitly reopened the actual dialog and found the typed title
  empty; one success control passed. Real provider/store/toast/hooks were used;
  only WebSocket and frontend report transports were mocked.
- Receipts record all originals joined, owned groups absent, temporary tests
  removed and ROOT clean. No rerun is needed for design reassurance.
- Proof base `5638725e5d22d8089269fc53a5b79ed61d9b9445`, unchanged relevant sources
  through known main `7697d361c51cf1e73f6ab72b41d19a57a1a397d5`. Actual inspected
  current main/HEAD is `fd4f6c7583e77b546a926128cfe1fda0965af0f8`; cheap comparison
  found no changes in vcs-dialogs, use-git-with-feedback, use-session-git or
  use-git-operations. Accepted proof is diagnostic evidence, not future
  implementation coverage.

## Tests

| Criteria of REQ-WORKSPACES-COMMIT-DRAFT-RETRY-001 | Implemented evidence |
| --- | --- |
| `.1`, `.2`, `.3` | `vcs-dialogs.commit.test.tsx`: reported failure/rejection, reopen exact draft, retry then success/reset; `use-git-with-feedback.test.tsx`: false/throw/true and existing toast text/variants |
| `.4` | Native blank-title admission and request formatting; hook blank callback admission |
| `.5` | State and native deferred tests: title/body/Stage all changes, edit/revert, payload capture, duplicate callback admission |
| `.6`, `.7` | State and native tests: dismissal/reopen, undefined/root/named scope, newer attempt versus old settlement, session/environment change away/back and unmount |
| `.8` | Native seeded multi-repository fan-out, one success plus one failure, refreshed Git state and deliberate retry |
| `.9` | Shared native flow at desktop/phone window sizes, viewport change without remount |

Test files live under `apps/web/components/vcs/` except the feedback-hook suite
under `apps/web/hooks/`. The order specifies the exact capped command and
unchanged helper/payload controls. All five selected suites passed; the two
existing controls ran once as scoped compatibility checks.

## E2E evidence boundary

The original component evidence uses real providers and hooks with mocked
transport. Hosted run 37787894133, job 113360049681, failed the existing
`git-commit.spec.ts` hook-rejection scenario in all three attempts at the background
Technical details click. Raw logs and all three page contexts identify the retained
commit textarea/modal intercepting pointer events. The scenario must assert raw
failure retention and enabled Cancel, dismiss/reopen and recheck the same draft,
dismiss again, then preserve every original summary, hidden-output, Technical
details, Fix, prompt and agent-response assertion. The single owning work order
is reopened for this ROOT-authorized test-only correction and one focused managed
fresh-build run. No production change or hosted retry is authorized by this finding.

## Public documentation audit

Search covered `docs/public`, root README, `docs/screenshots.md`, specifications
and decisions. `docs/public/sessions-and-review.md`, under **Commit and open a
change request**, explains staged-default title/body entry but has no failed
draft recovery guarantee at the inspected base. The existing how-to now includes
a focused recovery note. `git-operations.md` remains the general Git
operation/permission reference; labels, screenshots and commands do not change.
No public document was edited during the completed design-only turn.

## Work orders

- [x] [Task 01: Preserve commit drafts through failed acknowledgements](task-01-preserve-commit-drafts.md)

One dependency wave, sequential in this same primary conversation. No agent
delegation or separate implementation layer task.

## Verification results

Historical documentation-only design checks passed on 2026-10-08:

- Catalog validation: 364 decisions and 1462 specifications; the new pair is
  discoverable under Workspaces.
- Specification-linter tests: 36 passed; full specification lint passed.
- Actual documentation coverage preflight: ok, errors empty, exempt because
  only the four design files changed. A separate explicitly planned-production
  path projection returned covered, errors empty, with this work order and
  requirement/design reference accepted. It is traceability evidence, not an
  executed production diff or product test.
- Scoped git whitespace/status checks passed; four owned files remain untracked,
  unstaged and uncommitted. No production/permanent-test changes.

Node was absent from shell PATH (the first coverage command did not execute,
exit 127). Read-only discovery found existing Node 24.21.0 at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`; both coverage
preflights ran through that executable, with no installation.

After ROOT's release, independently authored tests produced causal RED through
the real dialog and explicit acknowledgement expectations. RED included one
fixture wording defect, so not every failing assertion is claimed as product
proof. The approved boolean acknowledgement and bounded commit state owner then
passed the exact five-suite, one-worker, Node 4 GiB Vitest selection: 50 tests.
The existing helper/payload controls ran once. An independent-repository fixture
was corrected to respect existing dependency dispatch; no Git dispatch changed.

Changed-file eslint passed after splitting a test describe group. Typecheck
passed after repairing three integration-fixture typing errors. Affected state
and integration suites passed after those repairs (10 and 16 tests); these are
reruns within the same 50 unique tests. Production did not change after the final
five-suite green. i18n check and changed-line ratchet passed with no product copy
or locale changes; normal staged hooks also gate newly added files.

Catalog validation, 36 specification-linter tests, full specification lint,
62 public-doc validator tests and all 47 published pages passed. Actual changed
production-path documentation coverage returned covered, errors empty, through
this owning requirement/design/work-order chain. Scoped whitespace/status
checks confirmed only the eleven owned files changed.

Native start/intermediate/actual-terminal receipts and UTC/PID/group evidence
are retained in `/tmp/kandev-child84-execution`. The one frozen pnpm 9.15.9 install
subprocess was actually waited/reaped to exit 0, but its native handle was omitted
from the initial result. ROOT qualified terminal recovery for exactly that
install in `/tmp/kandev-root-child84-install-terminal-recovery-20261008.json`;
no native-level install join is claimed and no reinstall occurred. Every later
live native handle was retained and joined. Hooks/publication/hosted review and
merge verification remain external until they actually occur; see the work order.

## Risks

- Trimming the retained draft, truthiness-collapsing root scope, unconditional
  reopening and a stale success clearing newer edits.
- React closure/cleanup ordering, A-to-B-to-A identity reuse and an old request
  releasing a newer pending guard. Assertions must prove outcomes before and
  after actual settlement, not only flags or helper return values.
- Shared loading state alone cannot own a commit attempt; local admission must
  remain effective through unrelated refreshes and old request settlement.
- Multi-repository partial success is not rollback. Retry uses current status.
- The one pinned frozen install completed after release and heavy-lease admission,
  with its bounded native-handle recovery exception recorded above.

## Hosted integration correction results

ROOT amended the same sequential order for the causal modal-obstruction failure.
One focused managed fresh-build run passed 3/3 tests at desktop1280px and native
phone393px/767px, one worker and retries0: native23619/start e10a69/actual
join1ebd67 exit0, group1024527 absent. The existing scenario now checks raw
whitespace/newline title/body, enabled dismissal, reopen with the same draft,
then dismissal before all original chat summary/output/details/Fix/agent-response
assertions. Phone cases use shipped bottom navigation and MobileChangesPanel.
These are real browser/live rejecting Git hook outcomes with a fine pointer;
no touch-device geometry is claimed. The old observer25812 reached deadline,
actual join7a4e5e exit2, group711830 absent, and was not replaced before joining.
No earlier passing50 unit tests were replayed. Web typecheck excludes e2e and
localization guards exclude test copy; the correction needs affected eslint,
document traceability and normal hooks, not repeated typecheck/i18n suites.

Affected E2E eslint passed. Catalog/spec lint and actual twelve-path PR
documentation coverage passed with covered status and errors empty. The public
retry note already owns dismiss/reopen behavior; the test-only correction
requires no additional public document change.
