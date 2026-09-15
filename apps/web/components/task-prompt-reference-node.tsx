"use client";

import { Node, mergeAttributes } from "@tiptap/core";
import { NodeViewWrapper, ReactNodeViewRenderer, type ReactNodeViewProps } from "@tiptap/react";
import { IconX } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { PromptMentionChip } from "@/components/task/chat/messages/prompt-mention-components";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
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
  const { isMobile } = useResponsiveBreakpoint();
  const usesTouchTarget = isMobile || usesTouchDrawer;
  const attrs = node.attrs as TaskPromptReferenceAttrs;
  const removeLabel = t("task:removeLabeled", { label: attrs.value });

  return (
    <NodeViewWrapper
      as="span"
      data-testid="task-prompt-reference"
      data-prompt-name={attrs.name}
      className={cn(
        "inline-flex max-w-full min-w-0 box-border items-center gap-0.5 rounded-md border border-emerald-300/35 bg-emerald-400/20 px-1 align-baseline",
        usesTouchTarget ? "min-h-11" : "h-6",
      )}
    >
      <PromptMentionChip name={attrs.name} value={attrs.value} presentation="editable" />
      <button
        type="button"
        data-testid="task-prompt-reference-remove"
        aria-label={removeLabel}
        title={removeLabel}
        contentEditable={false}
        className={cn(
          "inline-flex h-5 w-5 shrink-0 cursor-pointer items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-emerald-500/15 hover:text-foreground focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-emerald-500/80",
          usesTouchTarget && "h-11 min-h-11 min-w-11 w-11",
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
