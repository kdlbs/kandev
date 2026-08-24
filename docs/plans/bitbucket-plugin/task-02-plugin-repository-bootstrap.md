---
id: "02-plugin-repository-bootstrap"
title: "Dedicated plugin repository bootstrap"
status: completed
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/requirements/bitbucket-plugin.md"
---

# Task 02: Dedicated plugin repository bootstrap

## Intent

Create public `kdlbs/kandev-plugin-bitbucket` from official plugin template and attach
that repository to its Kandev task. Establish package/CI skeleton only; make no
unsupported product claims.

## Owned paths

- Attached `kdlbs/kandev-plugin-bitbucket` worktree only.
- Template-owned manifest, package, CI, backend, and UI skeleton files in that
  repository.

## Dependencies

None.

## Acceptance

1. Repository is public, template-derived, and attached to the task rather than
   cloned inside this host worktree.
2. Skeleton identifies `kandev-plugin-bitbucket`, ships no OAuth secret, and contains
   no claimed minimum host version before generic seams release.
3. CI/package layout can build the release archive expected by task 13.

## Verification

```sh
make test
make vet
make build
make package
```

## Risks

Template drift can invalidate guessed commands or archive layout. Use the template's
current documented targets; do not add product behavior in this bootstrap task.
