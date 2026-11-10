type MarkdownNode = {
  type: string;
  value?: string;
  url?: string;
  children?: MarkdownNode[];
};

const SKIP_NODE_TYPES = new Set(["code", "inlineCode", "link", "linkReference"]);

// Only identifiers carrying the workspace's task prefix are Kandev tasks;
// other tracker keys (Jira, Linear) must stay plain text.
function identifierPattern(prefix: string): RegExp {
  const escaped = prefix.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`\\b(${escaped}-\\d+)\\b`, "g");
}

export function splitTaskIdentifierText(value: string, prefix?: string): MarkdownNode[] {
  if (!prefix) return [{ type: "text", value }];
  const re = identifierPattern(prefix);
  const nodes: MarkdownNode[] = [];
  let last = 0;

  for (let match = re.exec(value); match; match = re.exec(value)) {
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

export function remarkTaskLinks({ prefix }: { prefix?: string } = {}) {
  return function transformer(tree: MarkdownNode) {
    transformTaskLinks(tree, prefix);
    return tree;
  };
}

function transformTaskLinks(node: MarkdownNode, prefix?: string) {
  if (!node.children || SKIP_NODE_TYPES.has(node.type)) return;

  for (let i = 0; i < node.children.length; i++) {
    const child = node.children[i];
    if (child.type === "text" && child.value) {
      const replacement = splitTaskIdentifierText(child.value, prefix);
      if (replacement.length > 1 || replacement[0].type !== "text") {
        node.children.splice(i, 1, ...replacement);
        i += replacement.length - 1;
      }
      continue;
    }
    transformTaskLinks(child, prefix);
  }
}
