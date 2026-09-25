import { describe, expect, it } from "vitest";
import {
  isWorkspaceTreePath,
  normalizeWorkspaceFilePath,
  workspaceRelativeFilePath,
} from "./workspace-file-path";

const POSIX_WORKSPACE_ROOT = "/workspace";
const POSIX_WORKSPACE_FILE = "/workspace/src/app.ts";
const RELATIVE_WORKSPACE_FILE = "src/app.ts";

describe("workspaceRelativeFilePath", () => {
  it.each([
    ["POSIX path", POSIX_WORKSPACE_FILE, POSIX_WORKSPACE_ROOT, RELATIVE_WORKSPACE_FILE],
    ["trailing root separator", POSIX_WORKSPACE_FILE, "/workspace/", RELATIVE_WORKSPACE_FILE],
    [
      "Windows path",
      String.raw`c:\workspace\src\app.ts`,
      String.raw`C:\Workspace`,
      RELATIVE_WORKSPACE_FILE,
    ],
    ["Windows drive root", String.raw`C:\src\app.ts`, "c:\\", RELATIVE_WORKSPACE_FILE],
    ["file URI", "file:///workspace/src/app.ts", "file:///workspace", RELATIVE_WORKSPACE_FILE],
    ["file URI root", "file:///src/app.ts", "file:///", RELATIVE_WORKSPACE_FILE],
    ["Windows file URI", "file:///c:/workspace/SRC/App.ts", "file:///C:/Workspace", "SRC/App.ts"],
    [
      "UNC path",
      String.raw`\\BUILD-SERVER\WORK\src\app.ts`,
      String.raw`\\build-server\work`,
      RELATIVE_WORKSPACE_FILE,
    ],
    [
      "UNC file URI",
      "file://BUILD-SERVER/WORK/src/app.ts",
      "file://build-server/work",
      RELATIVE_WORKSPACE_FILE,
    ],
  ])("resolves a contained %s", (_label, filePath, workspaceRoot, expected) => {
    expect(workspaceRelativeFilePath(filePath, workspaceRoot)).toBe(expected);
  });

  it.each([
    ["prefix collision", "/workspace-old/src/app.ts", POSIX_WORKSPACE_ROOT],
    ["external absolute", "/opt/reference.md", POSIX_WORKSPACE_ROOT],
    ["absolute traversal", "/workspace/../outside/app.ts", POSIX_WORKSPACE_ROOT],
    ["case-distinct POSIX file URI", "file:///Workspace/src/app.ts", "file:///workspace"],
    ["missing root", POSIX_WORKSPACE_FILE, null],
  ])("rejects a %s", (_label, filePath, workspaceRoot) => {
    expect(workspaceRelativeFilePath(filePath, workspaceRoot)).toBeNull();
  });
});

describe("normalizeWorkspaceFilePath", () => {
  it("preserves an external absolute path", () => {
    expect(normalizeWorkspaceFilePath("/opt/reference.md", POSIX_WORKSPACE_ROOT)).toBe(
      "/opt/reference.md",
    );
  });

  it("canonicalizes separators in an already-relative path", () => {
    expect(normalizeWorkspaceFilePath(String.raw`src\app.ts`, String.raw`C:\Workspace`)).toBe(
      "src/app.ts",
    );
  });
});

describe("isWorkspaceTreePath", () => {
  it.each(["", "src", "src/components", ".codex/agents", "config:dev", "dir/.env:"])(
    "accepts the workspace-relative tree path %j",
    (path) => {
      expect(isWorkspaceTreePath(path)).toBe(true);
    },
  );

  it.each([
    "/home/jcfs/project/src",
    String.raw`C:\workspace\src`,
    "file:///workspace/src",
    "file:/workspace/src",
    ".",
    "..",
    "../src",
    "src/../outside",
    "src//components",
    String.raw`src\components`,
    "src\0components",
  ])("rejects the non-canonical tree path %j", (path) => {
    expect(isWorkspaceTreePath(path)).toBe(false);
  });
});
