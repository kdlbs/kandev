---
created: 2026-10-05
status: implemented
requirements:
  - REQ-UI-PREVIEW-URL-DETECTION-001
system_design:
  - ../../specs/ui/system-design/preview-url-detection.md
legacy_specs: []
---

# Implementation plan: Reject invalid preview ports

## Overview

Validate complete numeric port candidates so invalid announcements cannot
replace a working preview. Deliver one sequential work order, with regression
tests and the correction in the two existing detector files.

ROOT reviewed all four design artifacts and explicitly released implementation
and normal delivery on 2026-10-05 in this same session. The original design
handoff was unstaged/uncommitted. Execution belongs to task `681d352e-e2f5-4f23-b576-021ef5e8908d`, session
`583c980c-ff34-47e8-a4ed-fb480ac950fb`. ROOT is
`14825981-b175-411d-999a-31ddc2aa5fc3`. Keep this primary agent/executor and
session; no delegation or model switching. Parent notifications are full, so
record checkpoints in this task's versioned live plan; do not send callbacks.
Paused task `a6032d95-cc1e-4db8-adca-5b643ae4138d` and its worktree are excluded.

## Evidence and ownership

Accepted ROOT evidence on authoritative main
`f18fe2d9e3bafb567aff643f25356ac3ed651120`, without replay:

- Complete unchanged detector source SHA256:
  `8ffb095c295cb4ea9740d9b1e49f22606ec96d977a21912c49d08e1145c9aec3`.
- Read-only Node24 proof `/tmp/kandev-preview-port-repro.mjs`, SHA256
  `4f019726210e636642afd8aa8ecb7fd95d8f45c1c4e6d7701dceabba8a25e9df`.
  The joined run exited 1. It loaded the complete module with
  `stripTypeScriptTypes` and VM `SourceTextModule`, mocking only backend config.
- Bare3000 and full65535 controls passed; bare/full65536 were accepted,
  `123456` truncated to `12345`, and later70000 superseded earlier3000 so the
  exported output-to-rewrite pipeline returned null.

Preserve the ROOT proof until parent postmerge verification. The cause is the
unchecked bare fallback and its partial digit match. UI owns the reusable
candidate-selection contract. Existing manual port-opening and task-owned
feedback specs have different lifecycles; use the new focused
[requirements](../../specs/ui/requirements/preview-url-detection.md) and
[design](../../specs/ui/system-design/preview-url-detection.md).

## Scope

- Only `apps/web/lib/preview-url-detector.ts`, its existing test file, this
  requirement/design pair, and this plan with one work order may change.
- Validate complete numeric tokens and preserve selection among valid candidates.
- Preserve detector boundary 0..65535 and existing bare width, including zero
  and leading zeros. Backend admission stays at its separate 1024..65535 rule.
- Exclude backend/proxy policy, APIs, dependencies, schemas, lifetime,
  coordinators, frameworks, copy, layout, and unrelated URL normalization.
- No installs, Go, builds, E2E, app runtime, permanent tests/code, or publication
  during the design turn.

## Technical approach

Follow the paired design in the existing `detectPreviewUrl` and
`tryParseHostPort` path. Prevent partial numeric regex matches and skip invalid
bare candidates while walking backward. Retain forward full matching and
last-valid-line selection; the exported rewrite and both actual callers stay
unchanged. A fix needing broader domain changes must checkpoint for ROOT rather
than expand this package.

## Tests

All cases belong in `apps/web/lib/preview-url-detector.test.ts`.

| Criteria | Permanent evidence to add or retain |
| --- | --- |
| `.1` | `rejects overflow and overlong numeric port tokens`: table of all three hosts, bare/full forms, 65536, 70000, 123456; assert null |
| `.2` | `accepts the upper numeric boundary` and `preserves existing lower-bound and bare-width behavior`; existing scheme/path/query/hash/ANSI tests |
| `.3` | `preserves full-first and last-valid-bare preference with invalid candidates`: both relative orders and multiple valid controls |
| `.4` | `retains the working proxy after invalid later output`: real output detector then real rewrite, valid path/query/hash then invalid bare/full overflow; also upper-bound pipeline and invalid-only output |
| `.5` | Same shared detector and pipeline cases; pure data normalization mobile exception |

The `.N` suffixes above refer to `AC-UI-PREVIEW-URL-DETECTION-001.N`.
Write the new regressions and observe meaningful RED before production changes;
then implement the smallest GREEN correction. Do not replay the ROOT proof.

Concrete selection controls for those tests:

| Input | Expected detector selection |
| --- | --- |
| `localhost:3000 localhost:70000` | `http://localhost:3000` |
| `localhost:70000 localhost:3000 localhost:3001` | `http://localhost:3001` |
| `localhost:3001 http://localhost:3000 localhost:70000` | `http://localhost:3000/` |
| `http://localhost:65536 http://localhost:3000 http://localhost:3001` | `http://localhost:3000/` |
| `http://localhost:70000 localhost:3000 localhost:123456` | `http://localhost:3000` |
| `http://localhost:3000/app?debug=true#route` followed by an invalid bare or full line | Earlier complete URL retained; rewrite equals `http://localhost:8080/port-proxy/test-session-123/3000/app?debug=true#route` |

Lower-bound controls preserve full `:0` and bare `:00`, full `:1`, bare `:10`,
and zero-padded bare `:03000`; one-digit bare `:1` remains unmatched. These
controls characterize detector compatibility, not proxy admission. Upper-bound
full and bare65535 pipeline cases assert the exact proxy path ending `/65535/`.

## E2E and mobile parity

Use the mobile-parity pure data normalization exception. No layout, touch,
scrolling, navigation, copy, or viewport branch changes. Test the actual exported
output-to-rewrite chain used by the existing consumers; do not run app runtime,
Go builds, or browser E2E for this correction. Rendered verification is not run
because no rendered surface changes. ASCII UI previews do not apply.

## Work orders

- [x] [Task 01: Validate complete preview ports](task-01-validate-preview-ports.md)
  (`done`, wave 1, no dependencies, sequential).

## Verification strategy

The work order owns exact commands. After later release, perform one conditional
pnpm9.15.9 frozen install from `apps/` only if dependencies are missing. Run
targeted RED/GREEN with one worker and 4 GiB Node heap, changed-file lint and
formatting, web typecheck, i18n checks, specification gates, actual changed-file
documentation coverage, and normal active commit hooks. One heavy local command
at a time; retain and join all handles. Resource failures require a bounded
ROOT decision; no automatic repeat, foreign kills, or cache wipes.

## Delivery gates after later release

Use normal Conventional commit, push, and ready PR. Freeze the published SHA
except for real required corrections; no main-drift rebase or optional polish.
Use exactly one joined all-terminal PR monitor. Require configured authenticated
CodeRabbit App347564 substantive FULL current-head review over all actual files;
automatic coverage suffices. Inspect a gap before one necessary request.
Disposition actual findings; acknowledgments/skips do not count as review.
Require clean actual terminal required gates before normal expected-head squash.

Completion requires actual merge and owned cleanup. Independently verify the
actual merge SHA/tree/owned blobs and authoritative remote main, join handles,
verify clean managed worktree, and preserve ROOT's proof for parent verification.
ROOT owns archive and advancement to the next small task.

## Verification results

Design preflight passed on 2026-10-05:

- `python3 scripts/list-docs.py validate`: 349 decisions and 1345 specifications.
- `python3 scripts/lint-spec-files.py --all`: pass.
- `python3 scripts/lint-spec-files.test.py`: 36 tests pass.
- Catalog lookup finds both new `preview-url-detection` documents.
- Real `.github/scripts/pr-docs.cjs` `validateCoverage` over the actual four
  untracked docs: `exempt`, zero errors. Separate prospective evaluation with
  the two permitted detector files: `covered`, zero errors; linked work order,
  requirement, acceptance IDs, and design accepted. This prospective result
  does not claim implementation or coverage of a future actual code diff.
- Diff/whitespace checks pass, including direct validation of untracked docs.
  Git reports only the four new docs; index and both detector files are unchanged.

The bare `node` command was absent from the session PATH. Documentation coverage
completed with existing Node24 at
`/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`; no installation
occurred. RTK is unavailable, so the documented coverage command reads raw native
Git subprocess output without a display helper.

Implementation completed after ROOT's explicit reviewed-package release.
Permanent RED recorded 37 expected failures; final GREEN passed all 99 tests.
Scoped lint/formatting, web typecheck, both i18n checks, specification gates,
diff checks, and actual six-file documentation coverage pass. The only required
local correction after GREEN was extracting a repeated test literal for lint.
See [Task 01 results](task-01-validate-preview-ports.md#results) for exact commands
and outcomes. All local command handles are joined; one conditional pinned
frozen dependency install completed.

The paired requirement/design now describe the conforming implementation with
`active`/`current` statuses. Hook receipts and exact-head hosted CI/review,
expected-head squash, independent actual-merge verification, and owned cleanup
remain delivery gates recorded in the versioned live plan. `implemented` does
not mean the task has merged or completed.

## Risks

- A regex boundary that permits backtracking could retain truncation.
- Checking only the last bare match could discard an earlier valid candidate.
- Sharing the proxy minimum would change supported detector inputs and scope.
- Native URL default-port normalization and ANSI fallback have existing quirks;
  preserve them rather than broaden this repair.
