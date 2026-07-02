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
  it("restores known slash commands as slash command nodes", () => {
    const content = textToEditorContent("/slow 1s", [
      {
        id: "agent-slow",
        label: "/slow",
        description: "Run a slow response",
        action: "agent",
        agentCommandName: "slow",
      },
    ]);

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
});
