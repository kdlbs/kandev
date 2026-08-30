---
id: "01-design-package"
title: "Design package and task files"
status: completed
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 01: Design package and task files

## Intent

Materialize approved Bitbucket connector behavior, generic host boundaries, ADRs, and
dependency-scoped implementation packets without touching production code.

## Dependencies

None.

## Owned paths

- `docs/specs/integrations/requirements/bitbucket-plugin.md`
- `docs/specs/INDEX.md`
- `docs/specs/plugins/requirements/plugins.md`
- `docs/decisions/2026-07-31-authenticated-plugin-actions.md`
- `docs/decisions/2026-07-31-plugin-repository-provider-extensions.md`
- `docs/decisions/2026-07-31-provider-neutral-git-credential-broker.md`
- `docs/decisions/INDEX.md`
- `docs/plans/bitbucket-plugin/`
- `docs/plans/plugins/GRPC-CONTRACT.md`
- `docs/plans/plugins/PLUGIN-API.md`
- `docs/plans/plugins/HOST-DATA-API.proto`

## Acceptance

1. Spec preserves Cloud+DC, API token/PAT+OAuth, full supported-capability parity,
   plugin-first boundaries, native Link/review hooks, and composer submission
   reauthorization.
2. Three ADRs and frozen contracts define generic reusable seams without Bitbucket API
   payloads in the host.
3. `task-01` through `task-13` declare dependencies, owned paths, acceptance, exact
   checks, and risks; indexes and relative links resolve.

## Verification

```sh
git diff --check
rg -n 'Link → Bitbucket Pull Request|submit-time|reauthoriz|Cloud|Data Center' docs/specs/bitbucket-plugin docs/plans/bitbucket-plugin
```

## Risks

Frozen contract drift can make later host/plugin tasks incompatible. Reconcile every
contract amendment with the approved parent plan before implementation starts.
