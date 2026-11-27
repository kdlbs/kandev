---
status: active
system: executors
created: 2026-09-16
owners:
  - kandev
---

# SSH Identity Discovery Requirements

## Overview

Every executor reached over SSH resolves its credential from one of two
identity sources: the running `ssh-agent`, or a private key file named by an
absolute or `~`-relative path. The path is entered as free text.

Free text is the wrong control for this value. The user is naming a file that
must exist **on the machine running the Kandev backend**, not on their own
workstation and not on the remote host. A headless or service-managed install
makes that distinction invisible: the browser is somewhere else, and nothing in
the form reveals which filesystem the path is resolved against. A typo, a
`~/.ssh/id_25519` for `~/.ssh/id_ed25519`, or a path that is simply correct on
the wrong machine all fail identically at Test Connection, with no indication
that the file was the problem.

The backend already knows the answer. It reads `$HOME/.ssh/config` on every
target resolution, and it reads the key file itself at connect time. It can
enumerate the candidates it would accept and let the user pick one.

This capability is owned at the SSH connection layer rather than by a single
executor, because the connection form is shared: the SSH executor and the
Remote Docker executor render the same control
(`apps/web/components/settings/ssh-connection-form.tsx`).

Two existing constraints shape the result:

- `AC-EXECUTORS-SSH-EXECUTOR-001.4` states that passphrase-protected keys are
  not handled by Kandev; the user must load them into `ssh-agent`. A discovered
  key that is encrypted is therefore not usable as a `file` identity source,
  and presenting it as though it were would move the failure later rather than
  removing it.
- The Remote Docker executor's routes require an administrator
  (`AC-EXECUTORS-REMOTE-DOCKER-001.17`), while the SSH executor's do not.
  Discovery must not silently widen what a non-administrator can learn about
  the backend host's filesystem.

## Requirements

### REQ-EXECUTORS-SSH-IDENTITY-DISCOVERY-001: Identity File Discovery and Selection

**Intent:** A user configuring an SSH-backed executor shall choose a private
key from the identity files Kandev can actually see on the backend host, and
shall retain the ability to name a path that discovery did not find.

#### Acceptance criteria

- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.1:** The system shall enumerate
  candidate identity files on the host running the backend from two fixed
  sources: the immediate contents of `$HOME/.ssh`, and every `IdentityFile`
  value referenced by `$HOME/.ssh/config`. The caller shall not be able to
  supply, influence, or traverse to any other directory.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.2:** A candidate shall be reported
  only when it is a regular file whose contents begin with a recognized private
  key header. Public keys, `known_hosts`, `known_hosts.old`, `config`,
  `authorized_keys`, directories, sockets, and unreadable entries shall be
  excluded.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.3:** Each reported candidate shall
  carry its display path, its key type when determinable, and whether it is
  passphrase-protected. Key material shall never leave the backend: no response
  shall contain private key bytes, a public key body, or a fingerprint derived
  from an encrypted key the backend could not read.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.4:** A passphrase-protected
  candidate shall be reported as present and not selectable as a `file`
  identity source, naming ssh-agent as the supported route for that key.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.5:** Discovery shall require an
  administrator. When the caller is not an administrator, or discovery fails
  for any reason, the identity file control shall fall back to free-text entry
  with no loss of existing capability.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.6:** When the identity source is
  `file`, the identity file control shall present the discovered candidates and
  a custom-path choice. Choosing the custom path shall reveal free-text entry.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.7:** A saved value that discovery
  did not return shall be preserved, shown as the current custom path, and
  shall not be cleared, rewritten, or silently replaced by a discovered
  candidate.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.8:** The control shall state that
  paths resolve on the machine running the Kandev backend, and shall
  distinguish its loading, empty, and error states from a genuinely empty
  `$HOME/.ssh`.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.9:** Selecting a discovered
  candidate shall invalidate a prior successful connection test and its
  fingerprint trust, consistent with the existing treatment of
  connection-affecting fields.
- **AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.10:** The control shall be usable
  on a phone with native selection behavior, not a narrowed desktop dropdown.

## System design

See [SSH Identity Discovery system design](../system-design/ssh-identity-discovery.md).
