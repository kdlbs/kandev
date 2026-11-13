# Partial publication checkpoint

This document records a draft snapshot, not completed implementation or merge
readiness. Work order 01 is complete. Work order 02 is partial. Work order 03 is
pending. The coordinator requested publication of existing task-owned work and
no additional implementation, review handling, CI waiting, or merge.

## Preservation and base

- Branch: `feature/builtin-mcp-packages-6d1b`. Target: `main`.
- Verified remote base: `cc687bdce96bc22fb8a9ca87abe571b8309d3125`.
- Parent squash: `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`, already in the base.
- Pre-rebase archive and binary patch:
  `/tmp/kandev-builtin-publish-backup-dxqtwx2_/` (145 task-owned paths).
- Retained stash: `52b7708aecc7a3a442edf9f8ccf36014ef5808b4`.
- A normal fetch encountered the unrelated broken shared ref
  `refs/heads/feature/lead-pr-4371-to-merg-eeb`. Remote main was verified with
  `git ls-remote`; its complete commit was already available locally. No shared
  ref repair or change to another workspace was performed.

## Scope still open

Policy-reader and management API activation; automatic and retained prompt
selection; accepted binding/session evidence; queued Walkthrough intent;
remaining injection/adoption edge cases; all workspace/task UI, localization,
phone parity, and public documentation. The enabled test
`TestBuiltinAutoStartUsesFreshOrRetainedSelection` records pending TDD work.
The Office prompt inventory also needs to retain `show_rich_output_kandev`;
`TestSyspromptToolNames_ExactlyMatchMCPOfficeMode` currently fails.
No UI files changed, so browser screenshots and UI E2E are not applicable to this
publication snapshot. They remain required for work order 03.

Publication lint required behavior-preserving extractions for endpoint parsing
and policy reads. The new passthrough stream helper moved out of the existing
oversized file. No pending feature behavior was implemented for publication.

## Validation

The affected run uses `go test -p=1` with `-run 'Builtin|MCP|Mcp|Permission'`
and `-count=1` across the 22 changed Go packages. It uses `GOMAXPROCS=2`,
`GOMEMLIMIT=3GiB`, and the owned socket-only PostgreSQL fixture. The package list
is in `/tmp/kandev-builtin-publish-packages.txt`; output is in
`/tmp/kandev-builtin-publish-tests.log`. The run completed with 19 package checks
passing (one had no matching tests) and three failing. The task repository checks
passed on the owned database fixture, which was stopped after testing.

```bash
cd apps/backend
: "${KANDEV_TEST_POSTGRES_DSN:?Use an owned disposable PostgreSQL fixture}"
GOMAXPROCS=2 GOMEMLIMIT=3GiB go test -p=1 \
  ./internal/agent/mcpconfig \
  ./internal/agent/runtime/agentctl \
  ./internal/agent/runtime/lifecycle \
  ./internal/agent/settings/handlers \
  ./internal/agent/settings/store \
  ./internal/agentctl/server/adapter/transport/acp \
  ./internal/agentctl/server/api \
  ./internal/agentctl/server/config \
  ./internal/agentctl/server/instance \
  ./internal/agentctl/server/process \
  ./internal/agentctl/types \
  ./internal/agentctl/types/streams \
  ./internal/backendapp \
  ./internal/mcp/handlers \
  ./internal/mcp/profile \
  ./internal/mcp/scope \
  ./internal/mcp/server \
  ./internal/orchestrator \
  ./internal/orchestrator/executor \
  ./internal/persistence/requiredstores \
  ./internal/sysprompt \
  ./internal/task/repository/sqlite \
  -run 'Builtin|MCP|Mcp|Permission' -count=1
```

Known failures remain enabled:

- `TestBuiltinAutoStartUsesFreshOrRetainedSelection` (fresh and reset): pending
  automatic-prompt integration advertises unselected Rich Output tools.
- `TestSyspromptToolNames_ExactlyMatchMCPOfficeMode`: the Office prompt inventory
  omits `show_rich_output_kandev` although the server inventory includes it.
- `TestExecNativeMCPCommandRunnerBoundsInheritedPipeAfterCancellation`: the
  existing command fixture cannot find `sleep` on this Nix host, as detailed below.

After the publication lint extractions, this check passed from `apps/backend`:

```bash
go test -p=1 ./internal/agent/mcpconfig ./internal/agent/runtime/lifecycle -run Builtin -count=1
```

The lifecycle `Builtin` check passed again after both constant cleanups.
The broader affected run also passed the executor package after
its policy-read extraction. Documentation catalog validation, full specification
lint, SQL guard (`go run -p=1 ./cmd/sqlguard ./internal`), Go formatting, and
whitespace checks passed. No full backend, fresh full race, installed-provider,
or browser E2E result is claimed for this snapshot.

The first lint attempts encountered another workspace's lock and exceeded the
five-minute package-loading budget. After tagged package-cache preparation and
constant cleanup, the affected-package `golangci-lint run` passed with zero issues
using the same comparison base and checks, a ten-minute manual timeout, and two
workers. The repository's normal commit hooks remain required without bypass.
The publication handoff records their final receipt.

The existing native-command cancellation fixture fixes its child PATH to
`/usr/bin:/bin` and invokes `sleep`. This Nix host has neither `/usr/bin/sleep`
nor `/bin/sleep`. The fixture exits before cancellation and reports a nil error.
Its test and native runner files are unchanged from the publication base.
This environment limitation must remain explicit; no host files or unrelated
fixture code were changed to hide it.
