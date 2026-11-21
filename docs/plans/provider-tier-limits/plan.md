---
status: in_progress
requirements:
  - REQ-COSTS-TIER-LIMITS-001
  - REQ-COSTS-TIER-LIMITS-002
system_design:
  - ../../specs/costs/system-design/provider-tier-limits.md
---

# Provider account tier limits delivery

## Scope and dependency

Deliver the [requirements](../../specs/costs/requirements/provider-tier-limits.md) and [design](../../specs/costs/system-design/provider-tier-limits.md) as PR1 of two sequential changes. Restore the preserved tier-limit implementation on fork main `4830d61604`, repair its lint failures, and preserve monthly resets and paid-model blocks. Dependency fork PR #272 is already included in the base. PR2 follows PR1 merge and adds account/model/shared-group scopes, measured catalog values and scope-specific learned replacements. PR1 does not complete that owner outcome. PR1 merged as fork PR #302. PR2 delivers REQ-COSTS-TIER-LIMITS-002 on fork main `2829183ecf`.

| Work order                                                            | Status      | Dependency            |
| --------------------------------------------------------------------- | ----------- | --------------------- |
| [Account limits and settings](task-01-account-limits-and-settings.md) | in progress | 85942d927e40 (P2 usage base; original 07fa2d5e7) |
| [Model and group scopes](task-02-model-and-group-scopes.md)           | in_progress | PR #302               |

## ASCII UI preview

UI-01: Settings > Agents > Provider limits. The page owns scrolling.

```text
Desktop
Account and plan metadata
Free                 Subscription              Paid
Concurrent / RPM     Concurrent / RPM           Concurrent / RPM
Daily / short window 5h / week / month          Day / month spend
Default / learned / owner override on each allowance
Free: account-wide daily, then model and group scopes
  scope | models | today / allowance | reset | source | provenance
  [ Add model scope ] [ Add group scope ]
Profile tier mapping
Save / Clear overrides / status

Phone
Account and plan metadata
[ Free | Subscription | Paid ]
One focused tier editor (scopes stacked under the account-wide daily)
Profile tier mapping
Save / Clear overrides / status
```

Empty values show unknown or inherited values. Load/save failures use existing localized status patterns. Phone/coarse-pointer controls are 44px; ordinary desktop controls are 28px. Draft state survives tier and viewport changes.

## Verification and risks

Use source-adjacent companion units and existing focused package tests. Committed unit tests in `agent/usage` (`tier_limits_test.go`) and `backendapp` (`provider_tier_limits_test.go`) cover allowance boundaries, owner/learned/default merge precedence, model and group scope counts, reset windows, candidate admission and the per-model request count. Capture the real settings components with synthetic data in a disposable component preview. No provider or operating-runtime call is needed. Request estimates cannot enforce every HTTP request inside an already running turn. Unknown subscription windows keep their recovery ladder and visibly provisional learning.

## Delivery boundary

Costs owns usage and limit facts; Agents owns admission and recovery. Keep candidate order, admitted turns, settings permissions and the mutation interlock unchanged. Generic window/scope seams are upstream-portable candidates; installation policy and values belong in settings/catalogs, targeting the upstream extension and plugin boundary. The temporary fork-core bundle is D03 because that extension point is not yet available in this fork. Only the designated upstream owner publishes upstream.

The predecessor work order was marked done before its commit hook stopped. Its historical PASS claims are not current evidence. PR1 checks and any limitations are recorded in the work order from this execution. The initial independent review returned six reproduced defects and one migration-order finding that fresh initialization and replay did not reproduce. Review remediation and its targeted verification are recorded in the work order. PR1 merged after independent review and controller authorization. PR2 completion requires installed UI readback and learned-value readback after one naturally occurring classified limit error.
