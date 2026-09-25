import { describe, expect, it } from "vitest";
import { MATH_SOURCE_CLASS, rehypeMathSource } from "./rehype-math-source";

type TestNode = {
  type: string;
  tagName?: string;
  properties?: Record<string, unknown>;
  children?: TestNode[];
  position?: unknown;
};

function applySourceWrapper(tree: TestNode): TestNode {
  rehypeMathSource()(tree as never);
  return tree;
}

function displayPre(position?: TestNode["position"], children?: TestNode[]): TestNode {
  return {
    type: "element",
    tagName: "pre",
    ...(position ? { position } : {}),
    children: children ?? [
      {
        type: "element",
        tagName: "code",
        properties: { className: ["language-math"] },
        children: [],
      },
    ],
  };
}

describe("rehypeMathSource", () => {
  it("wraps a display math pre and preserves its source position", () => {
    const position = {
      start: { line: 3, column: 1, offset: 20 },
      end: { line: 5, column: 3, offset: 48 },
    };
    const pre = displayPre(position);
    const tree: TestNode = { type: "root", children: [pre] };

    applySourceWrapper(tree);

    const wrapper = tree.children?.[0];
    expect(wrapper).toMatchObject({
      type: "element",
      tagName: "div",
      properties: { className: [MATH_SOURCE_CLASS] },
      position,
    });
    expect(wrapper?.children).toEqual([pre]);
  });

  it.each([
    ["an empty children array", displayPre(undefined, [])],
    [
      "a pre with multiple children",
      displayPre(undefined, [
        {
          type: "element",
          tagName: "code",
          properties: { className: ["math-display"] },
          children: [],
        },
        { type: "text", children: [] },
      ]),
    ],
  ])("leaves %s unchanged", (_description, pre) => {
    const tree: TestNode = { type: "root", children: [pre] };

    applySourceWrapper(tree);

    expect(tree.children).toEqual([pre]);
  });

  it("wraps display math without inventing a source position", () => {
    const pre = displayPre();
    const tree: TestNode = { type: "root", children: [pre] };

    applySourceWrapper(tree);

    expect(tree.children?.[0]).toMatchObject({
      tagName: "div",
      position: undefined,
      children: [pre],
    });
  });
});
