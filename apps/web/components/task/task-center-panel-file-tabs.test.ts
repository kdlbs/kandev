import { describe, expect, it } from "vitest";
import type { OpenFileTab } from "@/lib/types/backend";
import { upsertOpenFileTab } from "./task-center-panel-file-tabs";

const file: OpenFileTab = {
  path: "README.md",
  name: "README.md",
  content: "# README",
  originalContent: "# README",
  originalHash: "hash",
  isDirty: false,
};

describe("upsertOpenFileTab", () => {
  it("enables rendered preview on an already-open tab", () => {
    const result = upsertOpenFileTab([file], { ...file, renderedPreview: true });

    expect(result).toEqual([{ ...file, renderedPreview: true }]);
  });

  it("updates the mode on an already-open Markdown tab", () => {
    const result = upsertOpenFileTab([file], { ...file, markdownMode: "edit" });

    expect(result).toEqual([{ ...file, markdownMode: "edit" }]);
  });

  it("keeps same-path tabs from different repositories separate", () => {
    const frontend = { ...file, repo: "frontend" };
    const backend = {
      ...file,
      repo: "backend",
      renderedPreview: true,
      markdownMode: "source" as const,
    };

    expect(upsertOpenFileTab([frontend], backend)).toEqual([frontend, backend]);
  });

  it("preserves an existing tab when no mode is requested", () => {
    const tabs = [file];
    expect(upsertOpenFileTab(tabs, file)).toBe(tabs);
  });

  it.each([
    [{ previous: "target.txt", next: undefined }, "symlink to regular file"],
    [{ previous: undefined, next: "target.txt" }, "regular file to symlink"],
  ] as const)("refreshes resolvedPath for an existing clean tab (%s)", (paths, _label) => {
    const existing = { ...file, resolvedPath: paths.previous };
    const refreshed = { ...file, resolvedPath: paths.next, content: "fresh content" };

    expect(upsertOpenFileTab([existing], refreshed)).toEqual([refreshed]);
  });

  it("refreshes resolvedPath without clobbering a dirty tab", () => {
    const existing = {
      ...file,
      content: "local edits",
      isDirty: true,
      resolvedPath: "old-target.txt",
    };
    const refreshed = { ...file, content: "disk content", resolvedPath: "new-target.txt" };

    expect(upsertOpenFileTab([existing], refreshed)).toEqual([
      { ...existing, resolvedPath: "new-target.txt" },
    ]);
  });

  it("adds a new tab", () => {
    expect(upsertOpenFileTab([], file)).toEqual([file]);
  });

  it("evicts the oldest tab at capacity", () => {
    const tabs = Array.from({ length: 4 }, (_, index) => ({
      ...file,
      path: `file${index}.md`,
      name: `file${index}.md`,
    }));
    const newFile = { ...file, path: "new.md", name: "new.md" };

    const result = upsertOpenFileTab(tabs, newFile);

    expect(result).toHaveLength(4);
    expect(result[0]?.path).toBe("file1.md");
    expect(result[3]).toEqual(newFile);
  });
});
