---
id: "02-session-mode-evidence"
title: "Prove permission scope and provider behavior"
status: in_progress
wave: 2
depends_on:
  - "01-protocol-mode-delivery"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-002
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-005
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-007
acceptance_criteria:
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-002.10
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.1
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-005.3
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.2
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.4
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.5
  - AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-007.8
system_design:
  - ../../specs/agents/system-design/agent-permission-control-integrity.md
---

# Prove permission scope and provider behavior

## Outcome and scope

Prove that task modes do not change shared settings. Distinguish protocol state
from actual permission enforcement.

- Extend adapter fixtures with the pinned Claude bridge response shapes.
- Add a reproducible isolated real-provider harness under
  `apps/backend/internal/agent/runtime/lifecycle/`, using the existing gated E2E pattern.
  Pin bridge `0.81.2` and record SDK version, runtime identity, and executor.
- Compare default and permissive modes on the same disposable Git repository.
  Use ACP mode control before the prompt. Disable Kandev automatic approval so
  the test measures provider mode enforcement independently.
- Record permission frames, actual Git state, and settings hashes before/after.
  Keep credentials out of logs. Never use the developer's active settings files.
- Cover concurrent sessions, reset, resume, missing capabilities, and denied
  bypass. Include existing profiles and an old isolated session settings file.
- Preserve existing UI behavior through the permission workflow E2E test.
  Update public permission documentation through the docs-maintainer skill.

## Acceptance

1. No default mode path modifies shared settings or redirects authentication.
2. The real provider permits the intended command in permissive mode and gates
   it in default mode. A provider refusal remains an explicit unresolved result.
3. Missing provider access is reported as incomplete evidence, never a pass.

## Likely files

- ACP adapter regression tests beside `adapter_session.go`.
- Lifecycle executor and session tests beside the changed production files.
- `apps/backend/internal/agent/runtime/lifecycle/permission_mode_e2e_test.go` (new).
- `apps/web/e2e/tests/chat/agent-permission-unattended.spec.ts`.
- Public permission documentation located through `/docs-maintainer`.

## Verification

The new real-provider test must use this name and opt-in gate:

```bash
cd apps/backend
KANDEV_PERMISSION_MODE_E2E=1 go test ./internal/agent/runtime/lifecycle -run '^TestPermissionModeE2E$' -count=1 -v
```

Run it only with disposable fixtures and separately supplied test credentials.
No access to normal user settings is permitted. Provisioning details belong in
the test header and must be reproducible before the work order is complete.

The existing Kandev workflow check runs from the repository root:

```bash
cd apps/web
pnpm e2e:run e2e/tests/chat/agent-permission-unattended.spec.ts
```

## Exclusions

No production account changes, global settings fallback, or claim that a mock
agent proves the behavior of real Claude.

## Results

The isolated provider harness and workflow coverage are implemented. The
provider harness now uses a real Git mutation (`git commit --allow-empty`) on a
disposable repository. In default mode, the permission handler records the
pending request and waits. When provider credentials are available, the test
checks that `HEAD` stays unchanged, then selects the offered `allow_once`
option only after it observes the request. It runs the same command in bypass
mode and checks that the commit succeeds without a permission request. Its
assertions verify the commit objects, refs, and unchanged isolated Claude
settings hash. Automatic approval remains off.

The mock permission workflow passed both cases with:

```bash
cd apps/web
pnpm e2e:run e2e/tests/chat/agent-permission-unattended.spec.ts
```

The gated Claude harness was compiled and invoked with its opt-in flag on the
reconciled local tree. It skipped because the separately supplied credential
`KANDEV_PERMISSION_MODE_E2E_ANTHROPIC_API_KEY` is unavailable. It uses isolated
home, config, npm paths, and a disposable Git repository, and never reads or
changes normal user settings. No real-provider result is claimed; the original
Git refusal remains unverified.

Public documentation validation passed: 62 validator tests and all 47 public
pages. The work order remains in progress until a separately provisioned test
credential permits the real-Claude acceptance run. Delivery status and exact-head CI evidence are recorded in the Kandev task plan.
