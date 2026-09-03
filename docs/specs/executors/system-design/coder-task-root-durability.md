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
It admits or rejects a Coder task root before remote task materialization; task
rehome behavior remains owned by the task system.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001` | Coder admission contract, exact-root materialization, isolation, presentation and documentation |

## Coder admission contract

The SSH executor detects Coder from the remote process environment rather than
profile names or hostnames. Before helper upload, task-directory creation, Git
preparation, or agent start, a detected Coder host requires profile-owned
`ssh_workdir_policy=durable`. The profile-owned value is authoritative over task
metadata.

Kandev expands the configured `ssh_workdir_root`, requires a dedicated absolute
path, rejects filesystem-root and known ephemeral home or temporary locations,
and performs a live create/remove probe below the root. Missing, inaccessible,
or non-writable roots fail with configuration guidance before Direction runs.
`ssh_workdir_policy=allow_ephemeral` is the explicit data-loss-risk escape hatch
for a known ephemeral location; it does not bypass the live usability probe.
Non-Coder SSH environments retain their existing launch behavior.

See [ADR-2026-09-03-coder-workdir-admission](../../../decisions/2026-09-03-coder-workdir-admission.md).

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

`docs/public/executors.md` documents both policy values, the `/work/.kandev`
example, the preflight failures, and the consequence of accepting an ephemeral
Coder root.

## Failure behavior

Admission errors use the ordinary visible launch-failure path, but occur before
remote task or agent resources are created. The error names the profile keys
that must be corrected. Unknown mount durability is resolved by the operator's
explicit profile policy; known unsafe locations still fail unless the risky
escape value is present.

## Isolation

The live write probe uses a random child directory and removes only that child.
It does not lock the shared root. Normal per-task directory naming,
per-session runtime directories, random agentctl ports, and independent SSH
forwards remain unchanged, so simultaneous Coder tasks and their frontend or
backend dev servers remain isolated.
