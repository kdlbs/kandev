import { describe, expect, it } from "vitest";
import { isMarkdownFile } from "./file-types";

describe("isMarkdownFile", () => {
  it("recognizes Markdown extensions case-insensitively", () => {
    expect(isMarkdownFile("docs/README.md")).toBe(true);
    expect(isMarkdownFile("docs/README.MDX")).toBe(true);
  });

  it("rejects non-Markdown files", () => {
    expect(isMarkdownFile("src/index.ts")).toBe(false);
  });
});
