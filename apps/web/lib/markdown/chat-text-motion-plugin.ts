import { splitMotionText, type TextRun } from "./chat-text-motion";

type MotionNode = {
  type: string;
  tagName?: string;
  value?: string;
  properties?: Record<string, unknown>;
  children?: MotionNode[];
  position?: { start: { offset?: number } };
};

/** A chat-only HAST transform; normalized source offsets guard decoded text. */
export function chatTextMotionPlugin(source: string, runs: TextRun[]) {
  return () => (tree: MotionNode) => {
    const visit = (node: MotionNode) => {
      if (!node.children || node.tagName === "code" || node.tagName === "pre") return;
      node.children = node.children.flatMap((child): MotionNode[] => {
        if (child.type !== "text" || !child.value) {
          visit(child);
          return [child];
        }
        return splitMotionText(child.value, child.position?.start.offset, source, runs).map(
          (part) =>
            part.receivedAt === undefined
              ? { type: "text", value: part.text }
              : {
                  type: "element",
                  tagName: "span",
                  properties: { "data-chat-text-motion": part.receivedAt },
                  children: [{ type: "text", value: part.text }],
                },
        );
      });
    };
    visit(tree);
  };
}
