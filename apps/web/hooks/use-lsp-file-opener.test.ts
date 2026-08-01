import { describe, expect, it } from "vitest";
import { toWorkspaceRelativePath } from "./use-lsp-file-opener";

describe("toWorkspaceRelativePath", () => {
  it("resolves attached-repository files from the task workspace root", () => {
    expect(
      toWorkspaceRelativePath("/task-root/second-repository-main/src/index.ts", "/task-root"),
    ).toBe("second-repository-main/src/index.ts");
  });

  it("keeps the absolute path when no workspace root is available", () => {
    expect(toWorkspaceRelativePath("/legacy-worktree/src/index.ts", null)).toBe(
      "/legacy-worktree/src/index.ts",
    );
  });
});
