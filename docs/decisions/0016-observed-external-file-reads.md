# 0016: Read-Only Absolute File Paths

**Status:** accepted
**Date:** 2026-06-14
**Area:** backend

## Context

Agent CLIs can read files outside the configured task workspace, and those absolute paths can appear in normalized ACP tool-call messages. Kandev's workspace file API previously rejected every outside-workspace path as path traversal, which prevented the UI from reopening files the agent could already inspect. The product decision is to let the UI read absolute file paths directly for parity with agent CLI behavior.

## Decision

`internal/agentctl/server/process.WorkspaceTracker.GetFileContent` remains workspace-scoped for relative paths and relative traversal attempts, but when the request path is absolute it resolves symlinks and reads the canonical file directly. The read path reuses the existing file-size cap and binary handling.

Write operations, directory tree reads, search, git-ref reads, directories, missing files, and relative traversal remain rejected by their existing workspace-scoped path validation. Only the read-only current-file content endpoint accepts absolute paths.

## Consequences

The UI can display absolute files, including Sprite documentation files outside `/workspace`, without needing an ACP observation first. This intentionally makes the session file-content API a read-only host-file reader for any file the agentctl process can read. Deployments that expose Kandev beyond trusted users must account for that risk.

## Alternatives Considered

- Allow only observed absolute paths from agent read events: rejected because it prevents direct inspection of absolute paths even though the agent CLI can already read them.
- Allow specific host prefixes such as `/.sprite`: rejected because the problem is not Sprite-specific and prefix allowlists are easy to broaden incorrectly.
- Keep rejecting all external paths: rejected because it breaks inspection parity with the agent CLI.
