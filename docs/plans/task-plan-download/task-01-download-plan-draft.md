---
id: "01-download-plan-draft"
title: "Download the current Plan draft"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-TASK-PLAN-DOWNLOAD-001
acceptance_criteria:
  - AC-UI-TASK-PLAN-DOWNLOAD-001.1
  - AC-UI-TASK-PLAN-DOWNLOAD-001.2
  - AC-UI-TASK-PLAN-DOWNLOAD-001.3
  - AC-UI-TASK-PLAN-DOWNLOAD-001.4
  - AC-UI-TASK-PLAN-DOWNLOAD-001.5
system_design:
  - ../../specs/ui/system-design/task-plan-download.md
---

# Task 01: Download the Current Plan Draft

## Summary

Expose the existing live draft as a local Markdown download from the main
Plan header. Prove consumer wiring, user-visible download bytes, responsive
reachability and documentation without altering persistence or helper internals.

## In scope

- Header action, optional small button component, mounted tests, focused
  desktop and mobile E2E, and the public Plan usage paragraph.
- Follow the [requirements](../../specs/ui/requirements/task-plan-download.md),
  [design](../../specs/ui/system-design/task-plan-download.md), and
  [test matrix](plan.md#tests).

## Out of scope

Revision and preview downloads, clipboard, new export format or protocol,
backend/workspace writes, save/implement/autosave/lifecycle changes, and
redesign of neighboring controls. No production work or permanent tests in
the design turn.

## Acceptance

1. The mounted header uses the current draft with no session dependency and
   proves disabled and error states without calling save or implement.
2. Real desktop and phone downloads have the exact current UTF-8 Markdown and
   fixed filename; accessible activation, geometry, containment, and visual
   captures meet the plan's responsive matrix.
3. The small public how-to paragraph and all targeted checks are complete;
   lifecycle statuses and results record actual evidence.

## ASCII UI preview

**UI-01: Main Plan header**, excerpt from the [full preview](plan.md#ascii-ui-preview),
criteria `.1`-`.5`:

```text
Desktop:  |                 [Implement v] [Download] [History] |
Phone:    | [Implement v] [Download] [Hist] |
          | Current Plan draft ...          |
```

The header stays above the scrolling editor; phone navigation remains in its
existing place. Download is disabled for empty/whitespace content; a helper
failure shows localized feedback. Spacing is illustrative, action reachability
and the sizing/containment contract are mandatory. At a narrow desktop panel
width the action row scrolls locally, including by keyboard and touch.

## Execution gate and TDD

ROOT must fully read this package and give a later separate implementation
request and exclusive global HEAVY release. Use the same primary session; no
delegation, extra sessions, or unowned cleanup. Re-read current code and public
doc before editing. First add the independently authored mounted header test
with saved Plan content different from a multiline Unicode draft and null
active session. Run it and record a causal RED for the missing Download action;
then implement. Test that changes to draft props before activation are used,
empty/whitespace disables the action, synchronous helper errors show translated
feedback, and neither save nor implement is called. Do not duplicate helper
internals. Update the public how-to paragraph without altering concurrent edits.

## Verification

Only after both gates, run from the repository root, serially. Install once in
this fresh worktree before pnpm or hooks if not already done; preserve the
complete output and every native process handle through actual exit. Each
`runq` invocation gets a unique external log directory; only E2E uses summary.
Do not overlap suites, reuse another worktree's build, kill processes, or clean
unowned caches, fixtures, volumes, or shared temp. Inspect runner cleanup
before execution and preserve owned screenshots, manifest, and hashes outside
managed `.pr-assets` before a subsequent run can clear it.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
runq() { local tag="$1"; shift; KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-plan-download-quiet-XXXXXX)" scripts/run-quiet "$tag" "$@"; }
runq install -- pnpm --dir apps install --frozen-lockfile
runq vitest-red -- pnpm --dir apps/web exec vitest run components/task/task-plan-panel-header.test.tsx
# Implement only after recording the expected RED.
runq vitest -- pnpm --dir apps/web exec vitest run components/task/task-plan-panel-header.test.tsx components/task/task-plan-panel-draft.test.ts components/task/task-plan-panel.save-ack.test.tsx components/task/task-plan-panel.session-switch.test.tsx
runq eslint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/task/task-plan-panel-header.tsx components/task/plan-download-button.tsx components/task/task-plan-panel-header.test.tsx e2e/tests/task/plan-download.spec.ts e2e/tests/task/mobile-plan-download.spec.ts
runq i18n-check -- pnpm --dir apps/web run i18n:check
runq i18n-ratchet -- pnpm --dir apps/web run i18n:ratchet
NODE_OPTIONS=--max-old-space-size=8192 runq typecheck -- pnpm --dir apps/web run typecheck
runq spec-catalog -- python3 scripts/list-docs.py validate
runq spec-lint -- python3 scripts/lint-spec-files.py --all
runq public-doc-tests -- node --test scripts/validate-public-docs.test.mjs
runq public-docs -- node scripts/validate-public-docs.mjs
runq web-build -- pnpm --dir apps --filter @kandev/web build:e2e
runq backend-build -- make -C apps/backend build build-e2e-delivery-fixture
runq plugin-build -- make -C apps/backend e2e-plugin-ui e2e-plugin-package
runq e2e-list-desktop -- pnpm --dir apps/web exec playwright test --config e2e/playwright.config.ts --project chromium --list e2e/tests/task/plan-download.spec.ts
runq e2e-list-mobile -- pnpm --dir apps/web exec playwright test --config e2e/playwright.config.ts --project mobile-chrome --list e2e/tests/task/mobile-plan-download.spec.ts
CAPTURE_PR_ASSETS=1 runq e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/task/plan-download.spec.ts -- --retries=0
# Preserve and inspect desktop captures/manifest/hashes before running mobile.
CAPTURE_PR_ASSETS=1 runq e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-plan-download.spec.ts -- --retries=0
# Preserve and inspect phone captures/manifest/hashes; inspect narrow fine-pointer geometry too.
git diff --check
git status --short
```

If the optional button component is not created, omit its ESLint path; add any
other changed source/test paths to the targeted invocation. Confirm exact test
discovery and inspect `test.describe.configure` overrides so retry zero holds.
Record same-worktree build receipts before `--no-build`; serialize the 8 GiB
typecheck. Run the repository PR-documentation coverage preflight if available.
Record actual command exits and captured visual/measurement evidence, never
infer success from a second run or an unjoined handle.

## Files likely touched

- `apps/web/components/task/task-plan-panel-header.tsx`
- `apps/web/components/task/plan-download-button.tsx` (optional)
- `apps/web/components/task/task-plan-panel-header.test.tsx`
- `apps/web/e2e/tests/task/plan-download.spec.ts`
- `apps/web/e2e/tests/task/mobile-plan-download.spec.ts`
- `apps/web/e2e/tests/task/plan-download-helpers.ts`
- `docs/public/sessions-and-review.md`
- This work order, its plan, and the paired spec lifecycle after verification.

## Dependencies

None. Later separate implementation request and ROOT HEAVY release are gates,
not work-order dependencies.

## Risks

Header clipping, localized label width, editor serialization, and concurrent
public-doc edits require the focused proof in the plan. Refresh the complete
open-PR inventory before publication; preserve concurrent changes.

## Parallelism

Sequential.

## Inputs

The paired requirement/design, current main Plan header and panel wiring,
shared `file-download.ts`, mobile language and control sizing references,
existing Plan E2E fixtures, and `docs/public/README.md`.

## Results

ROOT reviewed the package and separately released implementation and HEAVY.
The mounted header RED failed on the missing control; the final targeted
Vitest run passed 24 tests across four files. The original null-Plan Implement
fixtures remain intact. Targeted ESLint, `i18n:check`, `i18n:ratchet`, the
8 GiB typecheck, specification catalog/lint, and public-doc validators passed.
Same-worktree `build:e2e`, backend `build build-e2e-delivery-fixture`, and
`e2e-plugin-ui e2e-plugin-package` completed before `--no-build`.

Chromium passed three tests, including narrow fine-pointer geometry and the
180px desktop panel; mobile-chrome passed two, including a narrow coarse-pointer
workbench and touch scrolling. Each run
used one worker and zero retries, with exact file contents and filename checked
on desktop and phone. The first 900px coarse tablet attempt could not find a
Plan panel in the existing tablet layout, so the coarse workbench test uses
1280px. The final desktop and phone screenshots were inspected and preserved
outside `.pr-assets` with their manifests and SHA-256 hashes under
`/tmp/kandev-plan-download-captures-042/desktop2` and `phone4`.

Command logs are under `/tmp/kandev-plan-download-check-*`,
`/tmp/kandev-plan-download-e2e-desktop2-IRvtGy`, and
`/tmp/kandev-plan-download-e2e-phone4-qeDGh0`; the initial RED is under
`/tmp/kandev-plan-download-red-nNx4cd`. No live developer instance was used.

The current-head review identified action clipping at a user-narrowed desktop
panel. An independently authored 180px real-panel regression first failed on
the published build: the overflowed action row did not respond to horizontal
wheel input (`/tmp/kandev-plan-download-narrow-red2-hmdw3u`). The Plan header
now shrinks and scrolls its own action row; no shared primitive changed. The
focused regression verifies wheel scrolling, keyboard traversal and hit targets,
menu/history activation and exact download bytes. A coarse-pointer regression
verifies 44px targets, touch scrolling, menu/history access and download.
The final desktop and phone runs are in
`/tmp/kandev-plan-download-fix-desktop-aQwTZk` and
`/tmp/kandev-plan-download-fix-mobile-final-xJkTRV`; fresh captures, manifests,
and hashes are retained under `/tmp/kandev-plan-download-fix-captures-042`.
