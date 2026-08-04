# Coder executor

## Status

Implemented behind the `coder` executor type.

## User contract

- Settings lists Coder beside SSH and lets the user select a template returned by the locally authenticated Coder CLI.
- A profile stores the template and a workspace-name prefix, never a Coder token.
- Each task resolves to `<prefix>-<task-id-prefix>`. An existing workspace with that exact name is reused; a stopped/failed/canceled workspace is started; a missing workspace is created from the selected template using parameter defaults.
- Launch waits for `coder ssh --wait yes <workspace> -- true` before agentctl setup.
- Agentctl then uses an authenticated `coder ssh --stdio` transport while retaining the proven SSH remote filesystem, clone, process, port-forward, resume, and cleanup behavior.
- Stopping a Kandev session stops its remote agentctl process but does not stop or delete the Coder workspace. Workspace lifecycle outside readiness remains owned by Coder TTL/autostop policy and the user.

## Security and failure behavior

- Coder authentication remains in the CLI's configured profile; credentials are not accepted by or copied into Kandev executor config.
- Template, workspace, binary, and prefix are trusted executor-config routing fields and task metadata cannot override them.
- Creation fails closed when no template is selected, Coder is unavailable/unauthenticated, lifecycle commands fail, readiness does not complete, or the stdio SSH handshake fails.
- Workspace names are normalized to lowercase letters, digits, and hyphens and are task-scoped by default.

## Verification

- Unit tests pin deterministic naming and the missing/create, stopped/start, and template-required paths.
- An opt-in live transport test (`KANDEV_TEST_CODER_WORKSPACE`) proves the stdio tunnel with an existing workspace without provisioning or modifying it.
