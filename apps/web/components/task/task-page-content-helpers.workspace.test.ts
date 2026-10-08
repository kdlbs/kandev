import { describe, expect, it } from "vitest";
import { workspaceId as toWorkspaceId } from "@/lib/types/http";
import { shouldLoadWorkspaceRepositories } from "./task-page-content-helpers";

describe("shouldLoadWorkspaceRepositories", () => {
  it("skips local repository loading for Cursor Cloud tasks", () => {
    expect(
      shouldLoadWorkspaceRepositories({
        workspace_id: toWorkspaceId("ws-1"),
        primary_executor_type: "cursor_cloud",
      }),
    ).toBe(false);
  });

  it("loads local repositories only when the task has a workspace", () => {
    expect(
      shouldLoadWorkspaceRepositories({
        workspace_id: toWorkspaceId("ws-1"),
        primary_executor_type: "local",
      }),
    ).toBe(true);
    expect(shouldLoadWorkspaceRepositories(null)).toBe(false);
  });
});
