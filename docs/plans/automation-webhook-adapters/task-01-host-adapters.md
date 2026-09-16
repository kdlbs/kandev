---
id: automation-webhook-host
title: Signed webhook host adapters and review corrections
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-001
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-002
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-003
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-004
system_design:
  - ../../specs/plugins/system-design/automation-webhook-adapters.md
acceptance_criteria:
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.1
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.2
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.3
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.4
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.5
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.6
  - AC-PLUGINS-AUTOMATION-WEBHOOK-001.7
  - AC-PLUGINS-AUTOMATION-WEBHOOK-002.1
  - AC-PLUGINS-AUTOMATION-WEBHOOK-002.2
  - AC-PLUGINS-AUTOMATION-WEBHOOK-002.3
  - AC-PLUGINS-AUTOMATION-WEBHOOK-002.4
  - AC-PLUGINS-AUTOMATION-WEBHOOK-002.5
  - AC-PLUGINS-AUTOMATION-WEBHOOK-003.1
  - AC-PLUGINS-AUTOMATION-WEBHOOK-003.2
  - AC-PLUGINS-AUTOMATION-WEBHOOK-003.3
  - AC-PLUGINS-AUTOMATION-WEBHOOK-003.4
  - AC-PLUGINS-AUTOMATION-WEBHOOK-003.5
  - AC-PLUGINS-AUTOMATION-WEBHOOK-003.6
  - AC-PLUGINS-AUTOMATION-WEBHOOK-004.1
  - AC-PLUGINS-AUTOMATION-WEBHOOK-004.2
  - AC-PLUGINS-AUTOMATION-WEBHOOK-004.3
  - AC-PLUGINS-AUTOMATION-WEBHOOK-004.4
  - AC-PLUGINS-AUTOMATION-WEBHOOK-004.5
---

# Host adapters and verified delivery

## Summary and scope

Implement the optional manifest/SDK/RPC contract, workspace condition discovery,
vault-backed bindings, signed raw-body verification, durable receipts, task
admission/claim, native editor, diagnostics, localization, and public guidance.
Correct the verified review findings in PR #3692. This work order covers the
existing host implementation and its review fixes, not a provider implementation.

## Exclusions

No companion plugin commit or push. No live Bitbucket delivery without a
disposable repository and reachable host. No change to generic webhook auth.

## Acceptance

1. Host-owned authority fences edits, credential changes and installation changes;
   invalid signatures cannot admit work and replay cannot create duplicate tasks.
2. Retries are bounded and fair; deletion/revocation release unclaimed admission
   slots atomically; schedules cannot bypass the verified-event requirement.
3. Desktop and phone configure the same workspace condition snapshot, with
   backend-origin URLs, guarded operations, validated defaults, and correct errors.

## ASCII UI preview

UI-01: Automation editor, saved plugin condition. Structural requirements map to
AC-001.4 and AC-001.7; spacing and wording below are illustrative.

```text
Watch for  [ Provider condition v ]
Repository [ workspace/repository ]
Event      [ push v ]
[Configure webhook] [Reveal secret] [Rotate] [Revoke]
Webhook URL [ backend-origin/api/... ] [Copy]
[Recent deliveries]
```

Phone: the picker opens in a bottom drawer below 768px; fields remain full width
and actions wrap with touch targets. Desktop retains the popover and compact
controls. During initial binding lookup, operations are disabled. Failures use
the existing localized error surface. No signing secret appears until revealed.

## Likely files

- apps/backend/internal/automation/
- apps/backend/internal/plugins/ and apps/backend/pkg/pluginsdk/
- apps/backend/proto/kandev/plugin/v1/
- apps/backend/internal/orchestrator/ and apps/backend/internal/backendapp/
- apps/backend/internal/auth/httpmw/ and apps/backend/cmd/plugin-fixture/
- apps/web/components/automations/, hooks/, lib/api/, lib/state/, lib/types/
- apps/web/e2e/tests/plugins/ and apps/web/src/locales/
- docs/public/plugins-manifest.md and docs/decisions/

## Dependencies and risks

See the [plan](plan.md). Foreign keys must be enabled in cascade regression tests.
Already claimed task execution must not be retried on recovery.

## Verification

```bash
cd apps/backend
go test ./internal/automation ./internal/plugins/... ./internal/auth/httpmw ./pkg/pluginsdk
go test -race ./internal/automation -run TestPluginWebhook
cd ../web
corepack pnpm exec tsc --noEmit --incremental false
corepack pnpm exec vitest run hooks/use-automation-trigger-types.test.ts hooks/use-plugin-webhook-controls.test.ts components/automations/plugin-condition.test.ts components/automations/triggers-section.test.tsx components/automations/trigger-configs/plugin-webhook-controls.test.tsx
corepack pnpm e2e:raw tests/plugins/automation-webhook.spec.ts tests/plugins/mobile-automation-webhook.spec.ts --project=chromium --project=mobile-chrome
cd ../..
python3 scripts/lint-spec-files.py --all
```

Build current backend, web and fixture artifacts before E2E; run changed-code
Go lint and frontend ESLint/Prettier/localization checks. Record exact outcomes below.

## Results

Host corrections completed on 2026-09-15. Prior implementation validation remains
recorded in PR #3692. Corrective verification:

- Affected automation, plugin host, manifest/package, authentication middleware,
  and SDK Go suites passed. Final automation/manifest rerun passed after the
  retry index and trigger policy refinements.
- All `TestPluginWebhook` race checks passed, including concurrent claims,
  binding revocation/trigger deletion, generation changes, bounded retry,
  queue fairness, and receipt recovery coverage.
- TypeScript, changed-file ESLint with zero warnings, Prettier, and localization
  checks passed. Ten focused Vitest tests across five files passed, including
  out-of-order metadata, shared enumeration, binding operation ordering,
  backend URL construction, and resolved clipboard failure.
- Current backend, web and fixture-package artifacts built. Three Playwright
  scenarios passed: Chromium desktop, mobile Chromium at 393px, and mobile
  Chromium at 700px. Each exercised signed acceptance, unsigned rejection,
  duplicate suppression, task creation, and revocation. Phone tests assert
  touch emulation and the drawer at both widths. Deletion cleanup checks HTTP
  success. The initial artifact-freshness guard required rebuilding the fixture
  after the last backend change; the final run passed in 35.7 seconds.
- Changed-code golangci-lint reported zero issues. Specification lint passed.
  The repository's `validateCoverage` checker returned `covered` with no errors
  for the complete host PR plus this work order.

The full `make test-e2e` suite and live Bitbucket delivery were not run. The user
could not identify a disposable provider repository/host pair. This limits live
provider verification, not the completed host fixture acceptance scope. The
companion plugin repository was not modified or pushed during this correction.
