---
id: "01-native-test-delivery"
title: "Route settings Test through bounded native delivery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
acceptance_criteria:
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.8
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.15
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.16
  - AC-DESKTOP-DESKTOP-TAURI-APP-001.17
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
---

# Task 01: Route settings Test through bounded native delivery

## Summary

Make the existing Test button use native notification permission and delivery
in Tauri, preserving browser-only behavior. Admit one bounded app-scoped native
diagnostic identity and prove the correction through the real frontend consumer
chain and real Rust validator.

## Release gate

ROOT reviewed the corrected four-artifact package and later released
implementation to the same primary session. Task 01 was executed sequentially
under the sole local-heavy lease, without delegation or new sessions. Local
checks are complete; normal active-hook commit/push and hosted PR validation
remain delivery steps. Merge requires ROOT's separate later expected-head grant.

## In scope

- Independent rendered Test RED, real native transport fixture at OS invoke,
  browser-only controls and permission/result cases named in the plan.
- Independent Rust RED admitting `desktop.test_notification:<occurrence>`;
  retain origin checks, existing bounds, native OS permission, dedup and semantic
  validation. Require no task/session and a non-empty suffix for the new kind.
- Cohesive settings action helper using existing client and `generateUUID`,
  with the old hook delegating; no action-file growth past limits.
- Minimal public desktop Test paragraph after code change; synchronize this
  package's final command results/statuses and owning pair.
- Later ROOT-authorized bounded correction: only rustls 0.23.45 to 0.23.46
  in desktop Cargo.lock, plus General to Preferences in the owned new guide
  paragraph. No manifest or unrelated dependency update.
- Correct only `settings:testNotificationBody` in all seven settings catalogs
  to describe device notification delivery. Keep the existing title and keys;
  derive zh-hk/zh-tw through existing zh-hant generation and assert both actual
  native and browser message bodies.

## Out of scope

Provider/save/subscription/backend events, Go, ordinary session/update policy,
auth/origin ownership, Tauri capabilities/commands, new translation keys,
unrelated copy/catalog edits, dependency or lock changes beyond the authorized
rustls patch, config/flags, layout/touch/navigation/scroll/breakpoint work,
service workers, recovery/races/performance, broad test suites, bundled desktop
rebuilds, native physical display claims, foreign resource cleanup.

## Acceptance

1. Actual rendered native-granted Test clicks reach the real client/invoke
   transport despite absent or denied browser Notification, use the translated
   device-neutral body "If you can read this, notifications are working on this
   device." and unchanged generic Test title on both native/browser targets,
   distinct `desktop.test_notification:` identities, and omit task/session.
   Browser-only controls retain delivery behavior without provider writes.
2. Actual Rust validation accepts only the dedicated bounded app-scoped test
   shape in addition to existing types. Unknown/retired/lookalike events remain
   rejected, session task requirements and update behavior remain unchanged,
   and permission-before-claim/dedup/failure-release controls pass.
3. Native permission, display results, and rejections follow the reviewed design:
   no background prompt, denied no repeated request, no browser coupling/fallback,
   no false permission error after display failure, no unhandled rejection or
   success assertion. Listed checks pass with original handles joined and limits
   reported. Final implementation statuses/results are accurate.

## ASCII UI preview

`UI-01: Settings notification permission controls`, existing Notifications page.
See [combined preview and mobile exception](plan.md#ascii-ui-preview).

```text
BEFORE DESKTOP                   AFTER DESKTOP
[Enabled] [Refresh] [Test bell]   [Enabled] [Refresh] [Test bell]
Test -> browser only            Test -> native command

PHONE BROWSER (unchanged)
[Enable/Enabled] [Refresh] [Test bell]
Test -> browser permission and notification
```

Control order/composition are preserved; labels are illustrative translations.
No geometry, touch, scroll, navigation, or responsive changes. Existing denied,
unsupported, and permission-error guidance stays under the same group. No new
delivery result UI. AC `.15` through `.17` use the rendered consumer tests;
mobile-parity's state/data exception applies.

## TDD sequence

After the later release, author
`apps/web/components/settings/notification-test-delivery.test.tsx` independently.
Use the actual section, permission hook, settings state/actions, native client,
and invoke transport. Mock only external provider/store IO and OS invoke. First
run the causal native-granted/browser-absent actual click alone and record zero
show commands as the expected RED. Do not copy ROOT's private candidate/source.
Use `HTMLButtonElement.disabled`, not unsupported JestDOM assertions.

Both the native OS-invoke fixture's actual `request.body` and the browser
Notification constructor's actual `body` must equal the device-neutral English
diagnostic. Assert the unchanged title too. Do not only compare either body to
`t()` or a mocked helper, which would allow the old false browser claim to pass.
In the native-only case, no browser capability or delivery was exercised.

Correct the existing body value in en, pt-pt, zh-cn, ja, and ko. For zh-hk and
zh-tw, snapshot the current owned settings catalogs immediately before running
the existing generator, retain its generated `testNotificationBody` value,
and preserve all other keys/values from those snapshots. Record generation
truth and review the resulting key-only diff; do not hand-translate the
Traditional pair or retain unrelated generated drift. Preserve others' edits.

Separately add `bridge_accepts_app_scoped_settings_test` to the real Rust tests
and run just that test as RED before changing validation. Then add the cases
mapped in the plan, implement the minimal correction, and run the complete
affected frontend and native suites as GREEN. Keep the default-feature Rust
predicate tests independent of the feature-gated unchanged OS command builder.
Do not count ROOT's reproduction or historical tests as these independent REDs.

## Verification

Commands below run only after release and sequentially from repo root. Existing
Node, pinned cached pnpm, and Rust paths avoid altering toolchain defaults:

```bash
export PATH="/home/jcfs/.nvm/versions/node/v24.18.0/bin:/home/jcfs/.rustup/toolchains/stable-x86_64-unknown-linux-gnu/bin:$PATH"
pnpm() { node /home/jcfs/.cache/node/corepack/v1/pnpm/9.15.9/bin/pnpm.cjs "$@"; }

# One normal frozen install only if this managed worktree still lacks deps.
if [ ! -d apps/node_modules ]; then
  (cd apps && pnpm install --frozen-lockfile)
fi

# Independent frontend RED before its implementation (expect exit 1 causally).
(cd apps/web && pnpm exec vitest run components/settings/notification-test-delivery.test.tsx --maxWorkers=1 -t 'native granted with browser unavailable')

# Independent Rust RED before its implementation (expect exit 101 causally).
(cd apps/desktop/src-tauri && CARGO_BUILD_JOBS=2 CARGO_PROFILE_DEV_DEBUG=0 CARGO_PROFILE_TEST_DEBUG=0 cargo test --locked --lib native_notifications::tests::bridge_accepts_app_scoped_settings_test -- --exact)

# After the source translations change and target snapshots are captured.
(cd apps/web && pnpm run i18n:zh-hant --namespace settings)
# Retain only the generated body-key change in each Traditional catalog.

# GREEN after correction; no broad suite or bundled desktop build.
(cd apps/web && pnpm exec vitest run components/settings/notification-test-delivery.test.tsx components/settings/notifications-settings-actions.test.tsx components/settings/notification-permission-section.test.tsx lib/desktop/native-notification-client.test.ts --maxWorkers=1)
(cd apps/desktop/src-tauri && CARGO_BUILD_JOBS=2 CARGO_PROFILE_DEV_DEBUG=0 CARGO_PROFILE_TEST_DEBUG=0 cargo test --locked --lib native_notifications::tests)
(cd apps/desktop/src-tauri && cargo fmt --all -- --check)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/settings/notification-test-action.ts components/settings/notification-test-delivery.test.tsx components/settings/notifications-settings-actions.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)

# Public doc content validation only when its paragraph changes.
node scripts/validate-public-docs.mjs

# Once after final implementation statuses, not repeated validator tests.
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Before commit, stage only actual owned paths through normal delivery workflow.
Run `(cd apps/web && pnpm run i18n:ratchet --base 73c5ddb8c9550744655432d03edbe5872b6bbb80)`
against ALL actual owned staged TS paths/new fixtures; inspect the staged path
list and the ratchet's added/modified selection. Test files are intentionally
excluded by the existing tool, not silently absent from staging. Include every
actual new production helper; do not claim coverage from an unstaged new file.
Run scope lint on any additional actual changed TS paths. Genuine private event
identifiers/diagnostics use explained line `i18n-exempt` markers if the existing
non-JSX scanner requires them. The one existing body value changes in seven
settings catalogs; no new keys or unrelated copy is expected.

Inspect ALL actual owned staged paths, including the seven locale JSON files.
The TS ratchet intentionally does not judge JSON values; the listed full
`i18n:check` gates their key/placeholder/translation integrity. Review a parsed
before/after comparison of each settings catalog: key sets must match and the
only changed value must be `testNotificationBody`. Native/browser rendered body
assertions prove that the corrected English value reaches both targets; review
the six translated values for the same device-neutral meaning. No locale edits
or generation occurs during this design phase.

Design preflight and final delivery must exercise the repository's exported
`validateCoverage` in `.github/scripts/pr-docs.cjs` with the actual changed work
order, sibling plan, owning requirement/design and a proposed/actual production
path so it does not pass through the docs-only exemption. Confirm `ok=true`,
`status=covered`, `errors=[]` and exactly this work order. No validator-test replay.

If a resource/tool/auth check fails, checkpoint ROOT with the original command,
handle and actual exit. Do not install toolchains, wipe caches, kill foreign
processes, or rerun passed checks without a new change/failure. Hosted current
product CI remains required; default-feature Rust/jsdom coverage does not claim
OS display. No gratuitous full-feature Cargo build is required because the
changed predicate is feature-independent and the command/origin/builder are unchanged.

## Files likely touched

- `apps/web/components/settings/notifications-settings-actions.ts`
- `apps/web/components/settings/notification-test-action.ts` (new cohesive action)
- `apps/web/components/settings/notification-test-delivery.test.tsx` (new independent suite)
- `apps/desktop/src-tauri/src/native_notifications.rs` (initial implementation and tests)
- `apps/desktop/src-tauri/Cargo.lock` (later rustls security patch only)
- `apps/web/src/locales/en/settings.json` (`testNotificationBody` only)
- `apps/web/src/locales/pt-pt/settings.json` (same key only)
- `apps/web/src/locales/zh-cn/settings.json` (same key only)
- `apps/web/src/locales/zh-hk/settings.json` (same key via generation)
- `apps/web/src/locales/zh-tw/settings.json` (same key via generation)
- `apps/web/src/locales/ja/settings.json` (same key only)
- `apps/web/src/locales/ko/settings.json` (same key only)
- `docs/public/desktop-app.md` (affected notification paragraph only)
- Existing owning desktop pair and this package's statuses/results.

Read-only dependencies: `notification-permission-section.tsx`,
`notifications-settings.tsx`, `native-notification-client.ts`,
`tauri-event-transport.ts`, `lib/uuid.ts`, and their existing tests. No client
contract change is currently necessary. Preserve all others' edits.

## Dependencies

None. One vertical work order; no other package or agent is required.

## Risks

Browser-coupled permission, fabricated semantic identity, static dedup identity,
and misclassified display errors are the regression risks. Physical OS display
is outside deterministic local evidence. The near-limit actions file and fresh
worktree dependency/resource budget constrain the implementation.

## Parallelism

`sequential`

## Inputs

- [Owning requirement](../../specs/desktop/requirements/desktop-tauri-app.md),
  `.8`, `.15` through `.17`.
- [Owning design](../../specs/desktop/system-design/desktop-tauri-app.md#settings-notification-test).
- ADR-0039 and semantic notification ADR linked in the manifest; adjacent
  platform notification requirement is a boundary reference, not a second owner.
- Existing native notification Rust tests and real frontend client/transport;
  existing settings-action tests illustrate provider/store fixtures but mock
  the client and therefore cannot provide the new integration regression.
- Qualified receipts and constraints in [the manifest](plan.md#evidence-and-root-cause).

## Results

Implemented on 2026-10-14 after ROOT's later explicit release. Production code
uses the reviewed native test kind and guards; the actual native/browser
message bodies use the corrected existing key. The section markup, geometry,
permission owner, native client/transport, provider IO, and OS command builder
were not replaced or expanded.

### Original-handle receipts

- Frozen `pnpm install --frozen-lockfile` from `apps/`: handle `75201`, actual
  join `200b5c`, exit 0; 937 reused, zero downloads, no lock/dependency change.
  An initial status-edit script used the wrong working directory; it changed
  nothing, and the status was corrected from repo root before implementation.
- Frontend setup-only no-file run: handle `72557`, actual join `0613b6`, exit 1,
  zero tests collected. Retained as setup failure, not RED evidence.
- Frontend setup-only fresh-IO-array render loop: handle `1823`, actual join
  `27b16c`, exit 1; two harness failures and native transport control passed.
  Stabilizing the external IO snapshot corrected the fixture, not production.
- Independent frontend RED: handle `23595`, actual join `b9a8bd`, exit 1;
  actual native-granted/browser-absent click produced zero show commands
  (93 ms), browser body was the old false browser claim (9 ms), real native
  client/transport positive control passed (1 ms). Two failures, one pass.
- Independent Rust RED: handle `39410`, actual join `3504e3`, exit 101;
  `bridge_accepts_app_scoped_settings_test` failed because the real validator
  returned unsupported-event. No production validator change preceded it.
- Affected four-suite GREEN attempt: handle `38330`, actual join `99b61c`,
  exit 1; 48 passed and one new unsupported-state test selector was incorrect
  (`not supported` versus existing `does not support` copy). No product
  failure or unhandled rejection. Corrected only that test selector.
- New rendered suite rerun: handle `36680`, actual join `fdb9b2`, exit 0,
  21 passed. The three unchanged affected suites already supplied 28 passes.
- First scope lint: handle `35252`, actual join `daee11`, exit 1; one test
  group was 122 lines, over the 100-line function limit. Split only that group.
- Final new-suite plus scope-lint command: handle `40702`, actual join
  `329fdd`, exit 0; 21 rendered integration tests passed and all three actual
  changed TS files linted with zero warnings. No passed unchanged-suite replay.
- Native GREEN and format: direct receipt `e90e22`, exit 0; 15 real native
  notification tests passed and `cargo fmt --all -- --check` passed, using
  locked default features, two jobs, dev/test debug 0. Existing default-feature
  dead-code warnings were retained; no bundled desktop rebuild.
- Final web typecheck: handle `10931`, actual join `ce78af`, exit 0;
  `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`, including rich
  output catalog check, once after the final TS edit.
- Full `pnpm run i18n:check`: handle `75498`, actual join `8c2763`, exit 0;
  seven complete catalogs, pseudo key sync, Trans/plural/module-scope/copy
  guards passed. Existing 507 orphan-key warnings were reported, not hidden.
- Traditional generation: direct `50f2be`, exit 0; scoped settings generator
  changed exactly one web key per target. Retained generated body values and
  preserved unrelated target values from immediate snapshots. Its backend
  outputs were byte-identical and created no backend change.
- Parsed seven-catalog diff: direct `5886ac`, exit 0; unchanged key sets and
  only `testNotificationBody` changed in each. Native/browser actual delivered
  English bodies and unchanged title passed in the real rendered suite.
- Public docs: `node scripts/validate-public-docs.mjs`, direct `6e847f`, exit 0,
  48 published pages. Only the affected how-to notification paragraph changed.
- Staged `pnpm run i18n:ratchet --base 73c5ddb8c9550744655432d03edbe5872b6bbb80`:
  handle `98244`, actual join `5abdb1`, exit 0; one added + one modified
  production TS file selected, guard allowlist 643 intact. All owned paths,
  including the new test fixture and seven locale JSONs, were actually staged.
  Test fixtures are intentionally tool-exempt; full i18n checks gate JSON.

Every original command handle above was joined to actual terminal exit before
its next heavy step. No process was replaced while its handle remained live.
ROOT's private candidate/source files were not read, copied, replayed, or deleted.
The initial delivery made no Go, dependency, lock, native origin/permission, command/capability, provider,
layout, or protected-resource changes were made.

Final catalog/specification/coverage/diff gates are recorded in the manifest.
Normal hook, commit/push, PR-head and hosted-CI receipts are tracked externally
because those operations happen after this source record is committed.
Physical OS display was not exercised; the real frontend tests prove delivery
up to OS invoke, and Rust tests prove the real predicate and dedup behavior.


### Later bounded correction release

ROOT authorized closing the actual baseline security findings reported by
Cargo Audit run 38585372942/job 115925834868 and the valid Greptile guide-path
thread PRRT_kwDOQ2-eWs6sYT2X/comment 4272517574. Current order is reopened for
this one batched correction. Prior passing receipts remain historical evidence
for unchanged notification code and locale values.

Verify the authoritative advisory database/registry metadata and inspect the
exact target update. Existing local cargo-audit is unavailable; do not install
it or claim a local pass. Current-head hosted Cargo Audit must show all four
RUSTSEC-2026-0346/0347/0348/0349 findings closed. No old-head audit retry.

After the approved lockfile-only patch, run exactly:

```bash
(cd apps/desktop/src-tauri && cargo update -p rustls@0.23.45 --precise 0.23.46)
(cd apps/desktop/src-tauri && CARGO_BUILD_JOBS=2 CARGO_PROFILE_DEV_DEBUG=0 CARGO_PROFILE_TEST_DEBUG=0 cargo test --locked --lib)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the existing Node 24 and stable Rust paths from the earlier command block;
retain original handles and actual exits. No unchanged frontend/Rust-native-only
suite/typecheck/i18n replay, no test mirroring a generated lockfile version.
Inspect actual-file documentation coverage and the bounded later diff, then use
normal active hooks and one ordinary guarded push. Resolve the valid doc thread
with actual correction evidence, not an acknowledgement alone. Optional
annotation/docstring suggestions stay external. No new requirement/ADR/task,
full review, toolchain installation, audit suppression, or unrelated upgrade.

Correction local results:

- Raw authoritative RustSec advisory entries 0346/0347/0348/0349 successfully
  verified patched >=0.23.46; upstream release metadata confirms all four
  fixes. Registry metadata confirms non-yanked target, unchanged dependency
  declarations and minimum Rust 1.71. Sources are linked in the manifest.
- Targeted `cargo update -p rustls@0.23.45 --precise 0.23.46`: handle `63024`,
  actual join `d1d081`, exit 0. Parsed old/new lockfile comparison proves only
  rustls version and checksum changed; no transitive or manifest change.
- After patch `cargo test --locked --lib`: handle `10051`, actual join
  `fda2bd`, exit 0; all 119 default-feature owning-crate tests passed with two
  build jobs and dev/test debug 0. Existing compiler dead-code warnings retained.
- Corrected only General to Preferences in the owned public paragraph, based
  on actual `NOTIFICATIONS_SETTINGS_HREF` and settings route/catalog source.
- Public-doc validator, final catalog/spec/reference coverage and diff gates
  passed after final correction statuses. Normal active-hook commit/push,
  Greptile thread disposition and hosted Cargo Audit closure remain externally
  tracked until delivered. Local cargo-audit unavailable; no install or false
  local pass. No frontend21+28/typecheck/i18n replay, new source test, full
  review, unrelated dependency update, audit ignore, or optional style patch.
