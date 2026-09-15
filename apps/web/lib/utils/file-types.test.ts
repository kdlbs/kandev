import { describe, expect, it } from "vitest";
import { getFilePreviewKind, isMarkdownFile } from "./file-types";

describe("getFilePreviewKind", () => {
  it("recognizes Markdown and HTML preview extensions case-insensitively", () => {
    expect(getFilePreviewKind("docs/README.md")).toBe("markdown");
    expect(getFilePreviewKind("docs/README.MDX")).toBe("markdown");
    expect(getFilePreviewKind("examples/index.html")).toBe("html");
    expect(getFilePreviewKind("examples/index.HTM")).toBe("html");
  });

  it("rejects unsupported and binary files", () => {
    expect(getFilePreviewKind("src/index.ts")).toBe("none");
    expect(getFilePreviewKind("examples/index.html", true)).toBe("none");
  });
});

describe("isMarkdownFile", () => {
  it("recognizes Markdown extensions case-insensitively", () => {
    expect(isMarkdownFile("docs/README.md")).toBe(true);
    expect(isMarkdownFile("docs/README.MDX")).toBe(true);
  });

  it("rejects non-Markdown files", () => {
    expect(isMarkdownFile("src/index.ts")).toBe(false);
  });
});
