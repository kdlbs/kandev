"use client";

import { useState } from "react";
import type { Components } from "react-markdown";
import { IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { cn } from "@/lib/utils";
import { t } from "@/lib/i18n";
import { splitWorkflowInstructions } from "@/lib/utils/workflow-instructions";

type UserMessageBodyOptions = {
  hasContent: boolean;
  showRaw: boolean;
  hasAttachments: boolean;
  content: string;
  rawContent?: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

function UserMessageMarkdown({
  content,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  content: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  return (
    <div className="markdown-body markdown-body-user max-w-none">
      <MemoizedMarkdown
        content={content}
        taskId={taskId}
        components={promptMentionComponents}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
      />
    </div>
  );
}

function CollapsedWorkflowInstructions({
  instructions,
  rest,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  instructions: string;
  rest: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  return (
    <div className="space-y-2">
      <Collapsible open={open} onOpenChange={setOpen}>
        <CollapsibleTrigger
          className="flex w-full cursor-pointer items-center gap-1 rounded-md bg-muted/40 px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted/60"
          data-testid="workflow-instructions-toggle"
        >
          <IconChevronRight
            className={cn("h-3.5 w-3.5 shrink-0 transition-transform", open && "rotate-90")}
          />
          <span>{t("workflows:workflowInstructionsCollapsed")}</span>
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-2">
          <UserMessageMarkdown
            content={instructions}
            promptMentionComponents={promptMentionComponents}
            taskId={taskId}
            worktreePath={worktreePath}
            onOpenFile={onOpenFile}
          />
        </CollapsibleContent>
      </Collapsible>
      {rest.trim() !== "" && (
        <UserMessageMarkdown
          content={rest}
          promptMentionComponents={promptMentionComponents}
          taskId={taskId}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
        />
      )}
    </div>
  );
}

export function renderUserMessageBody({
  hasContent,
  showRaw,
  hasAttachments,
  content,
  rawContent,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: UserMessageBodyOptions): React.ReactNode {
  if (hasContent && showRaw) {
    return <pre className="whitespace-pre-wrap font-mono text-xs">{rawContent || content}</pre>;
  }
  if (hasContent) {
    const split = splitWorkflowInstructions(content);
    if (split.hasInstructions) {
      return (
        <CollapsedWorkflowInstructions
          instructions={split.instructions}
          rest={split.rest}
          promptMentionComponents={promptMentionComponents}
          taskId={taskId}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
        />
      );
    }
    return (
      <UserMessageMarkdown
        content={content}
        promptMentionComponents={promptMentionComponents}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
      />
    );
  }
  if (!hasAttachments) {
    return (
      <p className="whitespace-pre-wrap break-words overflow-wrap-anywhere">{t("task:empty")}</p>
    );
  }
  return null;
}
