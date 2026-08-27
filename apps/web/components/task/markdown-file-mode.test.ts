import { describe, expect, it } from "vitest";
import {
  defaultMarkdownFileMode,
  isMarkdownFileModeSupported,
  resolveStoredMarkdownFileMode,
} from "./markdown-file-mode";

describe("markdown file mode", () => {
  it("starts newly opened Markdown files in Preview", () => {
    expect(defaultMarkdownFileMode("README.md")).toBe("preview");
    expect(defaultMarkdownFileMode("components/Widget.mdx")).toBe("preview");
    expect(defaultMarkdownFileMode("src/index.ts")).toBeUndefined();
  });

  it("migrates explicit modes and the legacy preview boolean", () => {
    expect(resolveStoredMarkdownFileMode({ markdownMode: "edit" })).toBe("edit");
    expect(resolveStoredMarkdownFileMode({ markdownMode: "source", markdownPreview: true })).toBe(
      "source",
    );
    expect(resolveStoredMarkdownFileMode({ markdownPreview: true })).toBe("preview");
    expect(resolveStoredMarkdownFileMode({ markdownPreview: false })).toBe("source");
    expect(resolveStoredMarkdownFileMode({})).toBe("source");
  });

  it("does not offer hybrid Edit for MDX", () => {
    expect(isMarkdownFileModeSupported("README.md", "edit")).toBe(true);
    expect(isMarkdownFileModeSupported("README.mdx", "edit")).toBe(false);
    expect(isMarkdownFileModeSupported("README.mdx", "source")).toBe(true);
    expect(isMarkdownFileModeSupported("README.txt", "preview")).toBe(false);
  });
});
