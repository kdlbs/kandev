import { describe, expect, it } from "vitest";
import { splitPromptMentionSegments } from "./prompt-mention-segments";

describe("splitPromptMentionSegments", () => {
  it("marks stored prompt mentions without marking unknown mentions", () => {
    expect(splitPromptMentionSegments("Run @hello and @missing", ["hello"])).toEqual([
      { kind: "text", value: "Run " },
      { kind: "prompt", value: "@hello", name: "hello" },
      { kind: "text", value: " and @missing" },
    ]);
  });

  it("matches stored prompt names with spaces longest-first", () => {
    expect(splitPromptMentionSegments("Use @Daily Summary.", ["Daily", "Daily Summary"])).toEqual([
      { kind: "text", value: "Use " },
      { kind: "prompt", value: "@Daily Summary", name: "Daily Summary" },
      { kind: "text", value: "." },
    ]);
  });
});
