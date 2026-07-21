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

  it("matches stored prompt mentions at the start of the string", () => {
    expect(splitPromptMentionSegments("@hello world", ["hello"])).toEqual([
      { kind: "prompt", value: "@hello", name: "hello" },
      { kind: "text", value: " world" },
    ]);
  });

  it("matches stored prompt mentions at the end of the string", () => {
    expect(splitPromptMentionSegments("Run @hello", ["hello"])).toEqual([
      { kind: "text", value: "Run " },
      { kind: "prompt", value: "@hello", name: "hello" },
    ]);
  });

  it("returns a text segment when content is empty", () => {
    expect(splitPromptMentionSegments("", ["hello"])).toEqual([{ kind: "text", value: "" }]);
  });

  it("returns a text segment when no prompt names are provided", () => {
    expect(splitPromptMentionSegments("@hello", [])).toEqual([{ kind: "text", value: "@hello" }]);
  });
});
