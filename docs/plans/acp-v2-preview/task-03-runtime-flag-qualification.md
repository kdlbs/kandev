---
id: 03-runtime-flag-qualification
title: Gate and qualify the local ACP v2 pilot
status: done
wave: 3
depends_on:
  - 02-agentctl-session-lifecycle
plan: plan.md
requirements:
  - REQ-AGENTS-ACP-V2-001
  - REQ-AGENTS-ACP-V2-002
  - REQ-AGENTS-ACP-V2-003
acceptance_criteria:
  - AC-AGENTS-ACP-V2-001.1
  - AC-AGENTS-ACP-V2-001.2
  - AC-AGENTS-ACP-V2-001.3
  - AC-AGENTS-ACP-V2-002.1
  - AC-AGENTS-ACP-V2-002.2
  - AC-AGENTS-ACP-V2-003.1
  - AC-AGENTS-ACP-V2-003.2
  - AC-AGENTS-ACP-V2-003.3
system_design:
  - ../../specs/agents/system-design/acp-v2-preview.md
---

# Gate and qualify the local ACP v2 pilot

Expose the preview flag through the runtime registry. Qualify the path from
Kandev's local structured chat to the real provider bridge.

## Scope

- Add `features.acpV2` with all profile defaults off.
  Carry the effective launch preference through the local instance contract.
- Exercise both versions, the fresh-process fallback, and the negotiated
  version in authenticated instance information.
- Add a scripted v2 peer to the mock-agent harness for the task chat smoke.
  Verify both real bridges with the loopback fixture through agentctl.
- Document the flag, supported profiles, and rollback in the relevant runtime
  documentation. Container and remote qualification can follow the pilot.

## ASCII UI preview

UI-01. See the [plan preview](plan.md#ascii-ui-preview). The same route and
row appear on desktop and phone. The switch starts off and uses the existing
restart affordance.

```text
ACP v2 preview                       [ Off ]
Use ACP v2 for supported local agents.
```

## Acceptance

1. The flag starts off. The settings row uses current admin, env-lock,
   and restart rules. Ineligible task and utility paths use v1.
2. An eligible launch can use v2. A legacy selection starts one fresh v1
   process before session setup. Other initialization failures propagate.
3. The Kandev task smoke covers a turn and cancel. Rollback opens the same
   saved ID over v1 and fences new prompts to a retained v2 process.

## Verification

From `kdlbs-kandev/apps/backend`:

```bash
GOWORK=off go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles ./internal/agent/runtime/lifecycle ./internal/agentctl/server/instance -count=1
```

Run the process suite from [work order 02](task-02-agentctl-session-lifecycle.md).

From `kdlbs-kandev/apps`:

```bash
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts
```

Use these commands to build the browser test artifacts in `kdlbs-kandev`.

```bash
GOWORK=off make -C apps/backend build-kandev build-agentctl build-mock-agent build-e2e-delivery-fixture build-e2e-send-now-fixture e2e-plugin-package
pnpm --dir apps --filter @kandev/web build:e2e
```

Run the browser and frontend checks from `kdlbs-kandev/apps/web`.

```bash
GOWORK=off pnpm e2e:run --host --no-build --project chromium -- e2e/tests/chat/acp-v2-preview.spec.ts e2e/tests/settings/feature-toggles-restart.spec.ts --retries=0
GOWORK=off pnpm e2e:run --host --no-build --project mobile-chrome -- e2e/tests/settings/mobile-feature-toggles-restart.spec.ts --retries=0
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
```

Run the lint, documentation, and diff checks from `kdlbs-kandev`.

```bash
GOWORK=off bash scripts/lint-go-changed \
  apps/backend/internal/agent/runtime/lifecycle/manager_events.go \
  apps/backend/internal/agent/runtime/lifecycle/manager_message_snapshot_test.go \
  apps/backend/internal/agentctl/server/adapter/adapter.go \
  apps/backend/internal/agentctl/server/api/agent.go \
  apps/backend/internal/agentctl/server/api/agent_initialize_protocol_test.go \
  apps/backend/internal/agentctl/server/process/acp_preview.go
mapfile -t pr_go_paths < <(git diff --name-only --diff-filter=ACM origin/main...HEAD -- 'apps/backend/**/*.go')
GOWORK=off GOMEMLIMIT=5GiB GOMAXPROCS=2 GOFLAGS=-trimpath bash scripts/lint-go-changed "${pr_go_paths[@]}"
node --test scripts/backend-lint-workflow.test.mjs
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/external-runner-workflow-contract_test.py
python3 .github/scripts/ci-gates_test.py
python3 .github/scripts/lint-action-pinning_test.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check origin/main
```

## Files and dependencies

Likely paths include `profiles.yaml`, `apps/backend/internal/runtimeflags/`,
`apps/backend/internal/common/config/`, the standalone executor and agentctl
instance configuration, and `apps/web/lib/state/slices/features/types.ts`.
Use the existing feature settings catalog for all supported locales.

## Results

- Runtime flag precedence, configurations, and shipped profile defaults passed.
- The process fixture verified fresh v1 fallback, malformed startup failure,
  disabled launch, and a successful v2 selection. Lifecycle tests verified
  admission and the retained-process prompt fence after rollback.
- The lifecycle, instance, and process suites covered the default ACP v1 path
  and the preview, using the SDK revision in
  [work order 02](task-02-agentctl-session-lifecycle.md).
- Both qualified releases of each provider bridge completed a turn, replayed
  without duplication, and reopened the saved ID over v1 through agentctl
  with the loopback model fixture.
- Desktop Playwright tests covered ACP v1 prompts, cancellation, and restart
  resume. All six Chromium qualification tests passed. The preview chat
  streamed a reply before cancellation and restored the same native ID over
  v2 after restart. It reopened the same ID over v1 after rollback.
- Both phone qualification tests passed. The desktop and phone tests confirm
  that the saved opt-in survives a reload and still requires a restart.
- `pnpm install --frozen-lockfile` completed with the existing lockfile.
- All ten frontend feature contract tests passed.
- The backend and fixture builds passed.
- `pnpm --dir apps --filter @kandev/web build:e2e` passed.
- `pnpm run typecheck` passed.
- `pnpm run lint` passed.
- `pnpm run i18n:check` passed.
- The backend review fixes passed Go lint in the commit hook. A scoped run
  for all 77 changed Go paths passed with the 5 GiB memory target.
- The backend workflow, external runner, CI gate, and action pinning contract
  tests passed. The hosted static check uses larger budgets for cold lint runs.
  See the [Go lint design](../../specs/platform/system-design/go-pre-commit-lint.md#verification-boundaries).
- `python3 scripts/list-docs.py validate` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- All 63 public documentation validator tests passed.
- The public documentation validator checked 56 pages successfully.
- `git diff --check origin/main` passed.
