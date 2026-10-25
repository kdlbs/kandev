---
created: 2026-10-14
status: done
requirements:
  - REQ-DESKTOP-DESKTOP-TAURI-APP-001
system_design:
  - ../../specs/desktop/system-design/desktop-tauri-app.md
legacy_specs: []
---

# Implementation Plan: Desktop settings notification test

## Overview

Repair the existing Send test notification action so a desktop user with native
permission can test native delivery even when browser notifications are absent
or denied. Deliver one vertical slice: independent frontend and Rust REDs,
the narrow action/validator correction, then focused checks and documentation.
Desktop owns the contract because it owns native permission and delivery;
the shared settings surface is its consumer.

ROOT reviewed the corrected four-artifact design package and then issued the
later explicit implementation interrupt to the same primary session. Task 01
is complete with the local verification receipts below. Hosted PR validation
and any later merge grant remain separate externally tracked delivery gates.
No local result claims physical OS notification display.

## Evidence and root cause

ROOT's qualified reproduction tested main
`5c6cf5196f886bdb29bcccf6530aa48174efef55`. All five implicated source blobs
are identical on this worktree's authoritative main
`73c5ddb8c9550744655432d03edbe5872b6bbb80`.
Evidence files are external investigation receipts, not permanent test inputs:

- `/tmp/kandev-root-native-notification-test-20261014-1ghfq6o8/qualified-proof.json`
- `/tmp/kandev-root-native-notification-test-20261014-1ghfq6o8/qualification-v3-receipt.json`
- `/tmp/kandev-root-native-notification-test-20261014-1ghfq6o8/qualification-v3.log`

The real `DesktopNotificationsSection`, `useNotificationPermission`,
`useNotificationsState`, `useNotificationsActions`, `nativeNotifications`, and
`createTauriInvokeTransport` were used. Only external provider/store IO and OS
invoke were fixtures. Native permission returned granted, Enabled rendered, and
the actual Test button was clicked with browser `Notification` absent. The
causal case failed with zero `show_native_notification` commands (106 ms).
Controls passed for the real native client/transport (1 ms) and actual browser
Test click with permission granted (28 ms). Vitest exited 1: one failed, two
passed, tests 137 ms. Native handle `21656`, actual join `07ef84`, wrapper wait
0, PID `1566888` absent, source checksum-unlinked, ROOT clean.

Two setup failures are retained but are not causal evidence: stale root
dependencies missing `@json-render`, then unsupported `toBeDisabled` assertions.
Do not replay these runs, read/copy/replay/delete ROOT's `candidate*.tsx` or
`source*.json`, or derive the permanent RED from that private source. Author an
independent regression using the real consumer chain and DOM disabled property.
The fixture proves a failure before RPC; OS display itself was not exercised.

`handleTestNotification` currently inspects only the browser Notification API,
although the permission owner uses native permission in Tauri. Routing alone
cannot repair this: Rust's real `validate_request` accepts only three session
prefixes and `system.update_available`, with no dedicated test kind.

## Assumption check and contract reconciliation

- Confirmed: native target when available; existing browser-only fallback;
  one dedicated app-scoped test identity; independent click identities; no
  provider/backend/auth changes, delegates, or new sessions.
- Verified: native permission get/request/show are separate narrow commands;
  the request already permits omitted task/session fields; Rust validates
  origin before delivery and denies before claiming identity.
- Missing requirement: explicit settings Test outcomes. Add `.15` through
  `.17` to the existing desktop requirement, keeping `.1` through `.14` verbatim.
- Agent design choice under authorized scope: query native permission per
  gesture; request only undecided permission from that Test click; refresh
  denied/requested permission; contain failure without a false success or
  misleading permission error after display failure. Existing console failure
  diagnostics and permission guidance are reused; no new delivery UI.
- ROOT's required correctness correction: the current English body says
  "browser notifications are working", which is false for native-only delivery.
  Both targets shall instead use the existing body key with "If you can read
  this, notifications are working on this device." Correct only that value
  across all seven settings catalogs; the generic Test title remains unchanged.
- Existing [native integration ADR](../../decisions/0039-native-desktop-integration-boundary.md)
  and [semantic-event ADR](../../decisions/2026-07-24-semantic-notification-events.md)
  govern the unchanged boundaries. A local diagnostic kind is not a fabricated
  session/update occurrence. No material open question or new ADR is needed.

## Scope

### In scope

- Existing Test control target routing and contained permission/result handling.
- One named Rust-native local test prefix with app scope, payload bounds,
  non-empty occurrence, and existing origin/permission/dedup enforcement.
- Independent rendered consumer-chain regression, browser controls, and Rust
  validator/dedup coverage.
- Owning desktop pair, this manifest/order, and the smallest affected public
  desktop notification paragraph after implementation.
- ROOT's later bounded security correction: only rustls 0.23.45 to 0.23.46
  in the existing desktop Cargo.lock, plus the owned guide path correction
  General to Preferences. No manifest or unrelated dependency update.
- Device-neutral `settings:testNotificationBody` in English, pt-pt, zh-cn,
  zh-hk, zh-tw, ja, and ko; existing zh-hant generation for the Traditional pair,
  actual native/browser delivered-body assertions, and i18n gates.

### Out of scope

- Provider drafts, saves, subscriptions, backend events, Go code, auth or origin
  ownership, ordinary session/update policy, and native command/capability expansion.
- Service workers, mobile browser notification features, new translation keys,
  unrelated translation edits or copy polish, new dependencies or unrelated
  upgrades beyond the explicitly authorized rustls lockfile patch,
  configuration, flags, SDK abstractions, cleanup, race/recovery/performance work.
- Layout, touch, navigation, scrolling, breakpoint changes, full desktop rebuild,
  broad suites, and proof of physical OS notification display from jsdom.
- Foreign refs, worktrees, processes, dependencies/caches, protected tasks and
  stopped candidates, and the protected anonymous volume named in the task brief.

## Technical approach

Use the owning [Settings notification test design](../../specs/desktop/system-design/desktop-tauri-app.md#settings-notification-test).
Extract `sendTestNotification` into
`apps/web/components/settings/notification-test-action.ts`; the existing
actions hook delegates its Test action there. Keep this helper specific to the
settings action and use the actual native client and existing `generateUUID()`.
The near-600-line actions file must not grow past repository limits.

Rust adds only `desktop.test_notification:` to `validate_request`, requiring a
non-empty suffix and absent task/session for that kind. Keep existing bounds
and session/update handling. The origin check, show command, permission-before-
claim, notifier builder, cache limit, and failure-release flow stay unchanged.

| Client | Target/identity | Intended behavior | Evidence/fallback |
| --- | --- | --- | --- |
| Tauri, browser absent/denied | Actual native invoke; `desktop.test_notification:<occurrence>` | Native permission owns admission and display attempt | Rendered real consumer through OS invoke fixture; no browser fallback after native failure |
| Tauri, undecided native permission | Native get/request/show | Prompt only on explicit Test click, send only if granted | Granted/denied/request-rejection rendered cases |
| Tauri, denied/error/display failure | Same native target | Existing permission guidance or contained delivery diagnostic, no false success | Real permission hook/result and rejection cases |
| Browser desktop/mobile | Web Notification API, no native bridge | Existing default prompt/granted send/denied or absent no-op | Actual button browser controls; unsupported remains unsupported |
| Ordinary native semantic events | Existing session/update identities | Existing validation, context, permission, dedup behavior | Real Rust semantic and unknown-event controls |

## ASCII UI preview

`UI-01: Settings notification permission controls`, Notifications settings, native
permission granted. Source-supported control order is unchanged before/after:

```text
BEFORE (desktop)                  AFTER (desktop)
Desktop notifications            Desktop notifications
[Enabled] [Refresh] [Test bell]   [Enabled] [Refresh] [Test bell]
Test -> browser only, no RPC     Test -> native OS command
```

The same existing group/control composition serves phone browsers:

```text
PHONE BROWSER (before and after)
Desktop notifications
[Enable/Enabled] [Refresh] [Test bell]
Test -> existing browser permission and delivery
```

Labels illustrate translated copy and icons, not new text or pixel geometry.
Existing disabled Enabled state, denied OS/browser guidance, unsupported state,
and permission-query error remain in this group. No new loading/result state.
The structural requirement is the existing control order and action reachability;
all existing scroll ownership, touch sizing, and responsive composition remain.
The closest shipped exemplar is this settings group and its real section.
AC `.15` through `.17` map to the rendered Test suite.

Mobile-parity's state/data exception applies because only shared action routing
changes, with no viewport-dependent behavior or layout change. The real rendered
button and browser fallback controls provide the focused rendered check. No
new mobile Playwright or geometry work is required for this bounded repair.

## Tests

The permanent regression coverage below was authored after the later
implementation release, independently of ROOT's private reproduction source.

| Acceptance/boundary | Test file and cases |
| --- | --- |
| `.15` | `notification-test-delivery.test.tsx`: native-granted actual Test click with browser absent (independent causal RED), plus browser denied while native granted; actual native client and transport stay real |
| `.15` browser/mobile preservation | Same suite: browser-only granted send, default permission grant/deny, denied no-send, unavailable no-op, browser permission/constructor rejection contained |
| `.16` | Same suite: native prompt/prompt-with-rationale grant and denial; already-denied no re-request; get/request rejection shows existing error; show permission-denied refreshes; display-failed/show rejection preserves granted permission and contains failure; duplicate has no retry/success claim |
| `.15` diagnostic correctness | Same suite: actual native invoke body and actual browser constructor body equal the device-neutral English diagnostic, with unchanged Test title; native-only delivery does not claim browser capability. Existing catalog checks gate all seven translations |
| `.17` | Same suite: repeated actual clicks produce distinct bounded identities and translated copy with absent task/session; no provider/store mutation or browser calls on native path |
| `.17`, narrow validation | `src/native_notifications.rs`: `bridge_accepts_app_scoped_settings_test` (independent Rust RED), `bridge_rejects_invalid_settings_test_scope_and_occurrence`, `bridge_bounds_settings_test_payload`, `settings_test_occurrences_keep_existing_dedup_rules` |
| Existing `.8`/ADR boundary | Same real Rust suite: known semantic types accepted; unknown/retired/lookalike types rejected; session missing/empty task rejected; app-scoped update unchanged; denied does not claim and display failure releases identity |

New rendered suite must not mock the action helper, native client/transport,
section, permission hook, settings state/actions, or UUID helper. Restore global
Notification and Tauri internals after every case. Use existing testing-library
`act`, `fireEvent`, and DOM assertions; unsupported JestDOM matchers are forbidden
unless explicitly imported through an established test setup.

## E2E tests

The rendered suite traverses the real action path to an external OS invoke
fixture; Rust covers actual validator logic. It does not prove native OS display.
No new Playwright flow is justified for action-only routing with the documented
mobile-parity exception. Existing phone settings coverage in
`apps/web/e2e/tests/settings/mobile-notification-events.spec.ts` remains layout/
subscription evidence, not native Test evidence. Required current hosted product
CI, including Backend/Frontend/E2E parents, remains mandatory after delivery.
Do not rebuild a desktop bundle merely to repeat a startup smoke.

## Public documentation

Assessment: `docs/public/desktop-app.md` is a how-to guide and its notification
paragraph claims notifications are limited to session types. After implementation,
that affected paragraph was amended for the explicit native settings Test action,
independent browser permission, user-initiated native consent, repeated tests,
and the generic device notification diagnostic. Do not describe a native Test
as proof that the browser Notification API works.
Preserve unrelated copy. `docs/public/operations.md` describes ordinary update
policy, which this repair does not change; README/screenshots contain no Test
claim to amend. Only the affected desktop notification paragraph changed.

## Work orders

- [x] [Task 01: Route settings Test through bounded native delivery](task-01-native-test-delivery.md)

One order, wave 1, no dependencies; sequential in the same primary session.
Completed native-integration, isolated-startup, repository-discovery, and child-
reaping plans retain their historical scopes/results. The notification reliability
plan's native permission work is complete. No existing package owns this Test
fix; none of its old test counts count as new evidence.

## Verification results

Initial Task 01 delivery implemented and locally verified on 2026-10-14 after ROOT's later
explicit release. See the exact [work-order results](task-01-native-test-delivery.md#results).

- Independent frontend RED: native click sent zero commands; browser body was
  the old false browser claim. Native transport control passed.
- Independent Rust RED: actual validator rejected the dedicated test identity.
- Final new rendered suite: 21 passed. Three unchanged affected suites: 28
  passed in the affected four-suite run; its one incorrect new-test selector
  was corrected and only the changed suite rerun.
- Real native notification Rust scope: 15 passed; Rust formatting passed.
- Scope lint and the single final 8 GiB web typecheck passed.
- Full i18n check and staged new-code ratchet passed (one added production
  helper plus one modified production file; new fixture is staged/test-exempt).
- All seven settings catalogs changed only the existing body value. Existing
  zh-hant generation changed one web key per Traditional catalog; backend
  catalogs were rewritten identically and produced no backend diff.
- Public-doc content validator passed, 48 pages. Mobile-parity uses the reviewed
  state/action-only exception and actual rendered browser controls. No UI
  geometry or screenshot surface changed. No Playwright or desktop bundle run.
- Final catalog/specification/coverage and diff gates are recorded below.


Final implementation documentation gates:

- `python3 scripts/list-docs.py validate`: passed, 382 decisions and 1,609 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed after final implementation statuses.
- Actual staged-file `validateCoverage` preflight: passed, `ok=true`,
  `status=covered`, `errors=[]`, exactly one work order; includes the new helper
  and all seven locale paths. No documentation-only exemption or validator-test replay.
- `git diff --check` and `git diff --cached --check`: passed. Only the 16 owned
  files were staged in the initial delivery; no dependency, lock, generated
  backend, or foreign changes occurred in that initial pass. The later bounded
  security correction below is the explicit lockfile exception.

Design checks on 2026-10-14:

- `python3 scripts/list-docs.py validate`: passed, 382 decisions and 1,609 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed; no validator-test replay.
- Repository `validateCoverage` preflight: `ok=true`, `status=covered`,
  `errors=[]`, exactly this work order. Proposed frontend and Rust production
  paths exercised the real coverage branch, not the docs-only exemption.
- Direct comparison to HEAD: existing desktop AC `.1` through `.14` preserved
  verbatim. Owning requirement is declared by the design and work order;
  design reference is included by the manifest, and sibling links resolve.
- `git diff --check -- docs/specs/desktop docs/plans/desktop-notification-test`:
  passed. Status confirms the two modified owning docs and two new package
  files, all unstaged/uncommitted. No production or permanent test files changed.
- Existing Node 24.18.0, cached pnpm 9.15.9, and stable Rust 1.97.1 confirmed by
  version queries only. Dependency installation and product validation not run.

ROOT-required diagnostic amendment checks on 2026-10-14:

- Catalog and all-spec lint rerun once after the corrected package: passed,
  382 decisions and 1,609 specifications; no validator-test suite.
- Coverage preflight: passed, `ok=true`, `status=covered`, `errors=[]`, exactly
  one order, with the planned new helper and seven locale paths included.
- Existing AC `.1` through `.14` remain verbatim. All seven production locale
  files were byte-compared to HEAD and remain unchanged during design.
- Scoped diff whitespace check passed. Only the four design artifacts changed;
  implementation and product verification remain gated on the later release.

## Resources and tool assumptions

Read-only design inspection on 2026-10-14: 27 GiB RAM total, about 1.1 GiB free
and 13 GiB available (page cache), swap fully used, 58 GiB disk available.
This fresh managed worktree has no `apps/node_modules` or local Rust target.
Default shell Node/pnpm are absent; inherited system Cargo 1.75 is too old.
Existing Node 24.18.0 and cached pnpm 9.15.9 are present, plus stable Rust 1.97.1
under the user's existing toolchain. Use explicit existing tool paths, never
change shared defaults. No install or Rust build was run during design.

After release, inspect current memory/cache assumptions before the sole heavy
lease. Run a normal frozen workspace install once if missing. Use bounded two
Cargo jobs with debug information disabled when appropriate; keep locked deps,
normal validation, and foreign caches. Final web typecheck uses the documented
8 GiB heap once. Retain every original command handle, poll it to actual exit,
and record failures truthfully. Resource/tool/auth failure checkpoints ROOT;
never delete caches, kill foreign processes, or widen the pipeline to proceed.

## Risks

- A settings action using a session/update identity would fabricate domain
  meaning; the dedicated local type and real Rust RED prevent that shortcut.
- Reusing one identity would make later clicks disappear into dedup.
- Passing display failure to the permission error callback would falsely label
  a granted permission as broken; result/rejection tests must preserve state.
- Native-only delivery cannot establish browser capability. The corrected
  device-neutral body must reach both actual delivery targets, and Traditional
  Chinese generation must not introduce unrelated catalog changes.
- Physical OS delivery and OS-specific prompt presentation remain unproven by
  jsdom/default-feature Rust tests. Report that limit accurately.


## Bounded security and documentation correction

ROOT later released one correction pass after the actual current-head Cargo
Audit reported four rustls 0.23.45 vulnerabilities and Greptile identified the
owned guide's wrong settings section. Reopen the same order for only the
compatible rustls 0.23.46 Cargo.lock patch and General to Preferences in the
new public paragraph. The reviewed notification code, ABI, guards, UI, locale
values, requirement, and system design remain unchanged.

Verify authoritative advisory database and Cargo registry metadata, then use
`cargo update -p rustls@0.23.45 --precise 0.23.46`. Inspect the lockfile diff;
only strictly required compatible transitives may change with resolved-manifest
evidence. Never run an unqualified update, change a manifest, suppress an audit,
or widen an upgrade. An unavailable or incompatible target checkpoints ROOT.
Run the owning crate's default-feature `cargo test --locked --lib` once with
two jobs and dev/test debug 0. No new version-mirroring tests or unchanged
frontend/typecheck/i18n replay. Public-doc content validation, actual-file
coverage, and final reference/diff checks cover the correction. Local cargo-audit
is unavailable in the inspected existing tool paths; install no audit tool and
claim no local audit pass. Hosted Cargo Audit at the corrective head must close
all four findings. Old-head Cargo Audit retry budget remains zero.

Completed substantive CodeRabbit/Claude/Greptile reports at the initial head
plus a bounded assessment of this later diff provide review evidence; no
routine full review or Codex service retry. Optional docstrings, Clippy timeout,
and serde annotation suggestions remain deferred externally. Keep the original
hosted observer and clock while it can follow the new head. Any replacement
requires the original actual join and known absence, with separate receipts.

Correction local results on 2026-10-14:

- Authoritative raw RustSec entries for
  [0346](https://raw.githubusercontent.com/RustSec/advisory-db/main/crates/rustls/RUSTSEC-2026-0346.md),
  [0347](https://raw.githubusercontent.com/RustSec/advisory-db/main/crates/rustls/RUSTSEC-2026-0347.md),
  [0348](https://raw.githubusercontent.com/RustSec/advisory-db/main/crates/rustls/RUSTSEC-2026-0348.md),
  and [0349](https://raw.githubusercontent.com/RustSec/advisory-db/main/crates/rustls/RUSTSEC-2026-0349.md)
  were successfully read; all identify rustls >=0.23.46 as patched. The HTML
  pages previously unavailable to ROOT were not presented as verified.
- Authenticated [upstream release](https://github.com/rustls/rustls/releases/tag/v/0.23.46)
  metadata confirms the four fixes, published 2026-10-14T09:51:22Z.
  [Cargo registry metadata](https://index.crates.io/ru/st/rustls) confirms the
  non-yanked 0.23.46 target, identical dependency declarations, and unchanged
  Rust minimum 1.71. Direct receipt `777a06`, exit 0.
- Targeted resolver handle `63024`, actual join `d1d081`, exit 0. Parsed lockfile
  diff permits only rustls version/checksum and confirms no transitive or
  manifest changes. Actual bounded source diff is two lock fields and one
  word in the new public instruction, plus this package's scope/results.
- `cargo test --locked --lib`: handle `10051`, actual join `fda2bd`, exit 0,
  119 passed, two jobs, dev/test debug 0, default features, once after patch.
  No unchanged frontend/typecheck/i18n replay or new generated-version test.
- Public/reference/coverage/diff gates passed once after final correction
  statuses; normal active-hook delivery and current-head hosted audit closure
  are externally tracked. No local cargo-audit tool was found or installed;
  no local audit pass or vulnerability suppression is claimed.
