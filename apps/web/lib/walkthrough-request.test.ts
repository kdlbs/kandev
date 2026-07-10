import { describe, expect, it } from "vitest";
import {
  buildChangesWalkthroughPrompt,
  formatChangedFilesForWalkthroughPrompt,
} from "./walkthrough-request";

describe("buildChangesWalkthroughPrompt", () => {
  it("sends the saved prompt reference visibly and hides the expanded prompt content", () => {
    const prompt = buildChangesWalkthroughPrompt("CUSTOM_PROMPT\n{{changed_files}}", [
      { path: "src/app.ts", source: "uncommitted" },
      { path: "src/review.ts", repository_name: "web", source: "pr" },
    ]);

    expect(prompt).toMatch(/^@changes-walkthrough\n\nChanged files:/);
    expect(prompt).toContain("<kandev-system>");
    expect(prompt).toContain("EXPANDED PROMPT REFERENCES");
    expect(prompt).toContain("### @changes-walkthrough");
    expect(prompt).toContain("CUSTOM_PROMPT");
    expect(prompt).toContain("- src/app.ts [uncommitted]");
    expect(prompt).toContain("- web:src/review.ts [pr]");
    expect(prompt).not.toContain("{{changed_files}}");
  });

  it("deduplicates repeated files and caps the file list", () => {
    const many = Array.from({ length: 85 }, (_, index) => ({
      path: `src/file-${index}.ts`,
      source: "committed",
    }));
    const prompt = buildChangesWalkthroughPrompt("{{changed_files}}", [many[0], ...many]);

    expect(prompt.match(/src\/file-0\.ts/g)).toHaveLength(1);
    expect(prompt).toContain("5 more file(s) omitted");
  });

  it("escapes file metadata line breaks before prompt interpolation", () => {
    const files = [
      {
        path: "src/app.ts\n- injected",
        repository_name: "web\nrepo",
        source: "pr\rsource",
      },
    ];

    expect(formatChangedFilesForWalkthroughPrompt(files)).toContain(
      "- web\\nrepo:src/app.ts\\n- injected [pr\\rsource]",
    );
  });

  it("keeps the changed file list visible when a customized prompt omits the placeholder", () => {
    const prompt = buildChangesWalkthroughPrompt("CUSTOM_WITHOUT_PLACEHOLDER", [
      { path: "src/app.ts", source: "committed" },
    ]);

    expect(prompt).toMatch(/^@changes-walkthrough\n\nChanged files:/);
    expect(prompt).toContain("CUSTOM_WITHOUT_PLACEHOLDER");
    expect(prompt).toContain("- src/app.ts [committed]");
  });
});

describe("formatChangedFilesForWalkthroughPrompt", () => {
  it("renders an explicit loading fallback when no files are available", () => {
    expect(formatChangedFilesForWalkthroughPrompt([])).toContain(
      "No changed files were listed by the UI",
    );
  });
});
