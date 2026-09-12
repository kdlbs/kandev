"use client";

import { Node, mergeAttributes } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from "@tiptap/react";
import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { PromptMentionChip } from "@/components/task/chat/messages/prompt-mention-components";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { cn } from "@/lib/utils";

export type TaskPromptReferenceAttrs = {
  name: string;
  value: string;
};

export const TaskPromptReference = Node.create({
  name: "promptReference",
  group: "inline",
  inline: true,
  atom: true,
  selectable: true,

  addAttributes() {
    return {
      name: { default: "" },
      value: { default: "" },
    };
  },

  parseHTML() {
    return [{ tag: "span[data-task-prompt-reference]" }];
  },

  renderHTML({ HTMLAttributes }) {
    return [
      "span",
      mergeAttributes({ "data-task-prompt-reference": "" }, HTMLAttributes),
      HTMLAttributes.value || HTMLAttributes.name || "",
    ];
  },

  renderText({ node }) {
    return String(node.attrs.value || node.attrs.name || "");
  },

  addNodeView() {
    return ReactNodeViewRenderer(TaskPromptReferenceView);
  },
});

function TaskPromptReferenceView({ node, deleteNode }: ReactNodeViewProps) {
  const { t } = useTranslation();
  const usesTouchDrawer = useTouchDrawer();
  const attrs = node.attrs as TaskPromptReferenceAttrs;
  const removeLabel = t("task:removeLabeled", { label: attrs.value });

  return (
    <NodeViewWrapper
      as="span"
      data-testid="task-prompt-reference"
      data-prompt-name={attrs.name}
      className="inline-flex max-w-full items-center align-baseline"
    >
      <PromptMentionChip name={attrs.name} value={attrs.value} />
      <button
        type="button"
        data-testid="task-prompt-reference-remove"
        aria-label={removeLabel}
        title={removeLabel}
        contentEditable={false}
        className={cn(
          "inline-flex h-7 w-7 shrink-0 cursor-pointer items-center justify-center rounded-md text-muted-foreground hover:bg-muted/50 hover:text-foreground",
          usesTouchDrawer && "h-11 min-w-11",
        )}
        onMouseDown={(event) => {
          event.preventDefault();
          event.stopPropagation();
        }}
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          deleteNode();
        }}
      >
        <IconX className="h-4 w-4" />
      </button>
    </NodeViewWrapper>
  );
}
