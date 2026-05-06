const TASK_IDENTIFIER_RE = /\b([A-Z]+-\d+)\b/g;

type MarkdownNode = {
  type: string;
  value?: string;
  url?: string;
  children?: MarkdownNode[];
};

const SKIP_NODE_TYPES = new Set(["code", "inlineCode", "link", "linkReference"]);

export function splitTaskIdentifierText(value: string): MarkdownNode[] {
  const nodes: MarkdownNode[] = [];
  let last = 0;
  TASK_IDENTIFIER_RE.lastIndex = 0;

  for (let match = TASK_IDENTIFIER_RE.exec(value); match; match = TASK_IDENTIFIER_RE.exec(value)) {
    const identifier = match[1];
    if (match.index > last) {
      nodes.push({ type: "text", value: value.slice(last, match.index) });
    }
    nodes.push({
      type: "link",
      url: `/office/tasks/${identifier}`,
      children: [{ type: "text", value: identifier }],
    });
    last = match.index + match[0].length;
  }

  if (nodes.length === 0) return [{ type: "text", value }];
  if (last < value.length) {
    nodes.push({ type: "text", value: value.slice(last) });
  }
  return nodes;
}

export function remarkTaskLinks() {
  return function transformer(tree: MarkdownNode) {
    transformTaskLinks(tree);
    return tree;
  };
}

function transformTaskLinks(node: MarkdownNode) {
  if (!node.children || SKIP_NODE_TYPES.has(node.type)) return;

  for (let i = 0; i < node.children.length; i++) {
    const child = node.children[i];
    if (child.type === "text" && child.value) {
      const replacement = splitTaskIdentifierText(child.value);
      if (replacement.length > 1 || replacement[0].type !== "text") {
        node.children.splice(i, 1, ...replacement);
        i += replacement.length - 1;
      }
      continue;
    }
    transformTaskLinks(child);
  }
}
