import { describe, expect, it } from "vitest";
import { remarkMathCompat } from "./remark-math-compat";

function applyCompat(tree: Record<string, unknown>, source: string) {
  const transform = remarkMathCompat();
  transform(tree as never, { toString: () => source });
  return tree;
}

describe("remarkMathCompat", () => {
  // @covers AC-UI-MARKDOWN-MATH-002.1, AC-UI-MARKDOWN-MATH-002.5
  it("restores the exact source slice for currency-like math", () => {
    const source = "Cost: $100 and $200";
    const tree = {
      type: "root",
      children: [
        {
          type: "paragraph",
          children: [
            {
              type: "text",
              value: "Cost: ",
              position: { start: { offset: 0 }, end: { offset: 6 } },
            },
            {
              type: "inlineMath",
              value: "100 and ",
              position: { start: { offset: 6 }, end: { offset: 16 } },
            },
            {
              type: "text",
              value: "200",
              position: { start: { offset: 16 }, end: { offset: 19 } },
            },
          ],
        },
      ],
    };

    applyCompat(tree, source);

    expect((tree.children as Array<{ children: unknown[] }>)[0].children[1]).toEqual({
      type: "text",
      value: "$100 and $",
      position: { start: { offset: 6 }, end: { offset: 16 } },
    });
  });

  // @covers AC-UI-MARKDOWN-MATH-001.2
  it("promotes a paragraph-only single-line double-dollar node to display math", () => {
    const source = "$$\\frac{a}{b}$$";
    const node = {
      type: "inlineMath",
      value: "\\frac{a}{b}",
      position: { start: { offset: 0 }, end: { offset: source.length } },
    };
    const tree = {
      type: "root",
      children: [{ type: "paragraph", children: [node] }],
    };

    applyCompat(tree, source);

    const display = (tree.children as Array<Record<string, unknown>>)[0];
    expect(display).toMatchObject({
      type: "math",
      position: node.position,
      data: {
        hName: "pre",
        hChildren: [
          {
            type: "element",
            tagName: "code",
            properties: { className: ["language-math", "math-display"] },
          },
        ],
      },
    });
  });

  // @covers AC-UI-MARKDOWN-MATH-002.2
  it("does not change an embedded double-dollar expression", () => {
    const source = "Before $$x$$ after";
    const node = {
      type: "inlineMath",
      value: "x",
      position: { start: { offset: 7 }, end: { offset: 12 } },
    };
    const tree = {
      type: "root",
      children: [
        {
          type: "paragraph",
          children: [
            {
              type: "text",
              value: "Before ",
              position: { start: { offset: 0 }, end: { offset: 7 } },
            },
            node,
            {
              type: "text",
              value: " after",
              position: { start: { offset: 12 }, end: { offset: 18 } },
            },
          ],
        },
      ],
    };

    applyCompat(tree, source);

    expect((tree.children as Array<{ children: unknown[] }>)[0].children[1]).toBe(node);
  });
});
