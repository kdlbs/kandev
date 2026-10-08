---
id: "02-default-recovery-proof"
title: "Prove default recovery and update guidance"
status: pending
wave: 2
depends_on: ["01-retire-continuation-toggle"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-001
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-002
  - REQ-PLATFORM-INTERRUPTION-CONTINUATION-003
acceptance_criteria:
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.4
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-001.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.5
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-002.6
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.1
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.2
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.3
  - AC-PLATFORM-INTERRUPTION-CONTINUATION-003.4
system_design:
  - ../../specs/platform/system-design/provider-interruption-continuation.md
---

# Task 02: Prove default recovery and update guidance

## Summary

Run the existing recovery workflows without an enabling flag. Prove desktop and
phone behavior, former false values, and settings omission, then update public
guidance to describe ordinary supported recovery and its manual boundaries.

## In scope

- Use `/e2e` and `/mobile-parity`. Remove the helper's `enabled` option and forced
  true env assignment. Existing scenarios must run with ordinary fixture defaults.
  Remove the extra enabling value from the backend-restart scenario too.
- Replace the desktop disabled case with `desktop: retired continuation setting
  cannot disable recovery`, using the exact former environment variable set false.
  Assert normal recovery, native trace identity, registry omission, and absence of
  the feature-state field. Task 01 separately proves persisted false overrides.
- Preserve all desktop completed-tool, retained-runtime, hidden-message,
  budget/refusal, queued-human priority, cancellation, reload, and restart cases.
- Add `phone: completed tools continue without opt-in` using the existing successful
  foreground shell/tool scenario. Keep the phone Cancel/reload case. Assert no
  automatic user `continue` row or bubble, the same conversation ID, reachable
  44px Cancel, and no document horizontal overflow.
- Verify the Feature Toggles surface omits the retired row on desktop and phone
  using existing settings navigation. Capture focused screenshots of ordinary
  recovery in the isolated test runtime.
- Use `/docs-maintainer` for the three public pages named in the plan. Keep old
  public section anchors working while removing opt-in instructions and updating
  feature status. Preserve uncertainty and same-conversation explanations.
- Record exact results in both work orders and the manifest. Leave unrelated
  initial-task-brief documents and tests untouched.

## Out of scope

No live-instance restart, provider credential changes, new native protocol claims,
layout redesign, translations without a rendered-copy change, website publication,
historical recovery replay, or widening eligibility to pending/cancelled tools.

## Acceptance

- Desktop and phone pass default completed-work continuation and control/history
  checks without a true flag value; the former false environment value is inert.
- Settings and feature responses omit the retired identity while uncertain work,
  human priority, cancellation, and restart still enforce existing behavior.
- Public documentation describes the implemented default; all task commands have
  recorded results and no old package result is counted as new verification.

## ASCII UI preview

UI-02 from [the package preview](plan.md#ascii-ui-preview), `001.6` and `003.1`-`.4`:

```text
Desktop Chat
  Completed history
  Cursor: Attempt 1/5. Continuing in 5s.   [Cancel]
  Agent output

Phone Chat
  Completed history
  Cursor: Attempt 1/5
  Continuing in 5s.
  [ Cancel: at least 44px ]
  Agent output
  Existing composer
```

Reuse `TransientRetryNotice` and the existing full-height phone Chat. The
transcript owns scrolling; the composer retains its safe-area behavior. Technical
details wrap. The continuation instruction stays absent from user history and
status changes preserve focus. UI-01 in the plan additionally requires omission
of the old settings row. The preview adds no new product strings or controls.

## Verification

Run from repository root after Task 01. If the worktree is fresh, install from
`apps` before the first pnpm command. Run managed E2E commands sequentially; their
runner rebuilds the web/backend product through the existing fixture.

```bash
(cd apps/web && pnpm exec vitest run lib/state/slices/features/features-contract.test.ts components/task/chat/messages/interruption-recovery-feedback.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/provider-interruption-continuation.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-provider-interruption-continuation.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 e2e/helpers/provider-interruption-continuation.ts e2e/tests/session/provider-interruption-continuation.spec.ts e2e/tests/session/mobile-provider-interruption-continuation.spec.ts)
(cd apps/web && pnpm run e2e:sleep-ratchet)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node -e 'const fs = require("node:fs"); const cp = require("node:child_process"); const v = require("./.github/scripts/pr-docs.cjs"); const changedFiles = [...new Set([...cp.execFileSync("git", ["diff", "--name-only", "-z", "HEAD"], {encoding: "utf8"}).split("\0"), ...cp.execFileSync("git", ["ls-files", "--others", "--exclude-standard", "-z"], {encoding: "utf8"}).split("\0")].filter(Boolean))]; const docs = [...new Set([...changedFiles.filter(p => p.endsWith(".md")), "docs/specs/platform/system-design/transient-turn-runtime-continuity.md"])]; const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, "utf8")])); const r = v.validateCoverage({changedFiles, fileContents}); console.log(JSON.stringify({ok: r.ok, status: r.status, errors: r.errors}, null, 2)); process.exitCode = r.ok ? 0 : 1;'
git diff --check
git status --short -- docs/plans/provider-interruption-continuation-graduation
```

The Node command runs `.github/scripts/pr-docs.cjs:validateCoverage` against
actual changed/untracked paths and linked requirement/design/work-order contents.
Include further linked documents if implementation expands the package. Report
any unavailable rendered check with its exact blocker.

## Files likely touched

- `apps/web/e2e/helpers/provider-interruption-continuation.ts`
- `apps/web/e2e/tests/session/provider-interruption-continuation.spec.ts`
- `apps/web/e2e/tests/session/mobile-provider-interruption-continuation.spec.ts`
- `docs/public/sessions-and-review.md`, `configuration.md`, `feature-status.md`
- This manifest and its two work orders for execution results

## Dependencies

Task 01 must pass its backend/config/feature contract checks. No parallel
implementation: the E2E helper and startup behavior must describe the same policy.

## Risks

The fixture currently forces the toggle true, which would hide an incomplete
removal. A successful mock trace establishes host behavior, not new live-provider
compatibility. Preserve public links and historical failure rendering.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/platform/requirements/provider-interruption-continuation.md)
- [Design](../../specs/platform/system-design/provider-interruption-continuation.md)
- Existing default fixture, native trace assertions, and desktop/phone continuation specs
- [Public documentation guide](../../public/README.md)

## Results

Pending.
