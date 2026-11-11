---
id: 04-compatibility
title: Packaged upgrade and recovery evidence
status: in_progress
wave: 3
depends_on:
  - 01-host-contract
  - 02-template-plugin
  - 03-legacy-import
plan: plan.md
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.4
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.6
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.6
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
---

# Task 04: Packaged upgrade and recovery evidence

## Summary

Exercise the actual packaged plugin on isolated hosts at pinned revisions and
produce compatibility, UI, upgrade, and rollback receipts. Complete the two PR
records with actual checks and synthetic screenshots; publishing remains gated
by host/SDK support and separate release direction.

## In scope

- In the plugin repository, add a bounded compatibility harness with explicit
  executable/package paths and SHA-256 pins, isolated homes/ports/databases,
  mock transports, managed browser checks, and cleanup of owned processes/files.
- Negative install/capability case on unchanged official v0.99.0; operation on
  the exact development extended host; older plugin compatibility; plugin
  disable/re-enable/crash/uninstall/reinstall; host/plugin restart and upgrade.
- Candidate migration and coherent host+plugin backup/restore/rollback drill;
  compare ownership, bindings, templates, provider/subscription/history and
  task/session inventories. Do not claim arbitrary host downgrade compatibility.
- Verify a latest-stable-plus-minimal-patch candidate as well as the main-based
  contribution. Re-resolve latest stable at execution, record exact source/digest,
  reject unsupported unchanged hosts, and test six-event behavior on the candidate.
- Prove captured immutable revisions survive concurrent Save/activation, backup
  quiescence, lost acknowledgements, plugin restart, and delayed format responses.
  Reject mixed backup manifests; restore all selected snapshots before binding.
- After actual publication, rerun the released-SDK/stable-host lane. Before
  then, leave that lane blocked and label development evidence accordingly.

## Out of scope

Production upgrade, SMTP test mail, merge, release/tag, marketplace submission,
or deleting any active review worktree/branch.

## Acceptance

1. Receipts identify immutable inputs and distinguish unsupported, passed,
   skipped, blocked, and uncertain outcomes; failure cannot be reported as green.
2. Actual plugin UI composition passes desktop/phone UI-01 workflows, cancellation,
   conflicts, preview-no-send, ownership, and lifecycle checks with screenshots.
3. Restart/upgrade/restore preserve configuration; rollback recovers the selected
   prior candidate without duplicate sends, cross-owner access, or data loss.

## Verification

Run from the independent plugin task worktree root. The harness/matrix below
are new work-order deliverables, not existing tools. The matrix must point only
to disposable hosts, synthetic fixture DBs, and explicit package artifacts. It
must not discover or attach to the operator's primary instance.

```bash
make verify-package-host
node scripts/verify-host-compatibility.mjs --matrix testdata/host-compatibility.json --output .build/compatibility
git diff --check
```

The harness includes rendered desktop/phone checks using the repository's pinned
browser dependency and records screenshots. Use one browser worker. Add complete
UI-01 scenarios without changing its composition; this task introduces no new
UI design. Run `make verify-package` for all manifest-declared platforms before
release, recording unexecuted native runtime coverage separately from cross-builds.

In the host worktree, run Task 01's relevant checks after any integration fixes
and the CONTRIBUTING.md pre-PR commands recorded in the plan. Record each result.
Do not auto-repeat uncertain external writes or invent aggregate passes.

## Files likely touched

Plugin `scripts/verify-host-compatibility.mjs`,
`testdata/host-compatibility.json`, packaged UI browser scenarios, compatibility
and recovery docs, and the two PR verification records. No core product changes
are planned here; route required contract changes back to Task 01 and the design.

## Dependencies

Tasks 01-03, exact candidate artifacts, and a disposable test environment.
The stable-host release lane also needs the actual supporting SDK publication.

## Risks

Mail delivery cannot promise exactly once after an uncertain transport outcome.
This work proves formatter invocation does not add sends, not SMTP exactly-once.

## Parallelism

Sequential after candidate artifacts and migration parity are available.

## Inputs

Plan compatibility table, UI-01, paired design, source/package pins, and private
snapshot rules. Preserve prior operational runtime and personal data.

## Results

The actual host-platform development package passed desktop and Pixel 5 editor
scenarios, all six event previews, whole-pair fallback, preparation without
activation, explicit activation, reload, and restore-on-Save. Screenshots and
artifact digest receipts remain in the plugin worktree's ignored
`.build/formatter-browser/` directory.

A live, isolated host passed bounded-lease backup capture, configuration-write
rejection while health remained available, later-edit six-event export with
four-event projection, and same-candidate host/plugin DB restore with exact
readback of three bindings, including the migrated provider. Native backup tests also reject changed files,
foreign ownership/installation, unsupported runtime configuration, and expired
capture leases. These results do not prove arbitrary host downgrade safety.

Latest stable was resolved as `v0.99.1`, source
`813a741b21108ea8ed10ab58954d85c2e5205999`. An unmodified stable-source binary
rejected the actual formatter archive with HTTP 400 before activation. The
minimal patched stable candidate `cf8579699e40e38dcb2063d1692c18444c8afbd5`
reports `v0.99.1+notification-formatter.1` and passed the same packaged editor,
six-event preview, four-template migration, and three-binding recovery drill.
The candidate uses the normal `fts5` backend target; protobuf regeneration is
clean, SDK race tests pass, and frontend typecheck/build pass. These are local
stable-source builds, not verification of official release archive bytes.

Remaining gates are the full lifecycle/authenticated ownership matrix, the
minimum-host v0.99.0 rejection case, released-SDK lane, and actual operational
runtime/session continuity. The earlier template v0.99.0 smoke is not formatter
compatibility evidence. Synthetic screenshots and provenance are committed in
the independent plugin repository under `docs/images/formatter/`.

### CI fixture regression follow-up (2026-11-11)

The refreshed PR #5141 head was
`801af86298d53e52ad5442d834976bb516d96240`. The latest maintainer commit
adds error redaction and 512 KiB receive limits to both formatter RPCs.
Those implementations and their regression tests remain unchanged. The test
corrections are recorded at
`a3ce96d9ab63539d031bc172fefb0ceea027f9a2`; the subsequent documentation
commit changes only this verification record.

The refreshed failures came from Backend run `42878056823`, Frontend run
`42878056886`, and E2E run `42878056888`. The fixture manifest failure
reproduced locally: `Actions[0]` was `notification.prepare`, while the test
expected `connection-status`. Key-based checks now independently retain the
workspace action and verify the provider-scoped preparation action, body bound,
and formatter declaration. The draft transport now answers formatter GETs with
`binding`, `formatters`, and `events`, while deferring only provider mutations.
Existing assertions still verify exact writes, editing during Save, failure
feedback, dirty state, and Reset to the acknowledged baseline.

The rescan mock now matches only the provider collection URL. It no longer
returns provider-list JSON to formatter GETs. Initial loads and rescans use
causal HTTP waits. Phone and 900px coarse-tablet cases retain an edited event
through rescan, retain Save, enforce 44px targets, and check viewport fit.
Composition checks name all five real groups: Push Notifications, External
Providers, Webhooks, Notification Events, and Notification content. Each must
have one expanded frame without nested groups. Permission and sound remain
compact rows. No production UI or core/plugin responsibility changed.

#### Focused verification

Final checks used Node `24.11.1`, pnpm `9.15.9`, and Go `1.26.0` where
specified. Early host-default attempts used Node 26 and Go 1.27; their timeout
results are not substituted for the final checks.

| Check | Observed result |
| --- | --- |
| `go test ./cmd/plugin-fixture -run 'FixtureManifest\|FixtureFormatter' -count=1` | Passed with Go 1.26.0. |
| `go test -p 2 ./internal/notifications/... -count=1 -timeout=180s` | All notification packages passed with Go 1.26.0. |
| `go test -p 2 ./internal/plugins ./internal/plugins/manifest ./internal/plugins/runtime -run 'Formatter\|Notification\|FixtureManifest' -count=1 -timeout=180s` | Plugin and manifest checks passed; runtime reported no matching tests. |
| `go test -race -p 2 ./internal/notifications/service ./internal/plugins -run 'Formatter\|Notification' -count=1 -timeout=120s` | Passed with Go 1.26.0, including authority, fallback, deadline, and logging regressions. |
| `go test -race ./... -count=1 -timeout=180s` in `pluginsdk` | Passed with Go 1.26.0, including within/over-limit responses for both 512 KiB RPC limits. |
| Focused Vitest, `--pool=forks --maxWorkers=1` | Six files, 51 tests passed with the default test timeout: notification-save-drafts, notification-formatters, notification-formatter-save, settings-save-provider, use-notification-formatter, and notification-formatters-api. |
| Standard `pnpm e2e:raw`, Chromium and mobile-chrome | All 13 tests passed in 2.1 minutes across apprise-rescan, mobile-apprise-rescan, settings-composition, notification-formatters, and mobile-notification-formatters. One worker, no timeout override or freshness bypass. |
| Web `pnpm run typecheck`, `pnpm run lint`, and `pnpm run build:e2e` | Passed during the follow-up. A later full-lint repetition timed out; it is not another pass. |
| Changed fixture Go lint | Repository-pinned golangci-lint reported zero issues. |
| Two-axis review | No remaining Standards or Spec findings after adding the causal waits. |

Early browser attempts stopped at `Loading Settings…` under host load. A
temporary local 30-second assertion configuration helped distinguish startup
delay from fixture failures. It was deleted before the final standard-config
13-test pass. No checked-in timeout or assertion was relaxed.

#### Cancelled shard and required-check limitations

GitHub's jobs API reports E2E shard 8/28, job `130173661683`, as `cancelled`
after 55 minutes. Its log is unavailable and its report/resource uploads did
not finish. The final E2E report consequently rejects missing native tests;
the cancellation is not evidence that those tests passed.

The downloaded native manifest identifies 58 project/file units and 136 tests
for that shard. A local replay selected both owning projects and collected all
136 tests, but reached its 60-minute execution limit without a final report.
Its log records 108 completed attempts, including retries: 88 passing and 20
failing attempts. These are not unique-test outcome counts. A separate replay
of all 43 mobile tests with a canonical `/private/var/...` temporary path
completed: 42 passed and one oversized-message test failed. Task/worktree
failures include `repository_context_unavailable` and rejection of the macOS
`/var` symlink. Baseline causality was not established for every failure.

Required repository checks were attempted, not completed successfully:

- `make -k typecheck test lint GOFLAGS=-p=2` exceeded 30 minutes during backend
  execution. Root classic TypeScript checking first exhausted its heap;
  an 8 GiB retry exceeded ten minutes. Web's separate native typecheck passed.
- Backend output includes a 75-second non-loopback executor-fixture connection
  timeout, hostutility/managedruntime failures, launcher identity failures, and
  worktree tests rejecting `/var`. Full notification/SDK passes above do not
  make the full backend suite green.
- A full web `pnpm test` exceeded 30 minutes. It reported failures in sidebar
  archived freshness, agent-profile catalogue saving, storage maintenance,
  IndexedDB logging, and external-link dialog tests before termination. There
  was no final suite summary.
- CLI tests passed all five tests. Root script tests first selected a global
  pnpm 11 inside a disposable fixture; pinning pnpm 9.15.9 removed that failure,
  but a later `pr-state` strict-deadline test failed. Script tests remain red.
- Full lint first lacked golangci-lint; retrying with the repository-pinned
  executable exceeded ten minutes. Web lint, harness lint, spec lint, and
  architecture lint completed during separate attempts. Full lint is unverified.
- Full multi-project `make test-e2e`, fresh remote CI for the corrected commit,
  Windows execution of the corrected fixture, and live PostgreSQL verification
  remain open. Push and all external publication require separate approval.

#### Packaged editor and screenshot readback

The actual independent plugin archive passed the disposable host smoke on the
candidate runtime and web build. It verified desktop and Pixel 5 editing,
all six event previews, whole-pair fallback, prepare before explicit activation,
reload/default restoration, four-template import, and exact three-binding
recovery. Synthetic providers stayed disabled and unsubscribed.

- Host binary SHA-256:
  `ce6c4830d3f5ff3432e32d65abb1282bba25fa1c545597784b890620aa5b27c5`.
- Plugin archive SHA-256:
  `c4917938428e06146eedab91976c8c6fa61fe2db0877505b468991ec742ce655`.
- Plugin checkout: `d8910f6928e3aa56b48a9fcfe38458dd65ad5a7e`.
- Fresh desktop/phone editor and control captures plus `receipt.json` are retained
  in that plugin worktree at `.build/formatter-browser/pr5141-ci/`.
- All four fresh captures were visually compared with the existing
  `docs/images/formatter/` captures. Provider ordering and scroll framing differ;
  fields and controls remain consistent. The fresh captures are the current
  local evidence, while the linked PR images remain historical until publication
  is approved. Rescan captures were also refreshed in the host's `.pr-assets/`.

Local diagnostic logs use the `pr5141-` prefix in the task's temporary artifact
directory. The broader failures above keep the overall completion gate open;
this record does not claim merge readiness, publication, or deployment.
