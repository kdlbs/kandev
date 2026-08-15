---
id: "01-design-package"
title: "Design package: manifest capabilities, secret/health-poll pattern"
status: completed
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 01: Design package

## Intent

Confirm the plugin needs no new host contracts (all required seams already shipped in
PR #2117), and settle the plugin-internal design decisions that have no host
precedent to copy verbatim: manifest capability declarations, the workspace-scoped
secret key-composition scheme, and the health-poll interval/backoff shape. Produce
this spec and plan; no ADR is required in the host repo since no host-side contract
or convention changes — the "Redmine ships as a plugin" decision itself is recorded
as a project memory, not a new ADR, following the same precedent as the Bitbucket
pivot.

## Owned paths

- `docs/specs/redmine-plugin/spec.md` (host repo)
- `docs/plans/redmine-plugin/plan.md` + this file (host repo)
- `docs/specs/INDEX.md` (host repo, index row)

## Dependencies

None.

## Acceptance

1. The spec's "API and host contracts" section names only already-shipped seams; no
   new host RPC, manifest field, or frontend registry kind is proposed.
2. The plugin's manifest will declare exactly the capabilities it uses:
   `api_read:tasks`, `api_write:tasks`, `state`, `secrets` — and no capability it does
   not exercise (least privilege, per the create-kandev-plugin skill).
3. The secret key-composition scheme (`redmine:<workspace_id>:api_key`, encrypted with
   workspace-derived key material before calling `SetSecret`) and the health-poll
   convention (~90s interval, jitter, backoff, stored in `plugin_state`) are documented
   in the spec before task 03 implements them.

## Verification

```sh
# Docs-only task; verify structure and links resolve.
test -f docs/specs/redmine-plugin/spec.md
test -f docs/plans/redmine-plugin/plan.md
grep -q "redmine-plugin" docs/specs/INDEX.md
```

## Risks

Low — this is a documentation task. The main risk is under-scoping: if a genuinely new
host capability turns out to be needed once task 03+ starts real implementation, that
discovery must come back here as a plan update, not be worked around silently in the
plugin repo.
