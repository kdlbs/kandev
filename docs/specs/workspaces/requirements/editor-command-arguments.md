---
status: active
system: workspaces
created: 2026-11-16
owners:
  - kandev
---

# Editor Command Argument Requirements

## Overview

A custom command editor is an administrator-authored command template with
`{cwd}`, `{file}`, `{rel}`, `{line}` and `{column}` placeholders. Kandev fills
the placeholders from the resolved worktree target and starts the program on
the backend host.

The person opening the editor chooses the file, and with `session.exec` they
can also create files with arbitrary names in the worktree. A file name must
therefore be data in exactly the argument slot the administrator wrote. It must
never add, remove or split arguments, regardless of how the administrator
quoted the placeholder.

Workspaces owns this contract because the placeholder values are the
[contained editor target](editor-file-containment.md) that this system already
defines. The authority to author a template and to open an editor belongs to
[roles and scopes](../../auth/requirements/roles-and-scopes.md).

## Terminology

- **Command template:** The `config.command` string of a `custom_command`
  editor, written by a caller holding `org.config.manage`.
- **Template token:** One argument produced by splitting the command template
  with shell-word rules (whitespace separation, single quotes, double quotes,
  backslash escapes).
- **Placeholder value:** The worktree path, absolute target path, relative
  target path, line or column substituted for a placeholder.

## Requirements

### REQ-WORKSPACES-EDITOR-COMMAND-001: Placeholder values never change argument boundaries

**Intent:** Opening a file runs exactly the program and argument list the
administrator wrote, with the target substituted as data.

**User story:** As an administrator, I want my custom editor command to receive
the selected file as one argument, so that a crafted file name cannot change
what my command does.

#### Acceptance criteria

- **AC-WORKSPACES-EDITOR-COMMAND-001.1:** When a custom command editor opens a
  target, the system shall split the command template into template tokens
  before substituting any placeholder value, so the launched argument count
  equals the template token count for every possible placeholder value.
- **AC-WORKSPACES-EDITOR-COMMAND-001.2:** When a placeholder value contains
  whitespace, single or double quotes, backslashes, `$`, backticks, `;`, `|`,
  `&`, or parentheses, the system shall place it verbatim in the template
  token that contains the placeholder, whether that placeholder is unquoted,
  single-quoted, or double-quoted in the template.
- **AC-WORKSPACES-EDITOR-COMMAND-001.3:** When a template token combines a
  placeholder with literal text, such as `{file}:{line}` or `--goto={file}`,
  the system shall keep the literal text and substitute the value in place
  within that one argument.
- **AC-WORKSPACES-EDITOR-COMMAND-001.4:** When a template has an unclosed quote
  or yields no tokens, the system shall return the existing invalid editor
  configuration failure and start no process.

## Out of scope

- Who may author a template or open an editor; that is
  [`REQ-AUTH-ROLES-AND-SCOPES-001`](../../auth/requirements/roles-and-scopes.md).
- Target containment; that is
  [`REQ-WORKSPACES-EDITOR-CONTAINMENT-001`](editor-file-containment.md).
- How the launched program interprets its own arguments, including a target
  name that begins with `-`. The template author can separate options with
  `--` when the program supports it.
- Shell interpretation. Kandev never passes the template to a shell; an
  administrator who writes `sh -c` in a template owns that shell's parsing.
- Built-in editors, remote SSH URLs, hosted URLs and embedded VS Code, which do
  not use command templates.

## Implementation plan

- [Editor launch authorization](../../../plans/editor-launch-authorization/plan.md)
