import { describe, expect, it } from "vitest";
import { getMarkdownText, textToEditorContent } from "./tiptap-helpers";

describe("getMarkdownText", () => {
  it("serializes slash command chips as slash command text", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "please run " },
                { type: "slashCommand", attrs: { label: "/slow" } },
                { type: "text", text: " 1s" },
              ],
            },
          ],
        }),
      }),
    ).toBe("please run /slow 1s");
  });

  it("serializes slash command chips from commandName-only attrs", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [
                { type: "text", text: "please run " },
                { type: "slashCommand", attrs: { commandName: "slow" } },
              ],
            },
          ],
        }),
      }),
    ).toBe("please run /slow");
  });

  it("normalizes slash command chip labels when serializing", () => {
    expect(
      getMarkdownText({
        getJSON: () => ({
          content: [
            {
              type: "paragraph",
              content: [{ type: "slashCommand", attrs: { label: "slow" } }],
            },
          ],
        }),
      }),
    ).toBe("/slow");
  });
});

describe("textToEditorContent", () => {
  const slowCommand = {
    id: "agent-slow",
    label: "/slow",
    description: "Run a slow response",
    action: "agent" as const,
    agentCommandName: "slow",
  };

  it("restores known slash commands as slash command nodes", () => {
    const content = textToEditorContent("/slow 1s", [slowCommand]);

    expect(content).toEqual({
      type: "doc",
      content: [
        {
          type: "paragraph",
          content: [
            {
              type: "slashCommand",
              attrs: {
                id: "agent-slow",
                label: "/slow",
                commandName: "slow",
                description: "Run a slow response",
              },
            },
            { type: "text", text: " 1s" },
          ],
        },
      ],
    });
  });

  it("does not restore slash command chips inside recalled code fences", () => {
    const content = textToEditorContent("```python\n/slow arg\n```", [slowCommand]);

    expect(JSON.stringify(content)).not.toContain('"slashCommand"');
    expect(content).toEqual({
      type: "doc",
      content: [
        { type: "paragraph", content: [{ type: "text", text: "```python" }] },
        { type: "paragraph", content: [{ type: "text", text: "/slow arg" }] },
        { type: "paragraph", content: [{ type: "text", text: "```" }] },
      ],
    });
  });
});
