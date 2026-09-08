---
id: "03-configuration-docs"
title: "Configuration docs"
status: done
wave: 2
depends_on: []
plan: "plan.md"
spec: "../../specs/auth/requirements/trusted-proxies.md"
---

# Task 03: Configuration docs

Document `KANDEV_TRUSTED_PROXIES` in the public startup-configuration
reference.

## Acceptance

- `docs/public/configuration.md` documents the variable: comma-separated IPs /
  CIDRs (IPv4 and IPv6), default (unset = no trusted proxies, header ignored),
  the spoofing security implication (a directly reachable backend whose peer
  falls inside a trusted range can have `X-Forwarded-For` spoofed, which also
  defeats the ClientIP-keyed login rate limiter), invalid-entry behavior
  (startup warning naming the bad value, whole variable ignored, no crash),
  and the note that the variable must reach the backend process: set it in
  the environment of the process that launches kandev (service unit,
  container, or parent launcher env, e.g. a systemd drop-in for
  `kandev.service`). `~/.kandev/supervisor/launch.json` is launcher-generated
  (rewritten every launch, env filtered by an allowlist) and is NOT an
  environment configuration source. The variable is read once at startup.
- Copy follows the env-only variable prose pattern of
  `KANDEV_TASK_PREPARATION_TIMEOUT` in the same file, and the no-em-dash rule
  for public copy (ADR 2026-08-10).

## Files likely touched

- `docs/public/configuration.md` (new `###` section after the Authentication /
  feature-flags section, before Logging)

## Dependencies

None (text describes the Task 01 contract; files are disjoint from Tasks
01-02).

## Parallelism

`sequential` by default; scheduled as Wave 2 after Tasks 01-02. Parallel-safe
candidate only when the user authorizes subagents.

## Inputs

- Spec sections **What**, **Failure modes**; the task description's docs
  requirements.
- `docs/public/configuration.md` structure (`## Complete backend reference`,
  `### Authentication, Office, Plugins, and feature flags`, env-only prose
  pattern in `### Setup and launch timing`).
- Launcher env flow: `apps/backend/internal/launcher/env.go` (`backendEnv`
  starts from `os.Environ()`, so the launcher's own environment reaches the
  backend child) and `apps/backend/internal/launcher/supervisor.go`
  (`prepareSupervisorEnv`/`buildManifest`/`allowedSupervisorEnv`: the
  `launch.json` manifest is rewritten every launch and its env is an
  allowlist that omits `KANDEV_TRUSTED_PROXIES`, so it is not an injection
  point).

## Verification

```bash
node scripts/validate-public-docs.mjs && git diff --check && git diff --stat docs/public/configuration.md
```

Read the rendered section back and confirm the format, default, security
implication, invalid-entry behavior, and reach-the-backend note are all
present; `scripts/validate-public-docs.mjs` enforces the no-em-dash rule for
`docs/public/**` (ADR 2026-08-10).

## Risks

- Adding a YAML-key row to the reference tables would imply a `config.yaml`
  key that does not exist; the variable is environment-only and must be
  documented as prose like `KANDEV_TASK_PREPARATION_TIMEOUT`.

## Output contract

Report the exact files changed, the added section text, diff-check evidence,
remaining risks, and synchronized task/plan status. Record every exact command
and outcome in `## Results`.

## Results

Added the `### Trusted proxies for X-Forwarded-For` section to
`docs/public/configuration.md` between the Authentication / feature-flags
section and Logging: format, default (unset = no trusted proxies), security
implication (callers inside a listed range can spoof; rate-limiter impact),
fail-closed invalid-entry behavior (including trailing/doubled commas), and
reach-the-backend guidance (process-launcher environment, e.g. systemd
drop-in; `launch.json` explicitly noted as launcher-generated and not an
environment source).

- `node scripts/validate-public-docs.mjs` — "Validated 41 published docs pages." (no em dashes; ADR 2026-08-10).
- `git diff --check` — clean.
