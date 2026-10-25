---
status: done
created: 2026-09-15
requirements:
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-001
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-002
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-003
  - REQ-PLUGINS-AUTOMATION-WEBHOOK-004
system_design:
  - ../../specs/plugins/system-design/automation-webhook-adapters.md
---

# Signed automation webhook host delivery

## Scope

Deliver the host adapter contract, verified admission and native configuration
for the [requirements](../../specs/plugins/requirements/automation-webhook-adapters.md).
This records the implementation in PR #3692 and its accepted review corrections.
The companion Bitbucket provider remains a separate repository and release.

## Work orders

- [Host adapter implementation and regression verification](task-01-host-adapters.md), wave 1.

## Dependencies and risks

The host must precede a compatible provider release. Draft bindings require
reconfiguration. Retry exhaustion must not race an execution claim into duplicate
task creation. Existing generic webhooks retain their authentication contract.

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

## Verification

Use the work order commands for backend, frontend, fixture deliveries, and
traceability. A live Bitbucket repository/host pair is not available in this
session; live provider validation remains a separate integration follow-up.

## Completion

The host work order is complete with three signed-delivery browser scenarios
and targeted backend/frontend regression evidence. Live Bitbucket acceptance
remains pending a disposable repository and reachable instance.
