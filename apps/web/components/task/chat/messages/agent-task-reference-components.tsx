"use client";

import type { ComponentProps } from "react";
import type { Components, ExtraProps } from "react-markdown";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { markdownComponents } from "@/components/shared/markdown-components";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { agentTaskReferenceId, agentTaskReferenceLabel } from "@/lib/tasks/agent-task-reference";
import type { TaskMentionReference } from "@/lib/tasks/task-mention-references";
import { AgentTaskReferenceDetails } from "./agent-task-reference-details";
import { TaskMentionChip } from "./task-mention-components";

function AgentTaskReference({ reference }: { reference: TaskMentionReference }) {
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const chip = <TaskMentionChip reference={reference} prefix="" />;
  if (isMobile || !isFinePointer) return chip;
  return (
    <Tooltip delayDuration={200}>
      <TooltipTrigger asChild>{chip}</TooltipTrigger>
      <TooltipContent className="w-[22rem] max-w-[calc(100vw-2rem)] p-3 shadow-md motion-reduce:animate-none">
        <AgentTaskReferenceDetails reference={reference} />
      </TooltipContent>
    </Tooltip>
  );
}

function AgentReferenceAnchor(props: ComponentProps<"a"> & ExtraProps) {
  const taskId = agentTaskReferenceId(props.href);
  const title = props.node ? agentTaskReferenceLabel(props.node.children) : null;
  if (taskId && title) return <AgentTaskReference reference={{ taskId, title }} />;
  return <markdownComponents.a {...props} />;
}

export const agentTaskReferenceComponents: Components = {
  ...markdownComponents,
  a: AgentReferenceAnchor,
};
