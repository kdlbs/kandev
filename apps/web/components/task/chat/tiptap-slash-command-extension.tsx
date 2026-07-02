"use client";

import { mergeAttributes, Node } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from "@tiptap/react";
import { IconSlash } from "@tabler/icons-react";

export const SlashCommandNode = Node.create({
  name: "slashCommand",
  group: "inline",
  inline: true,
  selectable: false,
  atom: true,

  addAttributes() {
    return {
      id: { default: null },
      label: { default: null },
      commandName: { default: null },
    };
  },

  parseHTML() {
    return [{ tag: "span[data-slash-command]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes({ "data-slash-command": "" }, HTMLAttributes),
      HTMLAttributes.label || HTMLAttributes.commandName || "",
    ];
  },

  renderText({ node }) {
    return node.attrs.label ?? `/${node.attrs.commandName ?? ""}`;
  },

  addNodeView() {
    return ReactNodeViewRenderer(SlashCommandChipView);
  },
});

function SlashCommandChipView({ node }: ReactNodeViewProps) {
  const label = (node.attrs.label as string | null) ?? `/${node.attrs.commandName ?? ""}`;

  return (
    <NodeViewWrapper as="span" className="inline">
      <span
        contentEditable={false}
        data-testid="slash-command-chip"
        className="inline-flex max-w-[180px] items-center gap-1 rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary ring-1 ring-inset ring-primary/25 align-baseline"
      >
        <IconSlash className="h-3 w-3 shrink-0" />
        <span className="truncate">{label}</span>
      </span>
    </NodeViewWrapper>
  );
}
