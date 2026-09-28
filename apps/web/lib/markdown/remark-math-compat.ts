type MarkdownNode = {
  type: string;
  value?: string;
  children?: MarkdownNode[];
  data?: Record<string, unknown>;
  position?: {
    start?: { offset?: number };
    end?: { offset?: number };
  };
};

type MarkdownFile = {
  toString(): string;
};

function sourceSlice(node: MarkdownNode, source: string): string | null {
  const start = node.position?.start?.offset;
  const end = node.position?.end?.offset;
  if (start === undefined || end === undefined) return null;
  return source.slice(start, end);
}

function textNode(node: MarkdownNode, value: string): MarkdownNode {
  return { type: "text", value, position: node.position };
}

function displayMathNode(node: MarkdownNode): MarkdownNode {
  return {
    type: "math",
    value: node.value,
    position: node.position,
    data: {
      hName: "pre",
      hChildren: [
        {
          type: "element",
          tagName: "code",
          properties: { className: ["language-math", "math-display"] },
          children: [{ type: "text", value: node.value ?? "" }],
        },
      ],
    },
  };
}

function hasWhitespaceAtMathBoundary(raw: string): boolean {
  const inner = raw.slice(1, -1);
  return /^\s|\s$/u.test(inner);
}

function isSingleLineDisplay(node: MarkdownNode, raw: string, parent: MarkdownNode): boolean {
  return (
    node.type === "inlineMath" &&
    parent.type === "paragraph" &&
    parent.children?.length === 1 &&
    raw.startsWith("$$") &&
    raw.endsWith("$$") &&
    !raw.includes("\n")
  );
}

function transformChildren(node: MarkdownNode, source: string): void {
  if (!node.children) return;

  const transformed: MarkdownNode[] = [];
  for (const child of node.children) {
    if (child.type === "paragraph" && child.children?.length === 1) {
      const onlyChild = child.children[0];
      const raw = sourceSlice(onlyChild, source);
      if (raw && isSingleLineDisplay(onlyChild, raw, child)) {
        transformed.push(displayMathNode(onlyChild));
        continue;
      }
    }

    if (child.type === "inlineMath") {
      const raw = sourceSlice(child, source);
      if (raw && !raw.startsWith("$$") && hasWhitespaceAtMathBoundary(raw)) {
        transformed.push(textNode(child, raw));
        continue;
      }
    }

    transformChildren(child, source);
    transformed.push(child);
  }
  node.children = transformed;
}

/** Restore parsed dollar pairs whose edge whitespace makes them literal text. */
export function remarkMathCompat() {
  return (tree: MarkdownNode, file: MarkdownFile) => {
    transformChildren(tree, file.toString());
  };
}
