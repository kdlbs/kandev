---
status: draft
system: executors
requirements:
  - REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001
created: 2026-09-03
owners:
  - kandev
---

# Coder Task Root Durability System Design

## Purpose and boundaries

The executor system owns SSH profile configuration and remote health evidence.
It warns about a risky Coder task root; task rehome behavior remains owned by
the task system.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001` | Detection and warning contract, exact-root materialization, presentation and documentation |

## Detection and warning contract

The SSH executor profile settings card always presents a prominent durable-root
warning, with Coder called out as a common risk. This conservative warning does not guess from hostnames or
claim arbitrary mounts are durable, and it does not block profile save or
launch. Future remote mount-health evidence can narrow the warning without
changing the durability contract.

## Exact-root materialization

The default SSH prepare script considers an existing checkout reusable only
when `git rev-parse --show-toplevel` resolves, after canonicalization, to the
exact task workspace. Git discovery of an ancestor checkout is treated as no
task checkout: Kandev initializes `.git` in the task workspace before adding
the task repository origin. The post-prepare verifier independently enforces
the same exact-root invariant.

The remote-contribution preparation path uses the same exact-root predicate.
This prevents a durable task root such as
`/opt/jumprope-fullstack/.kandev/tasks/<task>` from adopting
`/opt/jumprope-fullstack/.git` and reporting a false origin conflict.

## Presentation and documentation

The existing executor profile settings-card surface renders the warning on
desktop and mobile. It recommends configuring `ssh_workdir_root` beneath the
Coder persistent mount, commonly `/work/.kandev`, and links to the SSH executor
guide. The UI shares warning data and copy across viewports and uses the
existing responsive settings-card layout.

`docs/public/executors.md` documents the durability requirement, the
`/work/.kandev` example, and the consequence of rebuilding a Coder workspace
whose task root is ephemeral.

## Failure behavior

The warning is advisory because Kandev cannot reliably classify every custom
Coder mount. Save and launch continue.
