---
id: "07-public-configuration-reference"
title: "Public configuration reference"
status: done
wave: 5
depends_on: ["06-message-queue-configuration-lock"]
plan: "plan.md"
spec: "../../specs/platform/requirements/startup-configuration-parity.md"
---

# Task 07: Public configuration reference

Publish the complete startup configuration contract and synchronize affected
specs and engineering guidance with the implemented behavior.

## Acceptance

- The public configuration guide documents home file discovery, exact file
  order, first-file failure behavior, no merging, precedence, and restart
  requirements.
- The guide lists every stable startup setting with YAML key, environment
  variable, type, default, and security notes where relevant.
- Every published startup environment variable has a YAML equivalent or a
  justified exclusion.
- Secret guidance states that Kandev supports YAML secrets, never logs their
  values, warns on broad Unix read permissions, and recommends mode `0600`.
- CLI and service guides explain that manual and service launches automatically
  use `<KANDEV_HOME_DIR>/config.yaml`.
- The trusted proxy example shows `server.trustedProxies` for the reverse proxy
  case and keeps the spoofing warning.
- The setup timeout and trusted proxy behavioral specs match the new YAML
  contract.
- Root and backend engineering guidance describe the catalog and source owner
  if the implementation creates new architecture boundaries.
- Public documentation validation and link checks pass.

## Files likely touched

- `docs/public/configuration.md`
- `docs/public/cli.md`
- `docs/public/run-as-a-service.md`
- `docs/specs/auth/requirements/trusted-proxies.md`
- `docs/specs/platform/requirements/setup-launch-timeout.md`
- `AGENTS.md`
- `apps/backend/AGENTS.md`
- Any generated public navigation metadata required by the docs system

## Dependencies

Task 06 completes behavior and final public names before the reference is
published.

## Documentation structure

1. Explain the configuration surfaces and source precedence.
2. Show discovery examples for manual, service, and working-directory launches.
3. Publish one canonical key table. Avoid separate partial tables that can
   drift.
4. Group exclusions by internal wiring, generated secrets, test and debug,
   packaging, platform detection, and UI-managed data.
5. Add secure secret and reverse proxy examples.
6. Link affected topic guides back to the canonical reference.

## Verification

```bash
node scripts/validate-public-docs.mjs
git diff --check
```

Also run the configuration catalog completeness test from Task 01 after the
documentation inventory is final.

## Risks

- A hand-maintained documentation table can drift from the catalog. Prefer a
  checked inventory or a test that compares documented identifiers with the
  catalog.
- Examples must not include real tokens, passwords, internal child variables,
  or unsafe broad trusted-proxy ranges.
- Existing topic specs currently describe some settings as environment-only.
  Leaving those statements unchanged would create two sources of truth.

## Output contract

Record validation results, the final inventory counts, synchronized specs and
guides, files changed, and remaining risks in `## Results`.

## Results

- `node scripts/validate-public-docs.mjs` passed and validated 41 published
  documentation pages.
- `git diff --check` is included in the final repository gate.

The public configuration reference now documents the working-directory, home,
and `/etc/kandev` candidate order, first-existing-file failure behavior, no
merging, restart requirements, stable YAML keys and environment aliases,
capacity and agentctl settings, message-queue source locks, YAML secret
handling, owner-only `0600` guidance, and the trusted-proxy example. The CLI
and service guides describe automatic home-file discovery and selected-file
handoff. Trusted-proxy and setup-timeout specs now describe the YAML keys and
environment precedence. Root and backend agent guidance identifies the typed
catalog/source owner and private child contract boundary.

Files changed:

- `docs/public/configuration.md`
- `docs/public/cli.md`
- `docs/public/run-as-a-service.md`
- `docs/specs/auth/requirements/trusted-proxies.md`
- `docs/specs/platform/requirements/setup-launch-timeout.md`
- `AGENTS.md`
- `apps/backend/AGENTS.md`
