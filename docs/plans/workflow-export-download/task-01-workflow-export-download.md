---
id: "01-workflow-export-download"
title: "Download the displayed workflow export"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-WORKFLOW-EXPORT-DOWNLOAD-001
acceptance_criteria:
  - AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.1
  - AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.2
  - AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.3
  - AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.4
  - AC-UI-WORKFLOW-EXPORT-DOWNLOAD-001.5
system_design:
  - ../../specs/ui/system-design/workflow-export-download.md
---

# Task 01: Download the Displayed Workflow Export

## Summary and scope

Add one visible localized action in the existing shared dialog, prove current
raw text and actual file delivery on desktop and phone, and update only the
short public UI instructions. Follow the [requirements](../../specs/ui/requirements/workflow-export-download.md),
[design](../../specs/ui/system-design/workflow-export-download.md), and
[plan](plan.md). Preserve existing clipboard tests and fixture meanings.

## Exclusions

No export fetch, schema, parser, reformat, BOM, dynamic naming, workflow/save,
import, selection, profile, backend, Git, session, helper or hook refactor.
Preserve existing modal, navigation, scroll owners and Copy/Close semantics.
No live developer data, broad suites, delegates or extra sessions.

## Acceptance

1. Mounted consumer uses the latest exact prop, fixed filename and unchanged
   helper; empty/whitespace disables, real synchronous failure triggers a
   localized default toast and permits retry, and prior clipboard/Close behavior
   remains intact.
2. Dedicated isolated desktop and configured mobile-chrome tests exercise real
   single and All entry points, exact downloaded bytes/name, keyboard/touch,
   responsive target/containment including narrow fine-pointer, and synthetic
   screenshots; preserve assets, manifest and hashes externally.
3. Only the specified public guide section changes; targeted checks pass and
   actual results and lifecycle statuses are recorded.

## ASCII UI preview

**UI-01: Workflow export dialog**, excerpt of the [plan](plan.md#ascii-ui-preview),
criteria `.1`-`.5`:

```text
Desktop: | current YAML preview (scrolls)              |
         |                    [Close] [Download] [Copy] |
Phone:   | current YAML preview (scrolls) |
         | [Copy]                         |
         | [Download]                     |
         | [Close]                        |
```

Phone may wrap instead; all actions remain visible and contained, Download is
at least 44 px on phone/coarse including narrow fine pointer. Empty/whitespace
is disabled; a failed initiation keeps the dialog usable. Spacing is illustrative.

## Execution gate and TDD

ROOT must fully read all four design documents and send a later separate
execution request and exclusive global HEAVY release. This initial turn does not
authorize production, permanent tests, install, checks, builds, hooks or delivery.
Use the same primary and preserve concurrent edits. Re-read scoped guidance,
current code, tests, the public guide, and the E2E/PR skills after release.
First extend the existing mounted dialog suite with an independently meaningful
test for the missing Download action and run it to record causal RED before
production edits. Then implement; prove exact Unicode/newlines and latest text
after rerender, helper filename and nonbinary mode, disabled states, actual
failure/default toast/retry, and unchanged Copy/Close. Keep existing clipboard
fixtures and assertions intact. Capture actual download events and file bytes;
compare to the displayed current export, not a second fetch or hardcoded
serialization. Use isolated synthetic fixture data and both entrypoints.

## Verification

Only after both gates, serialize from repo root. Retain and actually join every
native session handle, with complete logs; no duplicate inference. Use one
unique external `KANDEV_RUN_QUIET_DIR` per invocation/rerun. `--summary` is
only for E2E/Playwright. Install pinned pnpm once before pnpm or hooks; do not
share dependencies or builds. Respect `/home/jcfs/a.txt` exclusions, and do
not scan, kill, prune, or audit unowned processes, caches, temp, volumes, closed,
canceled or paused tasks, historical fixtures, or protected resources.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
runq() { local tag="$1"; shift; KANDEV_RUN_QUIET_DIR="$(mktemp -d /tmp/kandev-workflow-download-quiet-XXXXXX)" scripts/run-quiet "$tag" "$@"; }
runq install -- pnpm --dir apps install --frozen-lockfile
runq vitest-red -- pnpm --dir apps/web exec vitest run components/settings/workflow-export-dialog.test.tsx
# Record missing-control RED before implementing.
runq vitest -- pnpm --dir apps/web exec vitest run components/settings/workflow-export-dialog.test.tsx
runq eslint -- pnpm --dir apps/web exec eslint --max-warnings 0 components/settings/workflow-export-dialog.tsx components/settings/workflow-export-dialog.test.tsx e2e/tests/workflow/workflow-export-download.spec.ts e2e/tests/workflow/mobile-workflow-export-download.spec.ts
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
runq e2e-list-desktop -- pnpm --dir apps/web exec playwright test --config e2e/playwright.config.ts --project chromium --list e2e/tests/workflow/workflow-export-download.spec.ts
runq e2e-list-mobile -- pnpm --dir apps/web exec playwright test --config e2e/playwright.config.ts --project mobile-chrome --list e2e/tests/workflow/mobile-workflow-export-download.spec.ts
CAPTURE_PR_ASSETS=true runq e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project chromium tests/workflow/workflow-export-download.spec.ts -- --workers=1 --retries=0
# Preserve owned originals, manifest and hashes externally before next run clears .pr-assets.
CAPTURE_PR_ASSETS=true runq e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/workflow/mobile-workflow-export-download.spec.ts -- --workers=1 --retries=0
# Preserve mobile assets, manifest and hashes; inspect screenshots and geometry.
git diff --check
git status --short
```

Add any optional component/helper paths actually changed to targeted lint.
Inspect describe overrides and actual test discovery for zero retries. Verify
same-worktree build receipts and runner cleanup before `--no-build`; no freshness
bypass. Keep 8 GiB typecheck and all checks/builds serial. Record exits, logs,
visual and geometry evidence. After passing, promote paired draft specs and
mark work order done and plan implemented only when requirements match results.

## Files and dependencies

- `apps/web/components/settings/workflow-export-dialog.tsx` and existing
  `workflow-export-dialog.test.tsx`; optional small local button component.
- `apps/web/e2e/tests/workflow/workflow-export-download.spec.ts` and
  `mobile-workflow-export-download.spec.ts`; optional narrowly scoped test helper.
- `docs/public/workflow-import-export.md` Quick path/Use the UI only.
- This plan, work order and paired UI specifications.

No work-order dependencies. ROOT's later release is a gate. Current main and
complete open-PR inventory must be refreshed before publication for all actual
owned paths; reconcile actual overlaps and preserve concurrent edits. Guide-only
PRs 4922, 4714 and 3440 had disjoint fully read patches at qualifying heads.

## Delivery and observation after verification

Use active precommit and commit-message hooks with full logs, no bypass or amend;
ordinary push and ready PR. Invoke full `/pr`, including the authorized PR-scoped
screenshot step: validate owned synthetic assets/manifest/hashes, support and
write access, upload once, verify HTTPS readback, merge only owned screenshots
into a fresh live body preserving other blocks, and verify live body. No blind
retry after an uncertain response, media branch or permission ceremony. Freeze
published head except actual correctness, security or required CI findings.
Return exclusive HEAVY only when every owned local handle is actually joined.
Then start one original 90-minute all-terminal `scripts/pr-await` with GNU
91-minute timeout, 10-second kill grace and 60-second cadence in this primary;
record UTC start/deadline, native handle, wrapper PID, progress, result and
actual terminal join. No automatic retry, renewal, reset, new observer, new
FULL, head push or merge. Six mandatory contexts and actual current Backend,
Frontend and E2E parents must succeed; optional walkthrough failure alone may
merge. ROOT owns the static gate, later separate expected-head squash, proof,
count, full plan save and one deletion retaining the original workspace/deps.
Do not archive or delete this task; ROOT reads the plan/conversation directly.

## Results

ROOT reviewed the four documents and separately released implementation and
exclusive HEAVY. The mounted test first failed on the absent Download control
(`/tmp/kandev-workflow-download-red-czAzs0/kandev-run.vitest-red.txsGiDYT.log`).
The final focused Vitest run passed 10 tests, preserving the existing clipboard
feedback and fallback assertions; targeted ESLint, i18n checks and ratchet,
8 GiB typecheck, specification catalog/lint and public documentation validators
passed. Same-worktree web, backend, delivery fixture and plugin builds passed.

Desktop Chromium passed two tests and mobile-chrome passed one, with one worker
and zero retries. Both single and Export All paths delivered actual UTF-8 file
bytes equal to the displayed YAML and the fixed name. Keyboard, phone touch,
narrow fine-pointer and wide coarse-pointer geometry, containment and hit tests
passed. Synthetic screenshots were visually inspected; originals, manifests and
SHA-256 hashes are retained under `/tmp/kandev-workflow-download-captures-046/`
(`desktop-final` and `mobile-final`). Final browser logs are
`/tmp/kandev-workflow-download-desktop-final-REheZw/kandev-run.e2e.WnGF3H17.log`
and `/tmp/kandev-workflow-download-mobile-final-wKBvIk/kandev-run.e2e.uVZ61uzj.log`.
The initial phone and desktop captures are retained separately. No live developer
data was used.

A later short-phone review was reproduced on the unchanged published build:
mobile-chrome at 375x568 placed Close's bottom at 576px for both Single and
Export All. The causal RED and measurements are retained at
`/tmp/kandev-workflow-short-568-8CrXt9/kandev-run.e2e.oBHSsPl4.log`.
The local dialog now caps its height to the dynamic viewport and lets the YAML
preview shrink and scroll internally. The focused mounted suite, targeted
ESLint, i18n checks, 8 GiB typecheck, spec and public docs validators passed.
A fresh same-worktree web build passed; unchanged backend and fixture builds
were reused. Desktop Chromium passed three tests and mobile-chrome one test
with one worker and zero retries, covering 375x600 and 375x568 on both pointer
modes and both entry points, actual download bytes, hit testing, containment,
and preview scrolling. Logs are
`/tmp/kandev-workflow-short-fix-desktop-K9SKUf/kandev-run.e2e.ZhyqaeMo.log`
and `/tmp/kandev-workflow-short-fix-mobile-gWgiEa/kandev-run.e2e.G0wG7K9C.log`.
Fresh synthetic originals, manifests, and hashes are retained at
`/tmp/kandev-workflow-short-fix-captures-046/`; the short-phone view was
inspected and shows the complete dialog and footer within the viewport.
