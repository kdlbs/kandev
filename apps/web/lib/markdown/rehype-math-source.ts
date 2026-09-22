type RehypeNode = {
  type: string;
  tagName?: string;
  properties?: Record<string, unknown>;
  children?: RehypeNode[];
  position?: RehypePosition;
};

type RehypePosition = {
  start: { line: number; column: number; offset?: number };
  end: { line: number; column: number; offset?: number };
  indent?: number[];
};

const MATH_SOURCE_CLASS = "markdown-math-source";

function hasClass(node: RehypeNode | undefined, className: string): boolean {
  const value = node?.properties?.className;
  if (Array.isArray(value)) return value.includes(className);
  if (typeof value === "string") return value.split(/\s+/u).includes(className);
  return false;
}

function isDisplayMathPre(node: RehypeNode): boolean {
  const code = node.children?.[0];
  return (
    node.tagName === "pre" &&
    node.children?.length === 1 &&
    code?.tagName === "code" &&
    (hasClass(code, "language-math") || hasClass(code, "math-display"))
  );
}

function wrapDisplayMath(node: RehypeNode, source: RehypeNode): RehypeNode {
  return {
    type: "element",
    tagName: "div",
    properties: { className: [MATH_SOURCE_CLASS] },
    children: [node],
    position: source.position,
  };
}

function visit(node: RehypeNode): void {
  if (!node.children) return;

  for (let index = 0; index < node.children.length; index += 1) {
    const child = node.children[index];
    if (isDisplayMathPre(child)) {
      node.children[index] = wrapDisplayMath(child, child);
      continue;
    }
    visit(child);
  }
}

/** Preserve display-math source positions before rehype-katex replaces `<pre>`. */
export function rehypeMathSource() {
  return (tree: RehypeNode) => {
    visit(tree);
  };
}

export { MATH_SOURCE_CLASS };
