---
status: shipped
created: 2026-08-23
owner: kandev
---

# Agent Git permission boundary

## Why

Users can edit files and read Git status in a restricted agent mode, then see
the agent fail when it runs `git add` or `git commit`. The failure can make the
repository or Kandev appear broken, even though the Changes panel can commit
the same work through a different permission path.

## What

- Public Git documentation explains that Changes-panel Git operations and
  agent-shell Git commands use different permission paths.
- The documentation explains that `git status` can work while a shell commit
  fails because the agent cannot write Git metadata such as `.git/index.lock`.
- The documentation directs users to commit from the Changes panel without
  changing the agent mode.
- The documentation explains that a user who needs the agent shell to commit
  must select an agent mode that allows Git metadata writes.
- The documentation states that mode names come from the installed agent and
  uses Codex mode names only as examples.

## Scenarios

- **GIVEN** an agent edits files in a restricted mode, **WHEN** `git status`
  works but `git commit` fails with an `.git/index.lock` error, **THEN** the
  public Git guide explains the permission boundary and the Changes-panel
  recovery path.
- **GIVEN** a user opens the Changes panel with agent edits present, **WHEN**
  the user chooses Commit Changes, **THEN** the public Git guide explains that
  Kandev performs the Git operation outside the agent shell sandbox.
- **GIVEN** a user wants the agent shell to commit, **WHEN** the user reads the
  public Git guide, **THEN** the guide explains that the user must choose an
  agent-specific mode that permits Git metadata writes.

## Out of scope

- Changing agent sandbox behavior or permission modes.
- Changing the Changes-panel Git implementation.
- Adding a new Kandev-wide permission-mode name.
- Changing Git error handling or automatically changing a session mode.
