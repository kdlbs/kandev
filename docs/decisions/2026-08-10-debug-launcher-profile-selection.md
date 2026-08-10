# ADR-2026-08-10: Keep Debug Launches on the Production Profile

**Status:** accepted
**Date:** 2026-08-10
**Area:** backend, frontend, cli

## Context

The native `--debug` launcher sets `KANDEV_DEBUG_PPROF_ENABLED=true` and
`KANDEV_DEBUG_AGENT_MESSAGES=true`. Profile detection also treated the pprof
variable as the development-profile selector. As a result, `make start-debug`
received development defaults and the `Dev Kandev` browser title.

The debug launcher needs diagnostic behavior without development-only defaults.
The source-checkout `make dev` path already has a canonical development
selector, `KANDEV_DEBUG_DEV_MODE=true`.

## Decision

Use only `KANDEV_DEBUG_DEV_MODE=true` as the development-profile selector.
Keep `KANDEV_DEBUG_PPROF_ENABLED` as a legacy behavior variable for pprof and
related debug compatibility. It does not select the `dev` profile.

The launch contracts are:

- `make dev` selects `dev` and uses `Dev Kandev`.
- `make start-debug` enables debug diagnostics, selects `prod`, and uses
  `Debug Kandev` by default. An explicit title prefix still wins.
- `KANDEV_E2E_MOCK=true` keeps its existing priority and selects `e2e`.

The browser title continues to use `KANDEV_WEB_TITLE_PREFIX`. The debug
launcher supplies `Debug` as its default prefix without adding a new profile.

## Consequences

Debugging a release-style build no longer enables development feature defaults
or the development browser identity. It has the distinct `Debug Kandev` title
while retaining production defaults. Contributors who need the development
profile use `make dev` or set `KANDEV_DEBUG_DEV_MODE=true`.

The legacy pprof variable remains available for older launch scripts. Tests and
launcher checks must keep the selector and debug behavior separate.

## Alternatives Considered

1. Set only an empty title prefix in `make start-debug`. Rejected because it
   fixes the browser label but leaves other development defaults enabled and
   does not identify a diagnostics-only launch.
2. Keep pprof as a profile selector. Rejected because pprof is a diagnostic
   behavior, not a development-environment identity.
3. Remove the pprof variable. Rejected because existing launchers and config
   users depend on the compatibility alias.
