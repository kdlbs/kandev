# ADR-2026-08-12-setup-timeout-owns-launch-budget: One Setup Timeout Owns Launch Budgets

**Status:** accepted
**Date:** 2026-08-12
**Area:** backend

## Context

Session creation used an independent one-minute deadline for a shared execution.
Repository setup could legally run for five minutes, and some remote prepare
paths already allowed ten minutes. A setup that crossed one minute therefore
finished under its own limit but passed an expired context to runtime creation.
The resulting `agentctl not ready: context deadline exceeded` error blamed the
wrong stage.

Operators need one temporary configuration control while the product has
several setup-script implementations. Independent public setup and launch
values could recreate the same invalid ordering.

## Decision

`apps/backend/internal/common/constants` owns one process-start timeout policy.
`KANDEV_TASK_PREPARATION_TIMEOUT` sets the preparation-script limit and defaults
to 10 minutes. The parser accepts positive values supported by
`time.ParseDuration`. Missing, invalid, zero, and negative values use the
default.

All repository and executor prepare paths use that setup value. The internal
agent-launch limit is derived as the setup value plus a fixed five-minute
allowance. Every shared execution-creation path uses the derived limit instead
of defining another deadline.

The environment variable is the only new public setting. No compatibility
alias is needed because this setting has not shipped. Cleanup limits and
runtime-specific fatal or non-fatal setup behavior do not change.

## Consequences

- A setup script can use its documented budget without a shorter shared-launch
  deadline ending the session first.
- One operator value keeps setup and launch limits in a valid order.
- The default maximum shared launch grows to 15 minutes. A truly blocked launch
  therefore takes longer to fail than it did under the accidental one-minute
  limit.
- The fixed five-minute allowance is internal. A future product setting may
  replace this temporary environment-only contract.
- Invalid values fall back safely, but the fallback does not stop backend
  startup.

## Alternatives Considered

### Configure setup and launch separately

Rejected because an operator could set the launch value below the setup value
and restore the original failure.

### Raise only the shared launch deadline

Rejected because setup limits would remain inconsistent across repository,
local, Docker, Sprite, and SSH paths.

### Remove the shared launch deadline

Rejected because a blocked runtime call could hold the activity lease forever.

### Add YAML or Settings UI configuration now

Deferred. One startup environment variable fixes the reported failure with a
small public surface.
