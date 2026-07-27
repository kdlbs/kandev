import { describe, expect, it } from "vitest";
import { getFileBrowserResetKey } from "./file-browser";

describe("getFileBrowserResetKey", () => {
  it("changes the session-scoped key when refresh generation increments without an environment", () => {
    expect(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: undefined,
        worktreeCount: 1,
        workspaceFilesRefresh: 0,
      }),
    ).not.toBe(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: undefined,
        worktreeCount: 1,
        workspaceFilesRefresh: 1,
      }),
    );
  });

  it("changes the environment-scoped key when refresh generation increments", () => {
    expect(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: "environment-1",
        worktreeCount: 1,
        workspaceFilesRefresh: 0,
      }),
    ).not.toBe(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: "environment-1",
        worktreeCount: 1,
        workspaceFilesRefresh: 1,
      }),
    );
  });
});
