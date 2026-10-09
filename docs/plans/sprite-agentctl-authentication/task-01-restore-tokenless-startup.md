---
id: "01-restore-tokenless-startup"
title: "Restore tokenless agentctl startup"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-AGENTCTL-AUTH-MODES-001
acceptance_criteria:
  - AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.1
  - AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.2
  - AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.3
  - AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.4
  - AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.5
  - AC-EXECUTORS-AGENTCTL-AUTH-MODES-001.6
system_design:
  - ../../specs/executors/system-design/agentctl-authentication-modes.md
---

# Task 01: Restore tokenless agentctl startup

## Summary

Restore the tokenless startup path used by Sprites. Exercise the real agentctl startup factory and prove that authenticated requests still obey credential rotation.

## In scope

- Add `TestAgentctlAuthenticationModes` through the production `run` path in an isolated subprocess.
- Reproduce the tokenless configuration failure before the production correction.
- Attach the rotating source only when startup `Config.AuthToken` is nonempty.
- Cover successful tokenless and authenticated turns with the existing mock-agent fixture.
- Preserve listener policy, static-token behavior, authenticated rotation, and stream invalidation.

## Out of scope

- UI and error classification changes.
- Changes to Sprite provisioning, credentials, recovery, or retained workspaces.
- A new authentication interface, dynamic authentication mode, or feature flag.

## Acceptance

1. The tokenless test fails with 401 before the correction. Afterward, instance creation, configuration, status, streamed output, and completion succeed without credentials.
2. Authenticated startup rejects absent, malformed, and incorrect credentials. Valid bootstrap credentials work, and rotation governs both existing and newly created instances.
3. Existing rotation and listener tests pass. The fixture cleans up every process, listener, and temporary file without accessing live task data.

## TDD sequence

1. Mark this work order `in_progress` and synchronize the plan.
2. Add the production-startup fixture and the tokenless regression.
3. Run the focused command and record the expected 401 failure.
4. Add the startup guard and authenticated cases.
5. Run all verification commands and record their results.
6. Mark this work order `done` only after its checks pass.

The fixture must isolate inherited configuration, bootstrap nonce, parent-pipe, survival, and provider environment values. Use repository subprocess helpers where applicable.

Launch mock-agent from a temporary executable built with `-trimpath`. Do not invoke npm, real agent providers, or the Sprites API.

Exercise the actual control API and real agentctl HTTP/WebSocket client. A manually assembled test router does not satisfy this work order.

Use explicit completion barriers and bounded deadlines. Terminate and wait for owned subprocesses in test cleanup, including after an assertion failure.

## Verification

Run from the repository root. The first command is also the required red check before implementation.

```bash
(cd apps/backend && go test -trimpath ./cmd/agentctl -run '^TestAgentctlAuthenticationModes/tokenless$' -count=1 -timeout=3m)
(cd apps/backend && go test -trimpath -race ./cmd/agentctl ./internal/agentctl/server/api ./internal/agentctl/server/config -count=1 -timeout=10m)
(cd apps/backend && go test -trimpath ./internal/agent/runtime/lifecycle -run '^TestSpritesBuildInstanceResult' -count=1 -timeout=3m)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the local documentation preflight from the repository root. It validates the planned production path and all package references.

```bash
node <<'JS'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = [
  'docs/plans/sprite-agentctl-authentication/plan.md',
  'docs/plans/sprite-agentctl-authentication/task-01-restore-tokenless-startup.md',
  'docs/specs/executors/requirements/agentctl-authentication-modes.md',
  'docs/specs/executors/system-design/agentctl-authentication-modes.md',
];
const result = validateCoverage({
  changedFiles: ['apps/backend/cmd/agentctl/main.go', ...paths],
  fileContents: Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
JS
```

## Files likely touched

- `apps/backend/cmd/agentctl/main.go`
- `apps/backend/cmd/agentctl/authentication_modes_test.go` (new)
- `docs/plans/sprite-agentctl-authentication/plan.md`
- `docs/plans/sprite-agentctl-authentication/task-01-restore-tokenless-startup.md`

Read-only dependencies include `internal/agentctl/server/api/auth.go`, `control_server.go`, and `credential_rotation_stream_test.go`, under `apps/backend`.

## Dependencies

None. All work stays sequential in the primary session.

## Risks

- A fixture that omits production wiring can pass while real Sprite startup fails.
- Inherited runtime configuration can select authenticated mode in the tokenless test.
- A credential-source guard must retain rotation for instances created after the initial process launch.

## Parallelism

`sequential`

## Inputs

- [Authentication requirements](../../specs/executors/requirements/agentctl-authentication-modes.md)
- [Authentication design](../../specs/executors/system-design/agentctl-authentication-modes.md)
- [Investigation evidence](plan.md#evidence-and-root-cause)
- `.agents/skills/tdd/SKILL.md` and its backend test reference.
- `apps/backend/AGENTS.md` and scoped agentctl guidance.

## Results

Completed on 2026-10-09. `cmd/agentctl/main.go` attaches the rotating credential source only when resolved startup configuration has a nonempty auth token. The new production-startup subprocess regression proves tokenless and authenticated turns, protected-route rejection, and credential rotation across existing and newly created instances.

The required red check failed before the production change with HTTP 401 at agent configuration. The post-fix authentication-mode test, the race-enabled agentctl/API/config packages, `TestSpritesBuildInstanceResult`, standalone agentctl build, spec validation/lint, documentation coverage preflight, and `git diff --check` all passed. The fixture used temporary ports, workspaces, logs, and a locally built mock-agent; it did not contact a live Sprite or provider.

Review remediation: subprocess stdout and stderr now use synchronized snapshots, and fixture cleanup reports unexpected child exit errors with captured output. A focused helper test waits for readiness, releases a nonzero exit, and verifies cleanup detects it. `go test -trimpath -race ./cmd/agentctl -run '^TestAgentctlAuthenticationModes' -count=1 -timeout=5m` passed.
