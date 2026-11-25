# Remaining SSH manager acceptance: 2026-10-25

## Scope and provenance

The user requested implementation of the remaining work after draft PR
[#4778](https://github.com/kdlbs/kandev/pull/4778) was opened. This continuation
uses the recovered, reviewed package. It does not reopen design or promote the
feature. Original source task/session IDs and recovered artifacts remain in
[the recovery record](recovery.md).

Current task: `a2845dea-8086-4b3a-824c-65e1941d4cd8`.
Current session: `13973a35-0812-478d-a620-2e36314c61f1`.
Branch: `feature/recover-and-resume-s-1mt`.
Starting commit: `20661f71af0c8ac42f233d8de418411088ced639`.
At this 2026-10-25 handoff, the changes below were local and uncommitted. The
draft PR contained the previously pushed commits. The user authorized committing
and pushing this continuation on 2026-10-26. CI and review comments remain deferred.

## Closed implementation gaps

- Managed orphan cleanup now uses the saved executor's shared `OpenExecutor`
  capability. Its route reference remains held until inventory and cleanup end.
  Disabled, stale, and unavailable managed routes do not fall back to inline
  credentials. Existing task fences and remote process checks still govern
  signals; task directories remain intact.
- Identity list/detail responses include derived connection, executor, and
  retained-session dependencies. Bastion use includes dependent destinations.
  Refused deletion returns the same safe details with HTTP 409. Desktop and
  phone link to the existing management pages. A concurrent deletion refusal
  refreshes dependency details. Derived data never enters stored key metadata.
- Browser migration covers direct and single-bastion routes on desktop and
  touch phone. It checks cancellation, preserved pins, explicit trust when a
  pin is absent, concurrent edits, successful conversion, reload, and rejection
  of unsupported chains without changing the legacy profile.
- A native desktop test uses the actual command builder, process spawner, and
  built Go runtime. Both launches omit SSH agent variables. A generated managed
  identity authenticates against an isolated OpenSSH server before and after
  restart, with the same public key and credential generation.
- PostgreSQL coverage now includes identity create/delete rollback with encrypted
  storage, concurrent idempotency, binding rollback, runtime-reference rollback,
  stale revisions, adoption conflicts, enrollment journaling/recovery, dependency
  reporting, and competing bastion edits. Store conformance also ran on both
  SQLite and PostgreSQL.

## Acceptance mapping

The requirement IDs below use `AC-EXECUTORS-SSH-CONNECTIONS-` as their prefix.
Earlier evidence remains in [validation](validation.md) and the completed
[navigation package](../../ssh-connections-executor-navigation/plan.md).

| Criteria | Primary evidence |
| --- | --- |
| 001.1, 001.2 | Identity authorization/idempotency/import/private-output tests; real desktop/phone identity generation, protected import, clipboard and cancellation coverage recorded in validation. |
| 001.3, 001.4, 005.5 | Direct/bastion process recovery and protection-loss tests; real managed executor restart with a restored health tunnel; native desktop managed-key restart without an agent. |
| 001.5 | HTTP/service gates, managed-selector HTTP/WS rejection tests, and existing authenticated administrator/member browser checks. |
| 001.6 | `identity_dependencies_test.go`: direct/bastion connections, retained sessions, executor references, HTTP 409 details, member rejection, unchanged metadata, and unused-key deletion without remote revocation. Hook regression and desktop/phone dependency-link tests. |
| 002.1, 002.3, 002.6 | Draft/settings browser coverage, version and trust-receipt tests, bastion stamp invalidation, stale writes, atomic binding/reference checks, and PostgreSQL concurrency tests. |
| 002.2, 002.7, 002.8 | Protocol tests with separate destination/bastion identities and pins, changed-key rejection before credentials, direct/bastion enrollment, cycle/chain guards, and real OpenSSH browser migration. |
| 002.4, 002.5 | Shared executor/profile binding and runtime admission tests, retained generation checks, two profiles in the real managed runtime fixture, and restored-tunnel integration. Cleanup now holds the same protected route capability. |
| 003.1-003.6 | Enrollment script safety/idempotency tests, protocol tests for both bootstrap modes over both routes, unchanged bastion authorization, partial outcomes/audit failures, PostgreSQL recovery, and real OpenSSH enrollment/restart cases. |
| 004.1-004.7, 005.6 | Completed navigation/context package, preflight and hop diagnostics, duplicate/stale response and secret-lifetime tests, localized dependency details, phone touch/overflow checks, and real migration flows. |
| 004.5, 005.2 | Existing connection/executor readiness separation and explicit setup tests; real migration preserves resolved fields, requires current trusted SSH access, refuses concurrent changes, and preserves unsupported profiles. |
| 005.1, 005.3, 005.4 | Earlier legacy agent/file/IdentityAgent/config/ProxyJump checks, current recovery cases, default-off registration and selector gates, protected cleanup refusal, and audit/redaction/rollback coverage. |

This mapping records the implemented package and its evidence. It does not
claim every platform or every possible failure interleaving has an E2E test.

## Fresh validation

All commands ran in this isolated workspace. Browser runs used one worker and
ran sequentially. Real SSH actions used disposable fixtures only.

- Focused Go packages passed with `-race`: `sshconnections`, `sshtransport`,
  and lifecycle SSH cleanup/recovery. New/changed regressions passed after their
  final edits. Backend composition and administrative API checks passed.
- Changed-package lint through `scripts/lint-go-changed` passed with zero issues.
- Nine PostgreSQL SSH test families passed, including two identity rollback
  subcases. The secrets PostgreSQL suite also passed. `TestStoreConformance`
  passed on both dialects, including schema replay, CRUD, conflicts, and
  transactions. `go run -trimpath ./cmd/sqlguard ./internal` passed.
- Seven focused frontend unit cases passed. Frontend typecheck, changed-source
  and test ESLint, localization checks, and the copy ratchet passed.
- Eight real OpenSSH browser migration cases passed in 2.8 minutes.
- Ten real OpenSSH adoption/enrollment/executor/recovery cases passed in 3.6
  minutes. The managed runtime case also verifies the retained-session dependency
  link against a real task and its session.
- Desktop and phone dependency-link browser cases passed. The final phone
  capture was inspected: the explanation, disabled delete action, and connection
  link remain usable in the settings scroller without horizontal overflow.
- The opt-in native desktop test passed against a built runtime. It checks the
  native backend launch/restart boundary on Linux, rather than WebView rendering
  or a macOS/Windows GUI acceptance run. Its command and prerequisites are in
  `apps/desktop/e2e/README.md`.
- Public documentation validation passed: 63 validator tests and 54 pages.
  Specification validation passed: 393 decisions and 1673 specifications.
  Full specification lint and whitespace checks passed.

## Failures, environment, and cleanup

A browser attempt stopped at the artifact freshness guard because source files
changed during a build. The valid run rebuilt the artifacts without bypassing
that guard. The first native SSH fixture used `/tmp`, which OpenSSH rejected
under `StrictModes`; using a private cache child directory resolved it without
weakening host verification or changing existing permissions.

The conformance test process passed, but its temporary PostgreSQL server needed
longer than the wrapper's 60-second shutdown wait to complete its checkpoint.
The wrapper therefore returned failure during cleanup. A later check verified
`database system is shut down`, then removed the fixture data. No forced stop
was needed. This cleanup failure is distinct from the passing test result.

The dedicated Docker daemon had no containers left after testing. It stopped
successfully. Cleanup removed the single scoped INPUT rule, five remaining
bridge/subnet rules, the test bridge, temporary daemon data, and runtime pointer.
Native SSH/backend fixtures and PostgreSQL fixtures were also removed. Downloaded
Nix tooling and build caches remain available.

No old-instance recovery mutation, main-instance change, seeded-instance update,
live SSH enrollment, commit, push, or GitHub message occurred in this continuation.
The feature remains disabled in shipped profiles. CI, semantic PR review, and
publication of these local changes are separate delivery steps.
