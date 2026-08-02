# ADR-2026-07-31-system-service-user-continuity: Preserve System Service Identity Across Reinstallation

**Status:** accepted
**Date:** 2026-07-31
**Area:** backend, cli, security, operations

## Context

`kandev service install --system` currently derives its service account from the process that runs
the installer. A command invoked through `sudo` normally sees the non-root `SUDO_USER`, while the
same command invoked from a root login resolves to root. Re-running installation after a package
upgrade can therefore silently rewrite an existing non-root service as root while leaving
`KANDEV_HOME_DIR` and its managed repositories owned by the original account.

Git correctly rejects a repository owned by another account when the process runs with greater or
different authority. The new managed-checkout origin reconciliation made that latent mismatch
visible during every launch and resume: `git remote set-url` exits 128 before an agent starts. The
same identity drift can affect database access, secrets, generated files, executor credentials, and
any future operation, so treating only the Git symptom would leave the underlying operational and
security boundary broken.

Kandev also writes `<KANDEV_HOME_DIR>/service/install.json`. That file is intentionally readable and
writable by the service account. A privileged installer cannot trust it as the authority for a
future run-as account: a compromised service could edit the file to request a more privileged
identity before an administrator reruns installation.

## Decision

System-service identity is durable installation state. `kandev service install --system` resolves
the target account in this order:

1. An explicit `--run-as <user>` supplied for this installation.
2. The account in an existing root-controlled, Kandev-managed systemd unit or launchd plist.
3. A non-root `SUDO_USER` for a first installation.
4. Otherwise no implicit account: installation fails and requires `--run-as`, including the
   explicit `--run-as root` choice.

Only an existing definition carrying Kandev's managed-service marker participates in preservation.
The installer parses the effective account from the root-controlled unit or plist, validates that
the account exists, and rejects malformed or contradictory managed definitions before writing.
Service-owned install metadata remains useful for status and update compatibility checks, but it
may only corroborate the selected account; it cannot select or elevate it.

Before replacing or restarting a system service, the installer validates the system Kandev home:

- If the home does not exist, installation fails with pre-create and ownership guidance. Kandev
  does not create a privileged system data root implicitly.
- If it exists, its owner UID must match the selected account's UID.
- A mismatch fails before the service definition is changed or restarted and reports the current
  owner, selected account, and explicit recovery choices.

Kandev does not recursively chown an existing data tree, add `safe.directory=*`, or suppress Git's
ownership check. An operator intentionally migrating service identity must explicitly select the
new account and reconcile the home ownership according to local policy before installation can
complete. This keeps destructive or privilege-changing filesystem operations outside an otherwise
routine package update.

`--run-as` is valid only with `service install --system`. User services continue to run as the
calling user and do not expose an account override.

## Consequences

- Reinstalling after a Homebrew, npm, or bundle upgrade cannot silently change an established
  system service from a non-root account to root merely because the command was run from a root
  shell.
- First-time root-shell installs become explicit. Operators who genuinely want a root service use
  `--run-as root` and receive a reviewable command history.
- Existing installations that already run as root remain root when their managed definition is
  valid; continuity does not force an unrelated migration.
- A service/data ownership mismatch is detected before restart, instead of appearing later as an
  opaque database, Git, or secret-store failure.
- Service-owned metadata cannot be edited to cause privilege escalation on the next privileged
  install.
- Deliberate account migration requires an additional operator-managed ownership step and may need
  ACL-specific handling on unusual deployments.

## Alternatives Considered

### Continue deriving the account from the current shell

Rejected. The account running a package update is not durable service configuration, and root
logins lack the `SUDO_USER` signal that happened to make earlier installs work.

### Trust `service/install.json` as the preserved identity

Rejected. The service account owns that file. Letting it choose the account written into a
root-controlled service definition would create a privilege-escalation path.

### Automatically chown the Kandev home when the account changes

Rejected. Recursive ownership changes are destructive, can cross operator-managed mount or
symlink boundaries, discard intended ACL semantics, and turn a typo into a broad data mutation.

### Add a broad Git `safe.directory` exception

Rejected. That would mask service identity drift, weaken Git's repository-ownership protection,
and leave non-Git files under the wrong operational authority.

### Always run system services as root

Rejected. It expands task and integration authority unnecessarily and breaks existing non-root
installations whose data and credentials belong to their established service account.
