---
created: 2026-10-20
status: implemented
requirements:
  - REQ-TASKS-TURN-CHANGES-001
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-007
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Small turn-change fixes

## Scope

Change new-account seeds and backend/frontend fallback defaults to false. Repair
the total-index count rejection and hide settled zero-entry chat cards.
Preserve existing saved true/false values. The user cancelled all reset/migration
work and requested small isolated fixes. No new migrations, flags, or live-instance changes.

## Approach

Change `newSidebarPresentationJSON`, backend `scanUserSettings` defaults, and
frontend `createDefaultUserSettings`. Keep the existing upgrade backfill and all
settings-save paths unchanged. Existing users may therefore remain enabled.

Remove the 20,000-entry guard from full-index flag/staged-entry parsing only;
keep existing output/deadline and changed-file bounds. Test real 30,001-file
capture/compare/export with zero and one file changes and unchanged index/HEAD.
Chat uses a separate eligibility helper at all three filters; settled zero-file
summaries disappear while pending preparation and explicit history stay available.

## ASCII UI preview

UI-01: Existing General chat preference for a new account, desktop and phone:

```text
Show changed files after each turn    [OFF]
```

UI-02, shared desktop/phone chat after a settled zero-entry capture:

```text
Agent reply
Next conversation item
```

Use existing Save/Discard and mobile composition. No new markup is introduced.
No layout, touch, scroll, or navigation changes. Existing settings component
regressions plus hydration/default tests cover this state-only adjustment.

## Work orders

- [x] [Task 01: Small turn-change fixes](task-01-default-off.md)

## Verification results

Red confirmed for new-account/missing-value defaults, empty-card eligibility,
and real large-index capture (unsafe_git_state/output limit exceeded).
Green: backend user store/service/DTO packages passed; checkpoint/change tests
passed, including real 30,001-file capture/compare/export with zero and one-file
changes and unchanged real index/HEAD. Six frontend suites passed (137 tests).
Typecheck passed with NODE_OPTIONS=--max-old-space-size=8192 after the default
4 GiB heap was insufficient. Changed Go lint: zero issues. Frontend eslint passed;
changed files were formatted with Prettier. Public-doc validators, spec/catalog
checks, documentation coverage preflight and git diff whitespace passed.
PostgreSQL-specific cases were not exercised without a DSN. Browser E2E: six desktop Chromium cases and five mobile-chrome cases passed,
including saved opt-in from off, existing historical diff/file-list retry flows,
and hiding an unavailable zero-entry card after reload. Desktop/mobile PR assets
were captured in the isolated runner, inspected, and compressed. Desktop built
fresh artifacts; mobile reused those unchanged artifacts with --no-build. Live data was not changed. No reset, new migration,
flag, or deployment. Existing saved preferences and upgrade backfill remain unchanged.

Review remediation: full-index flag and staged-entry scans now check the capture
context between entries. The deterministic cancellation regression cancels after
the first entry and verifies both scans stop; normal-context positive controls
remain successful. Focused checkpoint/change tests passed again. No rendered UI
changed, so the verified desktop/mobile captures remain applicable.

Capture-dependent desktop/mobile scroll fixtures now explicitly opt in and restore
the saved preference. Their additional managed E2E runs passed (one case per
viewport), bringing browser coverage to eleven passing cases. No product code
or rendered composition changed in this follow-up.

Windows CI remediation: hashing the full 30,001-file checkout exceeded the
unchanged capture deadline on the hosted Windows runner. Both full-index scans
now have portable 30,002-record regressions that verify processing of the final
record; Windows uses those plus the existing small-repository capture tests.
The full-checkout capture/compare/export integration remains on Linux and is
skipped on Windows. A sparse fixture was investigated and discarded because it
also timed out. The focused checkpoint/change suite passed locally again (7.3s).
No production timeout, capture budget, or UI behavior changed in this follow-up.
Remote CI and merge remain pending.

Further CI remediation: the mobile delayed-card reload case reproduced against
GitHub's exact merge commit. Native restoration moved the viewport upward by
88px while the delayed card grew by 88px; the follow predicate incorrectly
released bottom ownership without reader input. A regression using the captured
geometry failed before the bounded absolute-delta correction and passed after it.
Existing explicit-position and reader-input guards remain covered. Four focused
frontend suites passed (151 tests), and the exact mobile browser regression
passed three repetitions after rebuilding the CI merge checkout. Eleven desktop
browser cases passed without retries, including manual reading, disabled/enabled
auto-scroll, cancellation, Quick Chat recovery, and delayed-card reload.

The interrupted-prompt test fixture now uses the existing 5-second SQLite busy
wait convention. A controlled concurrent writer reproduced the CI lock error
before the change and passed afterward. This changes only test setup. Frontend
lint, formatting, and typechecking passed. No public/spec contract changed:
AC-TASKS-TURN-CHANGES-005.9 already requires delayed cards to follow while preserving
manual reading. Temporary geometry tracing and retry overrides were removed.
Fresh remote checks and merge remain pending after the follow-up push.

Nine further mobile browser cases passed without retries, covering manual reading,
auto-scroll transitions, cancellation, and Quick Chat recovery. These reused the
unchanged rebuilt artifacts from the passing mobile reload repetitions. No new
rendered composition was introduced; existing settings and empty-card PR captures
remain applicable.

Pagination follow-up: the native-restoration correction exposed a stale follow
flag when an older-page request began without a wheel gesture. Existing desktop
pagination anchor cases reproduced the CI failures (two failures, no retries).
Older-page preparation now cancels motion/captures the prepend baseline and then
resynchronizes bottom ownership from the actual viewport before fetching data.
The same two desktop cases passed afterward without changing their assertions.
Full browser verification passed 16 desktop and 16 mobile cases without retries,
covering all pagination cases, deep/short/tool-only history, manual reading,
auto-scroll transitions, and delayed changed-file completion/reload together.
Four frontend suites (151 tests), lint, formatting, and typecheck passed again.
This preserves AC-TASKS-TURN-CHANGES-005.9; no new public behavior or composition is
introduced. Fresh remote checks remain pending after the remediation push.
