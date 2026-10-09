import rehypeSanitize from "rehype-sanitize";
import { describe, expect, it } from "vitest";
import { markdownMathSanitizeSchema } from "./math-sanitize-schema";

type TestElement = {
  properties?: {
    className?: unknown;
  };
};

// @covers AC-UI-MARKDOWN-MATH-002.4
describe("markdownMathSanitizeSchema", () => {
  it("preserves both math marker classes while removing unrelated classes", () => {
    const tree = {
      type: "root",
      children: [
        {
          type: "element",
          tagName: "code",
          properties: {
            className: ["language-math", "math-inline", "math-display", "unsafe-class"],
          },
          children: [],
        },
      ],
    };

    const sanitized = rehypeSanitize(markdownMathSanitizeSchema)(tree as never) as typeof tree;

    const code = sanitized.children[0] as TestElement;
    expect(code.properties?.className).toEqual(["language-math", "math-inline", "math-display"]);
  });
});
