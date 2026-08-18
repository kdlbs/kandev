# Executor Container Security Options for User Namespace Support

| Metadata | Value |
|---|---|
| **Date** | 2026-08-18 |
| **Status** | accepted |
| **Tags** | backend, security, docker |
| **Replaces** | (none — no prior ADR covers this boundary) |

## Context

Kandev's Local Docker executor launches every task container with no `SecurityOpt` set, so Docker applies its defaults: the default seccomp profile plus the `docker-default` AppArmor profile. Both deny `CLONE_NEWUSER` to processes without `CAP_SYS_ADMIN` — which is every process in a Kandev task container, since Kandev never adds that capability.

Any agent runtime that sandboxes its own file edits via user namespaces therefore fails. The observed symptom is Codex's `apply_patch` → `bwrap: No permissions to create a new namespace`. This stalled four board tasks on 2026-08-17/18. The interim mitigation was pinning agent profiles away from the affected runtime; this ADR records the durable fix, after which pinning can be reverted.

Empirical probe results on the target host confirmed two independent blocking layers:

1. **seccomp** — Docker's default profile gates `clone` on a `SCMP_CMP_MASKED_EQ` check over the namespace flags and puts `unshare`, `mount`, `umount2`, `pivot_root`, `setns` in a `CAP_SYS_ADMIN`-gated group.
2. **AppArmor** — the host runs `apparmor_restrict_unprivileged_userns=1` (Ubuntu 24.04 kernel), and `docker-default` carries no `userns` allow rule plus an explicit `deny mount,`.

Relaxing only one is not enough: seccomp alone fails because AppArmor blocks mount operations inside user namespaces, and AppArmor alone fails because seccomp blocks namespace creation.

## Decision

Add a per-executor-profile boolean `allow_user_namespaces` that, when enabled, emits two Docker `SecurityOpt` entries at container creation time:

- `seccomp=<Kandev-tailored seccomp profile>`
- `apparmor=unconfined`

The profile is a modified copy of Docker's default seccomp profile (vendored from the moby/profiles repository at v29.7.1). The modifications are:

- `clone`: the `SCMP_CMP_MASKED_EQ` namespace-flag restriction for non-CAP_SYS_ADMIN processes is removed.
- `clone3`: the `SCMP_ACT_ERRNO` (ENOSYS) fallback for non-CAP_SYS_ADMIN processes is removed.
- `mount`, `mount_setattr`, `move_mount`, `open_tree`, `setns`, `umount`, `umount2`, `unshare`: moved from the `CAP_SYS_ADMIN`-gated allow list into the unconditional allow list.

Every other syscall restriction in Docker's default profile is preserved. `kexec_load`, `bpf`, `perf_event_open`, `add_key`, and ~50 other syscalls Docker blocks for good reason remain blocked.

The setting is:
- **Off by default** — every existing profile produces byte-identical launch config.
- **Per-profile** — stored as a `config` entry (`allow_user_namespaces: "true"`), not a typed DB column.
- **Authoritative** — the profile value unconditionally overwrites any task-supplied metadata value, preventing self-escalation.
- **Gated from the MCP surface** — the `create_executor_profile` and `update_executor_profile` MCP handlers reject the key with a validation error. The operator HTTP/WS API remains unguarded; this is accepted because the same operator surface already exposes `prepare_script` (arbitrary shell at every launch), a strictly more powerful primitive.
- **Create-time only** — `SecurityOpt` cannot be changed on an existing container. Operators must reset task environments after enabling it.

## Rationale

### Why not free-form `security_opt` list?
Maximum flexibility, but it hands every profile editor `seccomp=unconfined`, `apparmor=unconfined`, and `privileged` — the setting becomes an arbitrary container-escape switch rather than a single, reviewed relaxation.

### Why not `seccomp=unconfined`?
It would restore `kexec_load`, `bpf`, `perf_event_open`, `add_key`, `open_by_handle_at`, and ~50 other syscalls. The tailored profile is strictly better: it only touches namespace-related syscalls.

### Why not `--privileged` or `CapAdd: SYS_ADMIN`?
These grant real host privilege. User namespaces grant privilege only within the new namespace. Strictly worse for the same outcome.

### Why not daemon-wide `userns-remap` / `HostConfig.UsernsMode`?
A different feature (remapping container root to host subuids); does not enable nested namespace creation, and is daemon-global rather than per-profile.

### Why not a Kandev-authored AppArmor profile instead of `apparmor=unconfined`?
Strictly better security, but loading it needs host root and `apparmor_parser`, and Kandev's control plane is frequently itself a container. Left as a documented future escape hatch.

### Why `apparmor=unconfined` for the AppArmor half?
`apparmor_restrict_unprivileged_unconfined=0` on the target hosts means an unconfined process is permitted to create user namespaces. The `apparmor=unconfined` SecurityOpt is what makes this work. On hosts without AppArmor, `apparmor=unconfined` is a no-op.

## Consequences

### Positive

- Agent runtimes that use user namespaces (Codex's `apply_patch`, bwrap-based sandboxing) work inside Kandev container executors without `CAP_SYS_ADMIN` or `--privileged`.
- Default-off means zero risk for profiles that don't need this.
- The tailored seccomp profile preserves all other Docker security hardening.
- The profile is vendored and version-pinned; the delta is test-asserted to be exactly the namespace-related syscalls.

### Negative

- `apparmor=unconfined` is the coarse half of the fix. It removes `deny mount,` and all other docker-default AppArmor restrictions — a broader relaxation than the user namespace itself needs. This is mitigated by default-off + explicit per-profile opt-in.
- User namespaces are historically the single richest source of Linux LPE CVEs. The exposure is kernel attack surface, not a direct grant: privilege obtained inside the namespace does not map to host privilege. Capabilities (still no `CAP_SYS_ADMIN`), cgroups, and namespace isolation remain in force.
- The setting can still be set through the operator HTTP/WS API by anything that can reach the backend port. This is accepted because the same surface already exposes `prepare_script`.

### Risks

- Create-time-only: toggling the profile affects only newly created containers. Operators must be aware of this (stated in UI copy and docs).
- The seccomp base profile is vendored from moby/profiles. A periodic manual refresh on moby upgrades is needed; automating it is out of scope.

## Future Work

- A Kandev-authored AppArmor profile (and a `docker_apparmor_profile` key to name a site-loaded one instead of `unconfined`).
- Installing `bwrap` / `newuidmap` into Kandev-provided agent images.
