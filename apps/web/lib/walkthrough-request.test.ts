import { describe, expect, it } from "vitest";
import { buildChangesWalkthroughPrompt } from "./walkthrough-request";

describe("buildChangesWalkthroughPrompt", () => {
  it("asks the agent to use show_walkthrough with range and availability constraints", () => {
    const prompt = buildChangesWalkthroughPrompt([
      { path: "src/app.ts", source: "uncommitted" },
      { path: "src/review.ts", repository_name: "web", source: "pr" },
    ]);

    expect(prompt).toContain("show_walkthrough_kandev");
    expect(prompt).toContain("Use `line_end` whenever a logical explanation spans multiple lines");
    expect(prompt).toContain("do not assume the PR head is checked out locally");
    expect(prompt).toContain("Do not include a `Justification:` preamble");
    expect(prompt).toContain("- src/app.ts [uncommitted]");
    expect(prompt).toContain("- web:src/review.ts [pr]");
  });

  it("deduplicates repeated files and caps the file list", () => {
    const many = Array.from({ length: 85 }, (_, index) => ({
      path: `src/file-${index}.ts`,
      source: "committed",
    }));
    const prompt = buildChangesWalkthroughPrompt([many[0], ...many]);

    expect(prompt.match(/src\/file-0\.ts/g)).toHaveLength(1);
    expect(prompt).toContain("5 more file(s) omitted");
  });
});
