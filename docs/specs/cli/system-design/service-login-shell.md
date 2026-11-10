---
status: draft
system: cli
requirements:
  - REQ-CLI-SERVICE-LOGIN-SHELL-001
---

# Service login shell design

## Purpose and boundaries

The native service installer owns the opt-in and its durable selection.
This follows [the service-shell decision](../../../decisions/2026-10-26-opt-in-service-login-shell.md).
The shared [local-agent environment](../../platform/system-design/local-agent-path.md)
continues to append `.local/bin` without invoking a shell.
The [service serialization design](native-service-configuration.md) retains its
Linux contract. This design adds a separate macOS user-service path.

## Requirement mapping

| Criteria under REQ-CLI-SERVICE-LOGIN-SHELL-001 | Design section |
| --- | --- |
| 001.1, 001.2, 001.5, 001.7 | Selection and persistence |
| 001.3, 001.4, 001.6, 001.7 | Startup and process lifetime |
| 001.5 | Apply update continuity |
| 001.8 | Documentation and verification |

## Selection and persistence

Extend `serviceArgs` in `internal/launcher/service.go` with a shell value and
an explicit-presence bit. Extend `nativeServiceUnitInput` with the resolved path.
Keep parsing, selection, and execution in a focused `service_shell.go` module.
Public `--shell` applies only to `service install` on Darwin user services.
The internal run action also accepts its required absolute shell argument.

Accept `bash`, `zsh`, or an absolute executable path whose basename is one of
those names. Resolve names through the installer's PATH once. Reject relative
paths with separators, empty values, non-executable files, and control characters.
Support spaces and shell metacharacters inside absolute directory names.
Do not persist shell command fragments or extra shell arguments.

Use the following selection order:

1. An explicit shell argument selects a validated shell. `none` clears selection.
2. Without that argument, a current managed user plist preserves its selection.
3. A legacy direct-launch plist or a new installation selects no shell.

Parse the managed plist structurally, including its `ProgramArguments` array.
Recognize the existing direct invocation and the new internal invocation only.
Reuse the XML parsing conventions in `service_identity.go`, without changing
system-account selection. A malformed managed invocation fails before writes.
An explicit selection or reset can repair an invalid saved shell selection.
Unmanaged definitions retain the existing backup policy and supply no selection.

Persist optional `shell` in `nativeServiceMetadata` within `service/install.json`.
The saved value is an absolute path, or an omitted field for direct startup.
Preserve version 1 and owner-only atomic writes. No database migration is needed.
Validate matching manager, user mode, service path, and shell selection when
metadata corroborates a managed installation. A mismatch requires explicit
reinstallation with a selection or `--shell=none`.
Never use user-service shell metadata to select a system-service identity.

## Startup and process lifetime

Without selection, `renderLaunchdPlist` retains `[<kandev>, --headless]`.
With selection, it emits structured arguments:

```text
<absolute-kandev> service run --shell <absolute-shell>
```

The new internal action stays out of public help. It verifies Darwin user-service
context and the shell executable before process replacement. It resolves the
effective UID's home through the account database, not the Kandev data directory.
Account lookup failure prevents startup. It sets `HOME` and `SHELL` for the shell.

Use a Darwin-specific process-replacement helper (`syscall.Exec`) and an explicit
unsupported-platform implementation. Execute the selected shell with `-l -c`.
Pass a fixed command, `exec /usr/bin/env "$@"`, a fixed `$0`, and separate
positional arguments for environment assignments and `<absolute-kandev> --headless`.
The shell command never interpolates executable paths or user-supplied shell text.
Tests must cover the different Bash and Zsh positional-argument conventions.

The final environment assignments restore `HOME`, `SHELL`, and each explicitly
supplied service value emitted by `renderLaunchdPlist`. This includes service
identity, metadata, selected configuration pointers, port, log level, bundle,
version, and data-home values. Preserve values exactly, including punctuation.
Do not put arbitrary inherited environment values or credentials into argv.
PATH and other shell exports remain inherited unless an existing downstream
override owns them. Do not serialize the complete shell environment.

The shell replaces itself with the original absolute Kandev binary. The final
invocation uses `--headless`, so it cannot recurse into shell initialization.
Backend-only supervisor restarts reuse the initialized environment. A full
service restart runs startup files again. Signals keep the existing process
identity across replacement, including while shell initialization runs.

Nonzero shell exits and missing executables reach existing service logs and
launchd restart policy. An explicit `exit 0` in startup files can also prevent
startup, without readiness. A hanging startup file remains a service-startup
failure that the operator resolves with stop, file repair, or `--shell=none`.
No automatic fallback, interactive flag, or new shell-output capture is added.

## Apply update continuity

The shell value must cross every existing serialization boundary:

```text
nativeServiceMetadata -> install.json
  -> updates.serviceInstallMetadata -> updateIntent.Install
  -> nativeUpdateInstall -> nativeServiceInstallArgs -> regenerated plist
```

Add the optional field to the backend metadata reader in
`internal/system/updates/install_state.go` and the native intent reader in
`service_self_update.go`. `writeApplyIntent` already copies the metadata value.
The new field must survive that copy and JSON round trip.
Validate its shape and supported manager/mode before package-manager commands.
Emit `--shell <absolute-path>` for a saved selection and `--shell=none` for an
explicit direct-start update snapshot. Legacy missing or null fields mean direct
startup. Existing manual install without a flag instead preserves its plist.

Guard Apply against a plist/metadata shell mismatch before writing an intent.
Keep the existing supported install kinds and user-service permission boundary.
Homebrew, npm, and npx planners all use `nativeServiceInstallArgs`.
The update helper does not need another login-shell wrapper for this change.

## Documentation and verification

Update `docs/public/cli.md` and `docs/public/run-as-a-service.md` with implementation.
Document Bash/Zsh support, user-service scope, reset, and update persistence.
Describe normal non-interactive login semantics, not complete terminal parity.
Zsh normally reads `.zshenv`, `.zprofile`, and `.zlogin`, but not `.zshrc`.
Startup options and explicit sourcing can change this behavior.
See the [official Zsh startup reference](https://zsh.sourceforge.io/Doc/Release/Files.html).

Use portable renderer, metadata, argument-validation, and subprocess tests.
Retain a separate native Darwin gate for real Zsh/Bash startup and launchd.
Verify the configured executable and observed process identity separately from
macOS background-item wording. The latter needs native observation.
No browser fixture can prove the service manager boundary.
