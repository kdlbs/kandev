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

  it("preserves an existing tab when no preview state is requested", () => {
    expect(upsertOpenFileTab([file], file)).toBeInstanceOf(Array);
    expect(upsertOpenFileTab([file], file)).toEqual([file]);
  });
});
