"use client";

import { mergeAttributes, Node } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from "@tiptap/react";
import { IconSlash } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  formatSlashCommandDisplayLabel,
  formatSlashCommandLabel,
  slashCommandAttrsFromElement,
  slashCommandHtmlAttributes,
} from "./tiptap-slash-command-utils";

export const SlashCommandNode = Node.create({
  name: "slashCommand",
  group: "inline",
  inline: true,
  selectable: false,
  atom: true,

  addAttributes() {
    return {
      id: { default: null, rendered: false },
      label: { default: null, rendered: false },
      commandName: { default: null, rendered: false },
      description: { default: null, rendered: false },
    };
  },

  parseHTML() {
    return [
      {
        tag: "span[data-slash-command]",
        getAttrs: (element) => slashCommandAttrsFromElement(element),
      },
    ];
  },

  renderHTML({ node, HTMLAttributes }) {
    const label = formatSlashCommandLabel(node.attrs);
    return [
      "span",
      mergeAttributes(
        { "data-slash-command": "" },
        HTMLAttributes,
        slashCommandHtmlAttributes(node.attrs),
      ),
      label,
    ];
  },

  renderText({ node }) {
    return formatSlashCommandLabel(node.attrs);
  },

  addNodeView() {
    return ReactNodeViewRenderer(SlashCommandChipView);
  },
});

function SlashCommandChipView({ node }: ReactNodeViewProps) {
  const label = formatSlashCommandDisplayLabel(node.attrs);
  const description = typeof node.attrs.description === "string" ? node.attrs.description : "";

  const chip = (
    <span
      contentEditable={false}
      data-testid="slash-command-chip"
      className="inline-flex max-w-[180px] items-center gap-1 rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary ring-1 ring-inset ring-primary/25 align-baseline"
    >
      <IconSlash className="h-3 w-3 shrink-0" />
      <span className="truncate">{label}</span>
    </span>
  );

  if (!description) {
    return (
      <NodeViewWrapper as="span" className="inline">
        {chip}
      </NodeViewWrapper>
    );
  }

  return (
    <NodeViewWrapper as="span" className="inline">
      <Tooltip>
        <TooltipTrigger asChild>{chip}</TooltipTrigger>
        <TooltipContent side="top">{description}</TooltipContent>
      </Tooltip>
    </NodeViewWrapper>
  );
}
