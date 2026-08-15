---
id: "02-plugin-repository-bootstrap"
title: "Plugin repository bootstrap"
status: completed
wave: 1
depends_on: ["01-design-package"]
plan: "plan.md"
spec: "../../specs/redmine-plugin/spec.md"
---

# Task 02: Plugin repository bootstrap

## Intent

Create `yattdev/kandev-plugin-redmine` from `kdlbs/kandev-plugin-template` and rename
its identity throughout so it is a clean starting point for tasks 03-08. Attach it to
this task's workflow as a sibling repository (subtask with its own worktree), never as
a nested clone inside the `kdlbs/kandev` checkout.

## Owned paths

Attached `yattdev/kandev-plugin-redmine` worktree: entire repository (from template).

## Dependencies

Task 01.

## Acceptance

1. Repository is public, template-derived, and attached to a dedicated Kandev subtask
   rather than cloned inside the host worktree.
2. Manifest `id`, Go module path, Makefile binary/package names, UI registration id,
   and release asset name are all renamed to `redmine` consistently (no leftover
   template placeholder strings).
3. `make package-host` succeeds against the renamed, otherwise-unmodified template,
   producing a `manifest.yaml` + host executable + generated `checksums.txt` archive.
4. The template's packaging, test, and release workflow files are preserved, not
   deleted, per the create-kandev-plugin skill's guidance to "replace template
   identity and example behavior without deleting its packaging, test, or release
   safeguards."

## Verification

```sh
# From the attached plugin worktree:
make test
make package-host
tar -tzf dist/redmine-*.tar.gz | grep -E 'manifest.yaml|checksums.txt'
```

## Risks

Low. The main failure mode is an incomplete rename (a leftover "template" string in
the manifest id, Go module path, or release asset name) that only surfaces at install
time — check every rename site the template's own README enumerates.
