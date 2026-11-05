---
id: "01-copy-commit-identifier"
title: "Copy the current commit identifier"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-COPY-COMMIT-ID-001
acceptance_criteria:
  - AC-UI-COPY-COMMIT-ID-001.1
  - AC-UI-COPY-COMMIT-ID-001.2
  - AC-UI-COPY-COMMIT-ID-001.3
  - AC-UI-COPY-COMMIT-ID-001.4
  - AC-UI-COPY-COMMIT-ID-001.5
system_design:
  - ../../specs/ui/system-design/copy-full-commit-identifier.md
---

# Task 01: Copy the current commit identifier

## Summary

Add an accessible source-neutral copy action to the existing loaded commit-detail header, using the full current identifier and shared success feedback. Verify actual consumer and browser behavior, then add a concise public how-to.

## In scope

- Small local button, minimal header integration, existing localized labels and clipboard hook.
- Extend the existing mounted consumer suite while preserving its regression cases, fixtures, assertions and test meaning; new copy cases may stub unrelated dependencies but must not rewrite existing assertions or fixtures to make them pass. Run RED/GREEN, focused isolated desktop and mobile browser coverage, synthetic screenshots and public docs.
- Preserve existing header, source identity, file index, diff, navigation and feedback semantics.

## Out of scope

- Rows, menus, history, index actions, backend, Git mutations, metadata surfaces, and changed loading/error/header visibility.
- Shared clipboard owner refactoring, feature flags, live developer data, broad suites and delegation.

## Acceptance

- Local and GitHub loaded details copy the exact full current identifier; target changes never copy a stale value or present old feedback as a new success.
- Actual helper success gives accessible localized feedback; failure creates no success claim, preserves prior feedback timing and remains retryable.
- Visible keyboard and touch action measures 28px desktop and at least 44px square on phone/coarse pointer, including narrow fine-pointer phone; it fits without overflow and surrounding detail behavior remains intact.

## ASCII UI preview

UI-01 and UI-02 excerpts from the [full preview](plan.md#ascii-ui-preview), covering AC-UI-COPY-COMMIT-ID-001.1 through .5:

```text
Desktop: Author · time · a1b2c3d [copy]   Files (2) / existing diffs
Phone:   Author · time
         a1b2c3d [copy]                   Files (2) / existing diffs
```

Action is visible, keyboard operable and touch-sized when required; success is accessible and failure remains retryable. Existing detail sheet remains the single scroll owner.

## Verification

Only after ROOT fully reads the four design artifacts and sends a later separate execution and exclusive HEAVY release, run from repository root. Use the specified toolchain and install exactly once before pnpm commands or hooks. Each `run-quiet` call below has its own external log directory; retain every native session handle and actually join it to exit before reporting. Save full logs and results. Run typecheck alone with an 8 GiB limit. Do not rerun a command to infer a lost handle's result.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:$PATH"
export GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2
export COPY_ID_EVIDENCE=/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/copy-commit-id
mkdir -p "$COPY_ID_EVIDENCE"
(cd apps && pnpm install --frozen-lockfile)
```

Write the new mounted copy cases in the existing suite first, then run this command before any production change. Record the failure attributable to the missing copy control. Implement only after the causal RED.

```bash
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/red" scripts/run-quiet copy-id-red -- pnpm --dir apps/web exec vitest run components/task/commit-detail-content.test.tsx
```

After implementation, run the following individually and record actual outcomes. Adapt the explicit component filename below if implementation chooses another small component, recording the exact resulting command.

```bash
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/green" scripts/run-quiet copy-id-green -- pnpm --dir apps/web exec vitest run components/task/commit-detail-content.test.tsx
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/eslint" scripts/run-quiet copy-id-eslint -- pnpm --dir apps/web exec eslint components/task/commit-detail-content.tsx components/task/commit-copy-identifier.tsx components/task/commit-detail-content.test.tsx
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/typecheck" scripts/run-quiet copy-id-typecheck -- env NODE_OPTIONS=--max-old-space-size=8192 pnpm --dir apps/web run typecheck
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/i18n-check" scripts/run-quiet copy-id-i18n -- pnpm --dir apps/web run i18n:check
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/i18n-ratchet" scripts/run-quiet copy-id-ratchet -- pnpm --dir apps/web run i18n:ratchet
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Build the matching artifacts in this worktree before `--no-build` E2E; retain build receipts. Run each command to completion sequentially.

```bash
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/build-web" scripts/run-quiet copy-id-web -- make build-web-e2e
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/build-backend" scripts/run-quiet copy-id-backend -- make build-backend
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/build-fixture" scripts/run-quiet copy-id-fixture -- make build-e2e-delivery-fixture
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/build-plugin" scripts/run-quiet copy-id-plugin -- make build-e2e-plugin-package
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/desktop" CAPTURE_PR_ASSETS=true scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium -- tests/git/commit-copy-identifier.spec.ts --workers=1 --retries=0
```

Before the next managed run, copy the owned synthetic `.pr-assets` originals and manifest to a new external evidence directory, hash the originals and manifest, and validate them. Then run and preserve the phone capture likewise. Never embed local paths in the PR.

```bash
mkdir -p "$COPY_ID_EVIDENCE/desktop-assets"
cp -a apps/web/.pr-assets/. "$COPY_ID_EVIDENCE/desktop-assets/"
(cd "$COPY_ID_EVIDENCE/desktop-assets" && find . -type f -print0 | sort -z | xargs -0 sha256sum > ../desktop-assets.sha256)
KANDEV_RUN_QUIET_DIR="$COPY_ID_EVIDENCE/mobile" CAPTURE_PR_ASSETS=true scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome -- tests/git/mobile-commit-copy-identifier.spec.ts --workers=1 --retries=0
mkdir -p "$COPY_ID_EVIDENCE/mobile-assets"
cp -a apps/web/.pr-assets/. "$COPY_ID_EVIDENCE/mobile-assets/"
(cd "$COPY_ID_EVIDENCE/mobile-assets" && find . -type f -print0 | sort -z | xargs -0 sha256sum > ../mobile-assets.sha256)
```

Before publication refresh the complete open-PR file inventory, paginate capped lists and reconcile actual overlaps with all owned files. Preserve current public-doc content. After checks, use normal active commit and commit-msg hooks with complete logs, no bypass or amend. Follow the full `/pr` workflow, including the already authorized PR-scoped screenshot upload/comment step: validate owned synthetic assets, manifest, CLI attach support and write access, upload once, read back HTTPS URLs, merge only the owned screenshot section into a fresh live body and verify the live body. Do not blindly retry an uncertain upload. One ordinary push and ready PR; verify clean local head equals actual `git ls-remote` and PR frozen head. No merge, queue, admin, deletion or observer before truthful HEAVY return. ROOT owns integration and later review.

## Files likely touched

- `apps/web/components/task/commit-detail-content.tsx`
- `apps/web/components/task/commit-copy-identifier.tsx` (new)
- `apps/web/components/task/commit-detail-content.test.tsx` (existing suite)
- `apps/web/e2e/tests/git/commit-copy-identifier.spec.ts` (new)
- `apps/web/e2e/tests/git/mobile-commit-copy-identifier.spec.ts` (new)
- `docs/public/sessions-and-review.md`
- This plan, work order and paired specs for lifecycle/results updates.

## Dependencies

None. Await the separate execution and HEAVY release before starting.

## Risks

Asynchronous feedback and changes in target identity can misattribute success; assert the rendered consumer behavior. Actual browser geometry and clipboard permissions must be tested rather than inferred from classes. Capture preservation must precede the next managed run.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/copy-full-commit-identifier.md)
- [System design](../../specs/ui/system-design/copy-full-commit-identifier.md)
- Existing commit detail, clipboard feedback, commit navigation fixtures, `/tdd`, `/mobile-parity`, `/e2e`, `/docs-maintainer` and `/pr`.

## Results

Completed implementation on 2026-10-28. All listed native handles were actually joined to their terminal exit. Full run-quiet logs are under `/home/jcfs/.kandev/loop-evidence/61dadc35-4a42-4581-8c94-62309c160fd9/copy-commit-id/`.

- Frozen pnpm 9.15.9 installation: session 31356, exit 0. Mounted RED: 96769, exit 1, absent accessible copy button in all three new cases; baseline tests and fixtures retained.
- Pre-change same-worktree builds: web 72438, backend 9193, delivery fixture 95554 and plugin 55212, all exit 0. Desktop E2E RED 16270 and phone RED 27360 exited 1 on the absent copy action after loading real detail.
- Mounted GREEN: 36913 and post-format 78605, exit 0; final suite 13 tests. Touched-file ESLint 42964, 8 GiB serialized typecheck 14726, i18n check 25441 and ratchet 97775, all exit 0.
- Final web build 28404 exit 0; unchanged same-worktree backend and fixture build receipts above were reused. Focused `--host --no-build --workers=1 --retries=0` desktop 57506 passed 2 cases and mobile-chrome 39955 passed 1. Both used actual clipboard reads and synthetic commit details. Original captures, manifests and SHA-256 receipts are in `desktop-assets`, `mobile-assets`, `desktop-assets.sha256`, and `mobile-assets.sha256` under the evidence directory.
- `node --test scripts/validate-public-docs.test.mjs`: 63 passed; `node scripts/validate-public-docs.mjs`: 54 pages validated. Initial invocations failed with `node` missing from the shell PATH (exit 127); the explicit pinned Node 24.21.0 binary succeeded. `python3 scripts/list-docs.py validate`: 414 decisions and 1814 specifications; `python3 scripts/lint-spec-files.py --all` and `git diff --check` passed.
- Shared clipboard behavior and existing header visibility were retained. No backend, Git write, new locale, or shared-helper changes were needed.
- Normal commit attempt 4142 exited 1 because the web-lint hook treated a duplicated test label as a warning; no bypass was used. After replacing the repeated literal with a local test constant, mounted suite 24975 and `eslint --max-warnings 0` 15290 both passed and were actually joined. The failed hook log is `commit-hooks.log` in the evidence directory.
