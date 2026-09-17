# Historical v0.94.0 integration evidence

Completed 2026-09-17. Base: `bf819a0228e742d069c528293d848c985a4d1bd1`
(`v0.94.0`). Original local integrated snapshot:
`b1cd0d2ea03336ce882d782f0f1ea61975db520c`, directly above that release.
That local commit and its backup are not published as this repository's history.
The [private import receipt](publication.md) accounts for the sanitized replacement.

## Integration scope

Resolved 83 merge conflicts while preserving the prototype. Adapted coordinator
queue/session identity, run outcomes/continuation scopes, prompt lifecycle
payloads, session reconciliation, authentication and required-store startup to
v0.94.0. Automation delivery reuses admitted runs and clears task-only overrides.
Shared run/Orchestration schema owners have portable SQLite/PostgreSQL conformance
and role cleanup. Legacy Office navigation/flags remain compatibility scope.

The release's Automation YAML/ZIP format cannot represent a coordinator target;
the implementation explicitly rejects that export instead of changing semantics.

## Checks at the original snapshot

- Affected backend suites passed across orchestration/ownership, Automation,
  shared runs, queue/executor/lifecycle, Office, core task service/repository,
  agentctl, MCP and runtime flags. This was not the entire backend test suite.
- Frontend: 27 files, 206 passing tests, four existing skipped tests; TypeScript
  and changed-file ESLint passed.
- SQLite/PostgreSQL fresh/replay/upgrade conformance passed with race detection;
  PostgreSQL backend startup and SQL guard passed.
- Fresh backend/frontend E2E builds passed. Both coordinator/Automation browser
  specs passed together without retries, covering mobile navigation, feature
  flags, conversation callbacks and scheduled delivery behavior.
- Normal hooks passed: harness, architecture, specifications, gofmt, Go lint,
  Prettier, frontend lint, i18n, sleep guard, public copy and commitlint.
- The earlier source-preservation manifest compared 458 original paths without
  differences. The later scope inventory covers the final 476-path rebased delta;
  these are different comparison boundaries, not conflicting counts.

Raw logs and original source manifests remain in local private state. No live
installation was changed; temporary database/browser fixtures were stopped.

## Limits

Full repository tests, actual live-data migration, central-view implementation,
complete assistant behavior and real-provider quality were not established by
this checkpoint. New code must supply its own affected checks. The exact release
base is retained; the upstream PR target branch still needs maintainer agreement.
