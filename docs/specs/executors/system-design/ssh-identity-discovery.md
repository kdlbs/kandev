---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-IDENTITY-DISCOVERY-001
created: 2026-09-16
owners:
  - kandev
---

# SSH Identity Discovery System Design

## Context

`SSHConnectionForm` renders a free-text `TextField` for `identity_file` when
the identity source is `file`. The value is persisted into executor config as
`ssh_identity_file` and resolved at connect time by
`apps/backend/internal/agent/runtime/lifecycle/executor_ssh_connection.go`,
which expands `~` against `os.UserHomeDir()` on the backend host.

Two callers share the form: `ssh-connection-card.tsx` (SSH executor) and
`remote-docker-connection-section.tsx` (Remote Docker executor). Neither owns
the control, so discovery is added once, beneath both.

## Decisions

### D1: Discovery runs on the backend, over a fixed root set

The value names a file on the backend host, so only the backend can enumerate
it. The endpoint accepts **no path parameter**. Roots are fixed at
`$HOME/.ssh` (non-recursive) plus the `IdentityFile` values already parsed out
of `$HOME/.ssh/config`. This satisfies
AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.1 by construction rather than by
validation: there is no input to traverse with.

`$HOME` is resolved with `os.UserHomeDir()`, matching the resolver, so
discovery and connection agree on the same home even when `KANDEV_HOME_DIR`
points elsewhere.

### D2: Classification reads headers, never material

For each candidate the backend opens the file and reads a bounded prefix
(4 KiB is sufficient for every header form) rather than the whole file. It
classifies on:

- `-----BEGIN OPENSSH PRIVATE KEY-----` — OpenSSH format.
- `-----BEGIN RSA/EC/DSA/PRIVATE KEY-----` — legacy PEM.

Anything else is not a candidate. Encryption is determined by parsing with
`ssh.ParseRawPrivateKey` and treating `*ssh.PassphraseMissingError` as
"encrypted"; any other parse error excludes the file. Key type comes from the
successfully parsed key. This reuses `golang.org/x/crypto/ssh`, already a
direct dependency of the SSH connection path, rather than hand-parsing.

The response carries `path`, `display_path`, `key_type`, and `encrypted`. It
carries no bytes from the file. This is the whole of
AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.3 — the endpoint's value is that it
answers "which files would work" without ever becoming a key-exfiltration
primitive.

### D3: Administrator-gated, with graceful degradation

The endpoint is registered as
`GET /api/v1/ssh/identities` with `authn.RequireAdmin()`.

The SSH executor's existing routes are not admin-gated and this design does not
change that. Discovery is gated because it is a filesystem-listing primitive
over the backend user's home, which is a strictly larger disclosure than
anything the SSH routes expose today. Gating the *picker* costs a
non-administrator nothing, because the free-text field remains and is the
current behavior in full.

The frontend therefore treats 401/403 exactly as it treats a network failure:
render the existing `TextField`. Discovery is an enhancement layered on a path
that already works (AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.5).

### D4: The control is a Combobox with a terminal custom-path option

`apps/web/components/combobox.tsx` already provides search, `disabled` with
`disabledReason`, `description` per option, and a portalled popover. It is
reused rather than extended.

The component owns its own grouping: it renders the selected option first, then
the remaining enabled options, then a separator, then the disabled ones. The
option list is therefore supplied as discovered candidates in order
(`$HOME/.ssh` entries before config-referenced ones, each alphabetical) followed
by the custom-path row, and the rendered result is:

1. The current selection, if any.
2. Remaining selectable candidates, then the custom-path row.
3. A separator, then encrypted candidates with `disabled: true` and a
   `disabledReason` naming ssh-agent
   (AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.4).

Encrypted entries land last rather than mid-list because the shared component
sorts them there. That is the desired reading order anyway: the rows a user can
act on come first, and the ones they cannot are still visible below the rule.

`__custom_path__` is a control-flow sentinel compared with `===`, never translated,
and therefore never a locale value. Selecting it sets a local `customMode` flag and reveals the existing `TextField` beneath the combobox;
the persisted `identity_file` shape is unchanged in every case, so nothing
downstream of the form learns that discovery exists.

### D5: Unknown saved values are custom, not errors

On mount, if `identity_file` is non-empty and matches no discovered `path`, the
form starts in custom mode with that value intact
(AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.7). A saved profile whose key has
since been moved, or which was configured against a different backend host,
must remain editable and must not be silently repointed at a discovered key
that happens to be present.

### D6: Selection is a connection-affecting change

`identity_file` is already in `CONNECTION_FIELDS` in `ssh-connection-card.tsx`,
so a change flips `resultStale` and re-gates trust and Save. Selecting from the
combobox routes through the same `onChange("identity_file", …)`, so
AC-EXECUTORS-SSH-IDENTITY-DISCOVERY-001.9 needs no new mechanism — only a test
that proves the combobox path is not a bypass.

## Interface

```
GET /api/v1/ssh/identities            (admin)

200 {
  "home_dir": "/home/user",
  "identities": [
    { "path": "/home/user/.ssh/id_ed25519",
      "display_path": "~/.ssh/id_ed25519",
      "key_type": "ssh-ed25519",
      "encrypted": false,
      "source": "ssh_dir" },
    { "path": "/home/user/keys/prod",
      "display_path": "~/keys/prod",
      "key_type": "",
      "encrypted": true,
      "source": "ssh_config" }
  ]
}
```

`source` is `ssh_dir` or `ssh_config`. An unreadable `$HOME/.ssh` is a 200 with
an empty list, not an error: "no keys here" and "cannot look" are the same
actionable outcome for the user, and the distinction would leak whether the
directory exists.

## Files

- `apps/backend/internal/ssh/identities.go` (new) — enumeration and
  classification.
- `apps/backend/internal/ssh/handlers.go` — route registration.
- `apps/web/lib/api/domains/ssh-api.ts` — client, via `fetchJson` so it honors
  `apiBaseUrl`.
- `apps/web/components/settings/ssh-identity-file-field.tsx` (new) — the
  control.
- `apps/web/components/settings/ssh-connection-form.tsx` — swap the field.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/executors.json`.

## Risks

- **Symlinked keys.** A symlink into an agent-backed store may read as a
  non-key file and be excluded. Exclusion is the safe direction: the free-text
  path still accepts it.
- **Large `$HOME/.ssh`.** Enumeration is bounded by reading a 4 KiB prefix per
  entry and by the directory being non-recursive. No pagination is designed.
- **Discovery implies support.** Listing a key does not prove the remote
  authorizes it. The control must not read as a validity check; Test Connection
  remains the only proof.
