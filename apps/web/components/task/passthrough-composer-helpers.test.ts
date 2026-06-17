import { describe, expect, it } from "vitest";
import {
  buildPassthroughCommands,
  detectPassthroughSuggestion,
  fileReferenceToken,
  filterPassthroughCommands,
  replacePassthroughRange,
} from "./passthrough-composer-helpers";

describe("passthrough composer helpers", () => {
  it("detects slash commands only at token boundaries", () => {
    expect(detectPassthroughSuggestion("/", 1)).toEqual({
      kind: "command",
      triggerStart: 0,
      query: "",
    });
    expect(detectPassthroughSuggestion("run /help", 9)).toEqual({
      kind: "command",
      triggerStart: 4,
      query: "help",
    });
    expect(detectPassthroughSuggestion("src/app", 7)).toBeNull();
  });

  it("filters agent commands and excludes bundled commands", () => {
    const commands = buildPassthroughCommands([
      { name: "review", description: "Review changes" },
      { name: "cost", description: "Show cost (bundled)" },
      { name: "resume" },
    ]);

    expect(commands.map((cmd) => cmd.label)).toEqual(["/review", "/resume"]);
    expect(filterPassthroughCommands(commands, "rev").map((cmd) => cmd.agentCommandName)).toEqual([
      "review",
    ]);
  });

  it("detects file mentions and replaces the typed token with an @file reference", () => {
    const value = "check @src/au please";
    const suggestion = detectPassthroughSuggestion(value, "check @src/au".length);

    expect(suggestion).toEqual({ kind: "file", triggerStart: 6, query: "src/au" });
    const next = replacePassthroughRange(
      value,
      suggestion!.triggerStart,
      "check @src/au".length,
      fileReferenceToken("src/auth/login.ts"),
    );

    expect(next.value).toBe("check @src/auth/login.ts please");
    expect(next.caret).toBe("check @src/auth/login.ts ".length);
  });

  it("formats dropped files as newline-separated references", () => {
    const inserted = ["logs/error.txt", "src/main.go"].map(fileReferenceToken).join("\n");
    const next = replacePassthroughRange("see ", 4, 4, inserted);

    expect(next.value).toBe("see @logs/error.txt \n@src/main.go ");
  });
});
