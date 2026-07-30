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
  it("enables Markdown preview on an already-open tab", () => {
    const result = upsertOpenFileTab([file], { ...file, markdownPreview: true });

    expect(result).toEqual([{ ...file, markdownPreview: true }]);
  });

  it("keeps same-path tabs from different repositories separate", () => {
    const frontend = { ...file, repo: "frontend" };
    const backend = { ...file, repo: "backend", markdownPreview: true };

    expect(upsertOpenFileTab([frontend], backend)).toEqual([frontend, backend]);
  });

  it("preserves an existing tab when no preview state is requested", () => {
    const tabs = [file];
    expect(upsertOpenFileTab(tabs, file)).toBe(tabs);
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
