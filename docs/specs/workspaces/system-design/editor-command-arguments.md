---
status: current
system: workspaces
requirements:
  - REQ-WORKSPACES-EDITOR-COMMAND-001
---

# Editor Command Argument System Design

## Purpose and boundaries

The editors service (`apps/backend/internal/editors/service`) turns a
`custom_command` template and a contained worktree target into one
`exec.Command` call. This design fixes the order of tokenization and
substitution. It does not change target resolution, which
[editor file containment](editor-file-containment.md) owns, or launch
authority, which [roles and scopes](../../auth/requirements/roles-and-scopes.md)
owns.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `AC-WORKSPACES-EDITOR-COMMAND-001.1` | [Control flow](#control-flow) |
| `AC-WORKSPACES-EDITOR-COMMAND-001.2`, `AC-WORKSPACES-EDITOR-COMMAND-001.3` | [Data and contracts](#data-and-contracts) |
| `AC-WORKSPACES-EDITOR-COMMAND-001.4` | [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `launchCommand` builds the argument vector and starts the process with
  `cmd.Dir` set to the worktree.
- `securityutil.SplitShellCommand` remains the only shell-word tokenizer. It
  is applied to the administrator's template only.
- Placeholder substitution operates on one already-split token at a time.

## Data and contracts

The template contract and its placeholders are unchanged: `{cwd}`, `{file}`,
`{rel}`, `{line}`, `{column}`. Substitution inserts the raw value. It does not
apply `securityutil.ShellEscape`, because the value is never tokenized again;
escaping a value that will not be parsed would leave literal quote characters
in the argument.

A token may contain several placeholders and literal text. Each placeholder in
the token is replaced in place; the token stays one argument.

Compatibility: for an unquoted placeholder, the old escape-then-split order
already produced one argument holding the raw value, so the resulting argument
vector is identical. For a quoted placeholder (`"{file}"` or `'{file}'`), the
old order produced literal quote characters inside the argument or, for a
value containing a double quote, extra arguments. The new order produces the
raw value, which is what the template author intended.

## Control flow

1. `dispatchEditorKind` passes the parsed template, worktree path, absolute
   target, line and column to `launchCommand`.
2. `launchCommand` calls `SplitShellCommand(template)`.
3. For each token, it substitutes every placeholder with its raw value.
4. It calls `exec.Command(tokens[0], tokens[1:]...)` and `Start()`.

No step passes the template or any value to a shell.

## Failure and recovery

A tokenizer error (unclosed quote) or an empty token list returns
`ErrEditorConfigInvalid`, which the handler maps to 400. No process starts.
A placeholder value can no longer cause a tokenizer error, because values are
not tokenized.

## Persistence

None. Stored templates are unchanged and need no migration.

## Security

The template author holds `org.config.manage`. The opener holds `session.exec`
on the session's workspace and controls only the target path inside the
worktree, the line and the column. With this design that control is limited to
argument values. Program interpretation of those values is out of scope, as
recorded in the requirement.

## Observability

No new logs or metrics. Launch failures keep the existing handler error log.
