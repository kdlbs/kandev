# First candidate qualification

Status: in progress. This is a validation record, not a deployment receipt.
The private source remains based on exact v0.94.0
(`bf819a0228e742d069c528293d848c985a4d1bd1`). All three central-view and all eleven
assistant implementation work orders are complete. Source is being finalized
after the full repository checks below; no immutable bundle is qualified yet.

## Full-check findings

Qualification began on clean `cdd6eb2cccd42bedad212ed7d55e979a17001c6a`.
`make typecheck` passed across all configured TypeScript workspaces.

The first `make test` stopped in the backend with fourteen PostgreSQL failures.
The same selected tests were run against an isolated checkout of unchanged
v0.94.0, using the same disposable PostgreSQL service:

- Seven Office/run fixtures passed on v0.94.0 but failed on the integration
  branch. Their setup omitted the settings store now required by the retained
  orchestration schema. Initializing settings before Office matches native boot
  order. The fixture-only correction is private commit
  `fc05e06178a0818654f829cf53bb06a9d1483b83`.
- Seven unchanged plugin-state/secrets fixtures failed on both revisions because
  they append keyword options to the test DSN and therefore require keyword
  connection syntax. Supplying the equivalent keyword-form disposable DSN
  resolves them. No production DSN parser or unrelated fixture was changed.

All fourteen tests then passed with the race detector and no skips. The complete
backend rerun passed **286 packages** with PostgreSQL enabled. Native production
code was unchanged by these fixture corrections.

That backend run has 25 conditional skips: unsupported or unavailable provider
capabilities, Windows-specific paths, an opt-in diagnostic, the unauthenticated
GitHub-client case on an authenticated host, folder-opening/platform cases,
foreign-ownership cases requiring root, and unavailable `zsh` / `git-crypt`.
No PostgreSQL case was skipped. These skips are not claims of coverage for those
providers, tools or operating systems.

The subsequent full frontend run exposed a compatibility regression from the
prototype import: New Task unconditionally selected the Kanban dialog in Office
workspaces. Restoring the existing `useInOffice()` selector preserves workspace
mode and feature gating. The existing red test passes after the correction,
along with all **27 tests** in `app-sidebar-new-task-item.test.tsx`. The correction
is commit `2bcfd279a4e1fac760db46d8980e4ec2775dcd27`.

The frontend run was stopped after preserving that failure so the correction
could be applied. It is not recorded as a full pass. The full rerun uses one
worker and a temporary reporter that records each file before/after execution;
it changes no assertions or test configuration. Reporter arguments are passed
directly to Vitest, because the pnpm shortcut consumes `--reporter` itself.

`make lint` passed: backend, full frontend with zero warnings, all 188 harness
files, specification lint and architecture lint. Locale validation and the SQL
guard also pass. Database conformance, browser and packaged-runtime results
remain to be recorded.

The complete frontend rerun executed **1,913 files / 16,242 tests**: 1,912 files
passed, with five failures in `page-client.test.tsx` and four existing skips.
These startup-navigation fixtures leave onboarding incomplete, while the
prototype deliberately defers navigation until setup is complete. The existing
workspace-orchestrator specification requires setup to follow Kanban onboarding.
The fixture now explicitly completes onboarding for normal-startup tests and
adds two completion/navigation checks. The affected page and Office New Task
files pass **37/37 tests** after that correction. Only the affected files were
rerun; the earlier complete invocation remains recorded as failed, followed by
this passing correction. No assertion was skipped and no production onboarding
guard was removed.

The fixture correction is commit `3bb229028339523f770b6ecdc713599faba8b532`.
After extracting repeated fixture constants required by the normal lint hook,
the page's **10 tests** passed again. Both source commits used active pre-commit
and commit-message hooks, with no bypass; formatting, changed-file lint, the
localization guard, architecture and public-copy checks passed normally.

The four frontend skips are the pre-existing dormant human-participant
submission cases in `approval-action-bar.test.tsx`. That file differs from
v0.94.0 only in its shared type import. Its existing visibility tests still run;
the native participant feature itself is outside this contribution.

All five native CLI-shim tests pass. The script target initially stopped on a
missing `unzip`, then on the Make command-line home-path test under GNU Make 4.3.
That path test fails identically on unchanged v0.94.0. The
[current GNU Make shell contract](https://www.gnu.org/software/make/manual/html_node/Shell-Function.html)
exports marked variables to shell functions; the downloaded GNU Make 4.4.1
release notes identify this as the `shell-export` change. Both revisions pass
the complete path test with local Make 4.4.1. The environment now supplies that
Make and Ubuntu's `unzip` package without changing the repository's path logic.
The next script attempt reached desktop updater-signature tests and identified
missing Cargo. An isolated minimal Rust 1.98.1/Cargo 1.98.1 toolchain supplies the
real locked verifier build. The complete `make test-scripts` target then passed
with two build jobs and no overlap with the finished frontend suite.

After the navigation corrections, `make typecheck` passed again. SQL conformance
with `-race -tags fts5 -count=1` passed both required-store packages: **26 top-level
tests / 645 tests and subtests**, with no skips. SQLite and PostgreSQL each have
306 passing engine-specific checks, including fresh schema, replay and the
retained v0.93.0 previous-stable upgrade fixture. This does not replace the
separate rehearsal of the current live v0.94.0 database.

The freshly built Chromium invocation passed all **six tests with one worker and
no retries**: workspace coordinator configuration/chat, Automation delivery,
central task view, and all three native Office sidebar/navigation/New Task
compatibility scenarios. The phone Coordinator flow also passed with one worker
and no retries, using those same fresh artifacts. Immutable-bundle and matching
capture checks follow before the candidate can be marked qualified.

## Provenance and boundaries

The toolchain is Go 1.26.0, Node 24.11.1, pnpm 9.15.9 and GNU Make 4.4.1 on
Linux x86-64. Script checks use UnZip 6.0 (Ubuntu package 6.0-28ubuntu4.1).
Resource-heavy suites run sequentially; Go package concurrency is bounded to two.
PostgreSQL and maintenance-check containers are disposable test resources.
Raw logs and credentials remain outside Git; only safe findings belong here.

The inspected [coordinator](media/coordinator-view/README.md) and
[assistant](media/assistant/README.md) media identify their capture revision.
The final candidate receipt must record any later source differences and the
fresh browser results. Prior provider evidence establishes only the restricted
Linux Claude ACP path described in the [assistant matrix](assistant-evidence.md).

Preparation for private-data rehearsal has verified a network-isolated container
with only loopback, no Docker socket and a compatible C library. A synthetic
read-only-source `VACUUM INTO` trial preserves committed WAL data and passes
SQLite integrity checking without changing its source. These preparation probes
do not constitute a live-data backup, migration or rollback rehearsal.

The live service remains unchanged. Candidate bundle hashing/smoke and delivery
03 migration/rollback evidence are required before live enablement.
