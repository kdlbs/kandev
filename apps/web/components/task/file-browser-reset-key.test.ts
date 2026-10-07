import { describe, expect, it } from "vitest";
import { getFileBrowserResetKey } from "./file-browser";

const ENVIRONMENT_ID = "environment-1";

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
        environmentId: ENVIRONMENT_ID,
        worktreeCount: 1,
        workspaceFilesRefresh: 0,
      }),
    ).not.toBe(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: ENVIRONMENT_ID,
        worktreeCount: 1,
        workspaceFilesRefresh: 1,
      }),
    );
  });

  it("changes the cache key when a selected checkout is replaced at the same count", () => {
    expect(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: ENVIRONMENT_ID,
        worktreeCount: 2,
        inventoryRevision: "old-checkouts",
        workspaceFilesRefresh: 0,
      }),
    ).not.toBe(
      getFileBrowserResetKey({
        sessionId: "session-1",
        environmentId: ENVIRONMENT_ID,
        worktreeCount: 2,
        inventoryRevision: "new-checkouts",
        workspaceFilesRefresh: 0,
      }),
    );
  });
});
