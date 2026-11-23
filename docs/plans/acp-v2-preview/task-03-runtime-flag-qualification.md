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
go test -trimpath ./internal/runtimeflags ./internal/common/config ./internal/profiles -count=1
go test -trimpath ./internal/agent/runtime/lifecycle ./internal/agentctl/server/instance ./internal/agentctl/server/process -count=1
```

From `kdlbs-kandev/apps`:

```bash
pnpm install --frozen-lockfile
pnpm --filter @kandev/web test -- lib/state/slices/features/features-contract.test.ts
```

From `kdlbs-kandev/apps/web` after adding the scripted peer and test:

```bash
pnpm e2e:run --host --project chromium -- e2e/tests/chat/acp-v2-preview.spec.ts e2e/tests/settings/feature-toggles-restart.spec.ts
pnpm e2e:run --host --project mobile-chrome -- e2e/tests/settings/mobile-feature-toggles-restart.spec.ts
pnpm run typecheck
pnpm run lint
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
  resume. The preview chat streamed a reply before cancellation and restored
  the same native ID over v2 after restart. It reopened the same ID over v1
  after rollback.
- Phone Playwright tests covered the ACP v2 opt-in and the existing restart
  control.
- The frontend feature contract tests, type check, lint, and locale checks passed.
- Go lint passed for the full change, including new files. Specification and
  public documentation validation passed.
