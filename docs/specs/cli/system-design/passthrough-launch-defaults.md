---
status: current
system: cli
requirements:
  - REQ-CLI-PASSTHROUGH-LAUNCH-001
---

# Passthrough Launch Defaults System Design

## Purpose and boundaries

The CLI system owns the arguments that Kandev uses to launch agent CLIs in
passthrough mode. This design changes only the built-in Claude passthrough
default.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-CLI-PASSTHROUGH-LAUNCH-001` | [Command composition](#command-composition), [Compatibility](#compatibility), [Verification](#verification) |

## Components and responsibilities

- `agents.NewClaudeACP` defines the required base command for Claude
  passthrough.
- `agents.StandardPassthrough.BuildPassthroughCommand` adds model, permission,
  resume, prompt, MCP, and profile CLI arguments.
- `lifecycle.Manager.profileCLIFlagTokens` resolves enabled profile CLI flags
  before it builds the passthrough command.
- `agent/settings/controller.Controller.PreviewAgentCommand` builds the same
  passthrough argv for the settings preview, including resolved profile CLI
  flags.

## Command composition

The Claude base command contains `npx -y @anthropic-ai/claude-code`. It does not
contain `--verbose`.

The standard passthrough builder appends enabled profile CLI arguments. Thus, a
profile with `--verbose` keeps access to the Claude verbose view mode.

The change does not alter the ACP adapter command. It also does not alter model,
permission, resume, prompt, or MCP argument composition.

The settings command preview passes resolved profile CLI flags to the same
passthrough builder. The preview therefore matches the command that Kandev
launches for the selected profile.

## Compatibility

The profile model and storage remain unchanged. Existing profiles receive the
quiet default unless their CLI-flags list already contains `--verbose`.

No migration or new user-interface control is necessary. The current CLI-flags
field remains the explicit configuration path.

## Failure behavior

Malformed profile CLI flags keep the current warning and omission behavior.
This change adds no new error path.

## Security and observability

The change does not alter permission flags, environment handling, or command
redaction. Existing passthrough process logs continue to record the redacted
launch command.

## Verification

A focused agent-package test inspects the complete tokenized command. A
controller test compares passthrough preview output with an enabled profile
flag. The tests cover the quiet default and the explicit `--verbose` opt-in.

A live Claude session is not necessary for this contract. The command argv is
the Kandev-owned boundary, while Claude owns its output rendering.

## Related decisions

None. The design uses the existing profile CLI-flags contract and adds no new
architecture boundary.
