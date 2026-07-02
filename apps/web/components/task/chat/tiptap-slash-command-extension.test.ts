import { describe, expect, it } from "vitest";
import { SlashCommandNode } from "./tiptap-slash-command-extension";
import { formatSlashCommandDisplayLabel } from "./tiptap-slash-command-utils";

const SLOW_COMMAND_NAME = "slow";
const SLOW_COMMAND_LABEL = "/slow";
const SLOW_COMMAND_DESCRIPTION = "Run slowly";

type SlashCommandNodeConfig = {
  parseHTML: () => Array<{
    tag: string;
    getAttrs?: (element: Element) => Record<string, unknown>;
  }>;
  renderHTML: (args: {
    node: { attrs: Record<string, unknown> };
    HTMLAttributes: Record<string, unknown>;
  }) => [string, Record<string, unknown>, string];
  renderText: (args: { node: { attrs: Record<string, unknown> } }) => string;
};

const config = SlashCommandNode.config as unknown as SlashCommandNodeConfig;

describe("SlashCommandNode", () => {
  it("renders text from label when present", () => {
    expect(
      config.renderText({
        node: { attrs: { label: SLOW_COMMAND_LABEL, commandName: "fast" } },
      }),
    ).toBe(SLOW_COMMAND_LABEL);
  });

  it("renders text from commandName when label is missing", () => {
    expect(config.renderText({ node: { attrs: { commandName: SLOW_COMMAND_NAME } } })).toBe(
      SLOW_COMMAND_LABEL,
    );
  });

  it("renders HTML with slash-prefixed fallback text and data attrs", () => {
    const html = config.renderHTML({
      node: {
        attrs: {
          id: "cmd-1",
          commandName: SLOW_COMMAND_NAME,
          description: SLOW_COMMAND_DESCRIPTION,
        },
      },
      HTMLAttributes: { class: "chip" },
    });

    expect(html[0]).toBe("span");
    expect(html[1]).toMatchObject({
      class: "chip",
      "data-slash-command": "",
      "data-id": "cmd-1",
      "data-command-name": SLOW_COMMAND_NAME,
      "data-description": SLOW_COMMAND_DESCRIPTION,
    });
    expect(html[2]).toBe(SLOW_COMMAND_LABEL);
  });

  it("formats the visible chip label without a leading slash", () => {
    expect(
      formatSlashCommandDisplayLabel({
        label: SLOW_COMMAND_LABEL,
        commandName: SLOW_COMMAND_NAME,
      }),
    ).toBe(SLOW_COMMAND_NAME);
    expect(formatSlashCommandDisplayLabel({ commandName: "fast" })).toBe("fast");
  });

  it("parses slash command attrs from HTML data attrs", () => {
    const element = document.createElement("span");
    element.setAttribute("data-slash-command", "");
    element.setAttribute("data-id", "cmd-1");
    element.setAttribute("data-label", SLOW_COMMAND_LABEL);
    element.setAttribute("data-command-name", SLOW_COMMAND_NAME);
    element.setAttribute("data-description", SLOW_COMMAND_DESCRIPTION);

    const parser = config.parseHTML()[0];

    expect(parser.tag).toBe("span[data-slash-command]");
    expect(parser.getAttrs?.(element)).toEqual({
      id: "cmd-1",
      label: SLOW_COMMAND_LABEL,
      commandName: SLOW_COMMAND_NAME,
      description: SLOW_COMMAND_DESCRIPTION,
    });
  });
});
