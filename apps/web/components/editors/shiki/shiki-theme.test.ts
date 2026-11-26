import { expect, it } from "vitest";
import { highlightCode } from "./shiki-highlighter";
it("renders imported token colors in the real highlighter", async () => {
  const html = await highlightCode('const name = "hello";', "javascript", true, {
    reference: "theme:test/night",
    base: "dark",
    digest: "abcdef",
    status: "ready",
    definition: {
      schema_version: 1,
      base: "dark",
      editor: { background: "#272822" },
      syntax: { keyword: { foreground: "#f92672" } },
    },
  });
  expect(html.toLowerCase()).toContain("#f92672");
  expect(html.toLowerCase()).toContain("#272822");
});
