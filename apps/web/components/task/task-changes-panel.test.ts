import { describe, it, expect } from "vitest";
import { shouldCloseFileDiffPanel } from "./task-changes-panel";

const PATH = "src/foo.ts";

describe("shouldCloseFileDiffPanel", () => {
  it("returns false when gitStatus is undefined (not loaded yet)", () => {
    expect(shouldCloseFileDiffPanel(undefined, PATH)).toBe(false);
  });

  it("returns true when gitStatus.files is undefined (loaded, no changes)", () => {
    expect(shouldCloseFileDiffPanel({}, PATH)).toBe(true);
  });

  it("returns true when the file is missing from gitStatus.files (discarded)", () => {
    expect(shouldCloseFileDiffPanel({ files: {} }, PATH)).toBe(true);
  });

  it("returns false when the file has a non-empty uncommitted diff", () => {
    const gitStatus = { files: { [PATH]: { diff: "@@ -1 +1 @@\n-a\n+b\n" } } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(false);
  });

  it("returns true when the file entry exists but diff is an empty string", () => {
    const gitStatus = { files: { [PATH]: { diff: "" } } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(true);
  });

  it("returns true when the file entry exists but diff is undefined", () => {
    const gitStatus = { files: { [PATH]: {} } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(true);
  });

  it("is not affected by unrelated files in gitStatus.files", () => {
    const gitStatus = { files: { "other/file.ts": { diff: "diff content" } } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(true);
  });
});
