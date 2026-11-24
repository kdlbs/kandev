/** Recognize the explicit agent reference format without normalizing other URLs. */
export function agentTaskReferenceId(href: string | undefined): string | null {
  if (!href || href.length > 259) return null;
  return /^\/t\/([a-zA-Z0-9_-]{1,256})$/.exec(href)?.[0] === href ? href.slice(3) : null;
}

type ReferenceLabelNode = {
  type: string;
  value?: string;
  tagName?: string;
  properties?: Record<string, unknown>;
  children?: ReferenceLabelNode[];
};

/** Only renderer-owned motion spans may wrap a plain reference label. */
function readLabel(nodes: readonly ReferenceLabelNode[]): string | null {
  let label = "";
  for (const node of nodes) {
    if (node.type === "text") {
      label += node.value ?? "";
    } else if (
      node.type === "element" &&
      node.tagName === "span" &&
      typeof node.properties?.["data-chat-text-motion"] === "number"
    ) {
      const text = readLabel(node.children ?? []);
      if (text === null) return null;
      label += text;
    } else {
      return null;
    }
  }
  return label;
}

export function agentTaskReferenceLabel(nodes: readonly ReferenceLabelNode[]): string | null {
  const label = readLabel(nodes);
  return label?.trim() ? label : null;
}
