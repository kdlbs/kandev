import { describe, expect, it } from "vitest";
import { getMarkdownText } from "./tiptap-helpers";

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
});
