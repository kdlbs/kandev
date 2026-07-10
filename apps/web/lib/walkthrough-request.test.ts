import { describe, expect, it } from "vitest";
import { buildChangesWalkthroughPrompt } from "./walkthrough-request";

describe("buildChangesWalkthroughPrompt", () => {
  it("sends only the prompt reference visibly and hides expanded prompt content", () => {
    const prompt = buildChangesWalkthroughPrompt("CUSTOM_PROMPT\nshow_walkthrough_kandev");

    expect(prompt).toMatch(/^@changes-walkthrough\n\n<kandev-system>/);
    expect(prompt).toContain("<kandev-system>");
    expect(prompt).toContain("EXPANDED PROMPT REFERENCES");
    expect(prompt).toContain("### @changes-walkthrough");
    expect(prompt).toContain("CUSTOM_PROMPT");
    expect(prompt).not.toContain("Diff context:");
    expect(prompt).not.toContain("Base branch:");
    expect(prompt).not.toContain("Base commit:");
    expect(prompt).not.toContain("src/app.ts");
  });

  it("maps diff context placeholders to static instructions instead of dynamic context", () => {
    const prompt = buildChangesWalkthroughPrompt("CUSTOM\n{{diff_context}}");

    expect(prompt).toContain("CUSTOM");
    expect(prompt).toContain("Inspect the task changes and compare against the correct base.");
    expect(prompt).not.toContain("PR: kdlbs/kandev#42");
    expect(prompt).not.toContain("Head branch:");
    expect(prompt).not.toContain("Base branch:");
    expect(prompt).not.toContain("Changed files:");
    expect(prompt).not.toContain("{{diff_context}}");
  });

  it("maps legacy changed_files placeholders to static instructions instead of file paths", () => {
    const prompt = buildChangesWalkthroughPrompt("CUSTOM\n{{changed_files}}");

    expect(prompt).toContain("CUSTOM");
    expect(prompt).toContain("Inspect the task changes and compare against the correct base.");
    expect(prompt).not.toContain("Changed files:");
    expect(prompt).not.toContain("{{changed_files}}");
  });

  it("does not append extra fallback instructions when a customized prompt omits placeholders", () => {
    const prompt = buildChangesWalkthroughPrompt("CUSTOM_WITHOUT_PLACEHOLDER");

    expect(prompt).toMatch(/^@changes-walkthrough\n\n<kandev-system>/);
    expect(prompt).toContain("CUSTOM_WITHOUT_PLACEHOLDER");
    expect(prompt).not.toContain("Diff context:");
    expect(prompt).not.toContain("No precomputed changed-file list is supplied.");
  });
});
