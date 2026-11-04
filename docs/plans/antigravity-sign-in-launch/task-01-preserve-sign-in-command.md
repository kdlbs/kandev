---
id: "01-preserve-sign-in-command"
title: "Preserve the complete sign-in command"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ANTIGRAVITY-ACP-003
  - REQ-AGENTS-ANTIGRAVITY-ACP-009
  - REQ-AGENTS-ANTIGRAVITY-ACP-010
acceptance_criteria:
  - AC-AGENTS-ANTIGRAVITY-ACP-003.1
  - AC-AGENTS-ANTIGRAVITY-ACP-003.2
  - AC-AGENTS-ANTIGRAVITY-ACP-003.5
  - AC-AGENTS-ANTIGRAVITY-ACP-009.1
  - AC-AGENTS-ANTIGRAVITY-ACP-009.2
  - AC-AGENTS-ANTIGRAVITY-ACP-009.3
  - AC-AGENTS-ANTIGRAVITY-ACP-010.2
  - AC-AGENTS-ANTIGRAVITY-ACP-010.4
system_design:
  - ../../specs/agents/system-design/antigravity-acp-agent.md
---

# Task 01: Preserve the complete sign-in command

## Summary

Keep the resolved executable in the structured argument array sent by the Antigravity sign-in owner.
Add a failing regression through the real configure HTTP handler before changing the caller.
Preserve the existing standalone server, companion harness, subscription policy, and generic command-validation contract.

## In scope

- Add `TestAntigravityLoginConfigurePreservesCommand` in `antigravity_login_command_test.go`.
- Call `configureAntigravityLoginInstance` with the real HTTP client and real agentctl configure handler in an isolated test server.
- Inject deterministic commands through a test `LocalRuntimeAgent`; use the real Antigravity metadata.
- Cover Linux `--uid=`, executable-only commands, paths with spaces, and Windows-style path characters.
- Assert exact receiver argv and the paired `ANTIGRAVITY_HARNESS_PATH` value.
- Intercept start with a distinctive test marker after configure succeeds. Start no native process, stream, or Google authentication.
- Change the sign-in caller from `args[1:]` to the complete `args` slice.
- Run existing client/API, installer/resolver, and host launch-policy tests.
- Record results in this work order and its plan, then synchronize the linked original Task 03 and Task 07 notes.

## Out of scope

Installer code changes, `agy` installation, a bridge package, runtime migration, generic API changes, UI changes, and live Google qualification.
The original package retains its pending browser tests and native qualification work.

## Acceptance

1. The new regression fails before the correction at configure, then reaches the controlled start marker after the correction.
2. Every table case preserves the full command and harness value. Existing argument rejection and fallback tests pass unchanged.
3. Required checks pass with recorded commands. Companion records keep browser and native qualification gates open.

## Implementation details

Use `api.NewServer(...).Router()` and `process.NewManager` with a temporary `InstanceConfig` as the configure receiver.
Wrap the router only to advertise `antigravity_personal_auth_v1` at identity and intercept the start request.
Keep configure JSON decoding, structured-array precedence, and validation on the production path.
An HTTP stub that returns success for every configure request is insufficient.
An arbitrary later network error is also insufficient: assert the specific controlled start marker.

Use temporary fixtures, bounded contexts, immediate cleanup registration, and channels for request observation.
Do not read the developer's runtime cache, home, CLI, or Google credentials.
Platform-shaped argument literals test serialization; they do not execute or inspect foreign filesystem paths.

Keep the existing command string, resolver, empty-command guard, capability negotiation, and environment merge.
Change only the structured argument passed by `configureAntigravityLoginInstance`.
Do not split a joined command, weaken `ValidateCommandArgs`, or prepend executables in the generic client.

## Verification

Run from the repository root.
The regression was added first and the first command recorded the red result.
After the caller correction, the complete block recorded the green result and focused compatibility checks.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility -run '^TestAntigravityLoginConfigurePreservesCommand$' -count=1 -v)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/hostutility -run '^TestAntigravity' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/agentctl ./internal/agentctl/server/api -run '^Test(ConfigureAgent_|HandleAgentConfigure_)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/antigravityruntime -run '^Test(InstallAntigravityBundle|ResolveAntigravityBundle)$' -count=1)
bash scripts/lint-go-changed apps/backend/internal/agent/hostutility/antigravity_login.go apps/backend/internal/agent/hostutility/antigravity_login_command_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const files = [
  'docs/plans/antigravity-sign-in-launch/plan.md',
  'docs/plans/antigravity-sign-in-launch/task-01-preserve-sign-in-command.md',
  'docs/specs/agents/requirements/antigravity-acp-agent.md',
  'docs/specs/agents/system-design/antigravity-acp-agent.md',
];
const fileContents = Object.fromEntries(files.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [
  { filename: 'apps/backend/internal/agent/hostutility/antigravity_login.go', status: 'modified' },
  { filename: files[1], status: 'modified' },
];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ ok: result.ok, status: result.status, errors: result.errors }));
if (!result.ok) process.exitCode = 1;
NODE
git diff --check
```

If implementation changes another Go file, include it in the changed-Go lint command and record the actual invocation.
Do not run browser tests against the user's live instance.
No new Playwright test, pnpm bootstrap, real consent, or subscription request is required for this backend caller repair.

## Files likely touched

- `apps/backend/internal/agent/hostutility/antigravity_login.go`.
- `apps/backend/internal/agent/hostutility/antigravity_login_command_test.go` (new).
- `docs/plans/antigravity-sign-in-launch/plan.md` and this work order: status and results.
- `docs/plans/antigravity-local-subscription/plan.md`: linked repair evidence.
- `docs/plans/antigravity-local-subscription/task-03-google-sign-in.md`: repair evidence, with the work order still open.
- `docs/plans/antigravity-local-subscription/task-07-native-qualification.md`: repaired startup prerequisite, with native gates still blocked.

Read these existing files without changing their shared contracts:

- `apps/backend/internal/agent/runtime/agentctl/client.go` and `agent_test.go`.
- `apps/backend/internal/agentctl/server/api/server.go` and `agent_test.go`.
- `apps/backend/internal/agentctl/server/process/manager.go` and `apps/backend/internal/agentctl/server/config/config.go`.
- `apps/backend/internal/agent/hostutility/antigravity_launch_test.go` and `antigravity_login_test.go`.

## Dependencies

None within this fix package.
Existing runtime selection and authentication infrastructure is present at the diagnosed baseline.
Original native qualification depends on this repair, not the reverse.

## Risks

- Native startup or Google authentication can still fail after the configure defect is removed.
- A configure-only test cannot close original Task 07 or prove models are available.
- Caller and receiver fixtures can hide the defect if both duplicate the same incorrect argument assumption.

## Parallelism

`sequential`

## Inputs

- [Requirement document](../../specs/agents/requirements/antigravity-acp-agent.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/antigravity-acp-agent.md), Explicit local sign-in, Local runtime resolution, and Installation.
- [Fix plan](plan.md), root cause, compatibility matrix, and companion-package boundaries.
- `apps/backend/AGENTS.md`, `apps/backend/internal/agentctl/AGENTS.md`, and `apps/backend/internal/agentctl/server/api/AGENTS.md`.
- `.agents/skills/tdd/references/backend-tests.md`.

## Results

Done.

Production change:
`configureAntigravityLoginInstance` in `apps/backend/internal/agent/hostutility/antigravity_login.go` now passes the complete `command.Args()` slice to `Client.ConfigureAgent` instead of `args[1:]`.
The existing resolution, empty-command guard, capability check, environment merge, and start path are unchanged.

Permanent regression:
`TestAntigravityLoginConfigurePreservesCommand` in `apps/backend/internal/agent/hostutility/antigravity_login_command_test.go` drives the real sign-in caller through the real agentctl client, the production `/api/v1/agent/configure` handler, `process.Manager`, and a controlled start interception.
It covers Linux `--uid=`, an executable-only command, an executable path containing spaces, and a Windows-shaped executable.
Each case asserts the received `InstanceConfig.AgentCommand`, `InstanceConfig.AgentArgs`, the paired `ANTIGRAVITY_HARNESS_PATH`, the inherited fixture environment, and the start marker.
No vendor executable, installed CLI, account, credential store, or external network is used.
No native process or agent stream starts.

Red before the correction:

```text
linux standalone server with uid flag: configure request failed with status 500: {"success":false,"error":"agent command executable is invalid"}
executable only server: configure request failed with status 500: {"success":false,"error":"agent command cannot be empty"}
executable path with spaces: configure request failed with status 500: {"success":false,"error":"agent command executable is invalid"}
windows shaped command: configure request failed with status 500: {"success":false,"error":"agent command executable is invalid"}
```

Green after the correction: all four cases reached the controlled start marker with the complete argv and harness value.
Exit status was zero and the command ended with `PASS`.

Recorded verification on November 4, from the repository root:

```text
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/hostutility -run '^TestAntigravityLoginConfigurePreservesCommand$' -count=1 -v)
  PASS
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/hostutility -run '^TestAntigravity' -count=1)
  ok github.com/kandev/kandev/internal/agent/hostutility 1.093s
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/runtime/agentctl ./internal/agentctl/server/api -run '^Test(ConfigureAgent_|HandleAgentConfigure_)' -count=1)
  ok github.com/kandev/kandev/internal/agent/runtime/agentctl 0.066s
  ok github.com/kandev/kandev/internal/agentctl/server/api 0.029s
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/antigravityruntime -run '^Test(InstallAntigravityBundle|ResolveAntigravityBundle)$' -count=1)
  ok github.com/kandev/kandev/internal/agent/antigravityruntime 0.043s
bash scripts/lint-go-changed apps/backend/internal/agent/hostutility/antigravity_login.go apps/backend/internal/agent/hostutility/antigravity_login_command_test.go
  0 issues.
python3 scripts/list-docs.py validate
  Validated 431 decisions and 1915 specifications.
python3 scripts/lint-spec-files.py --all
  All specification files passed.
node coverage preflight (.github/scripts/pr-docs.cjs)
  {"ok":true,"status":"covered","errors":[]}
git diff --check
  passed
```

The linked original Task 03 and Task 07 notes now record this repair as implemented.
Their browser setup specs and live native/Google qualification remain open.
This work order does not promote the broader requirement or system design from draft.
