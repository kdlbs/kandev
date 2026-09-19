"use client";

import { useMemo, useState } from "react";
import type { Components } from "react-markdown";
import { IconChevronRight } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@kandev/ui/collapsible";
import { MemoizedMarkdown } from "@/components/shared/memoized-markdown";
import { BoundedMessagePreview } from "./bounded-message-preview";
import { cn } from "@/lib/utils";
import { t } from "@/lib/i18n";
import {
  getMessagePreview,
  MESSAGE_PREVIEW_MAX_CODE_UNITS,
  MESSAGE_PREVIEW_MAX_LINES,
} from "@/lib/utils/message-preview";
import { splitMessageSegments } from "@/lib/utils/workflow-instructions";

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

function CollapsedInstructions({
  label,
  testId,
  instructions,
  promptMentionComponents,
  taskId,
  worktreePath,
  onOpenFile,
}: {
  label: string;
  testId: string;
  instructions: string;
  promptMentionComponents?: Components;
  taskId: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <CollapsibleTrigger
        className="flex w-full cursor-pointer items-center gap-1 rounded-md bg-muted/40 px-2 py-1 text-left text-xs text-muted-foreground hover:bg-muted/60"
        data-testid={testId}
      >
        <IconChevronRight
          className={cn("h-3.5 w-3.5 shrink-0 transition-transform", open && "rotate-90")}
        />
        <span>{label}</span>
      </CollapsibleTrigger>
      <CollapsibleContent className="pt-2">
        <BoundedMessagePreview
          source={instructions}
          fileName="kandev-workflow-instructions.txt"
          renderContent={(preview) => (
            <UserMessageMarkdown
              content={preview}
              promptMentionComponents={promptMentionComponents}
              taskId={taskId}
              worktreePath={worktreePath}
              onOpenFile={onOpenFile}
            />
          )}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

function MessageSegments({
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
  const { t } = useTranslation();
  const segments = useMemo(() => {
    let remainingLines = MESSAGE_PREVIEW_MAX_LINES;
    let remainingCodeUnits = MESSAGE_PREVIEW_MAX_CODE_UNITS;
    return splitMessageSegments(content).map((segment) => {
      if (segment.type === "instructions") {
        return { segment, preview: getMessagePreview(segment.content) };
      }
      const preview = getMessagePreview(segment.content, {
        maxLines: remainingLines,
        maxCodeUnits: remainingCodeUnits,
      });
      remainingLines = Math.max(0, remainingLines - preview.logicalLines);
      remainingCodeUnits = Math.max(0, remainingCodeUnits - preview.codeUnits);
      return { segment, preview };
    });
  }, [content]);
  return (
    <div className="space-y-2">
      {segments.map(({ segment, preview }, index) => {
        if (segment.type === "text") {
          return (
            <BoundedMessagePreview
              key={`text-${index}`}
              source={segment.content}
              downloadSource={content}
              fileName="kandev-message.txt"
              preview={preview}
              renderContent={(previewContent) => (
                <UserMessageMarkdown
                  content={previewContent}
                  promptMentionComponents={promptMentionComponents}
                  taskId={taskId}
                  worktreePath={worktreePath}
                  onOpenFile={onOpenFile}
                />
              )}
            />
          );
        }
        const isMove = segment.kind === "move";
        return (
          <CollapsedInstructions
            key={`instructions-${segment.kind}-${index}`}
            label={t(
              isMove
                ? "workflows:workflowMoveInstructionsCollapsed"
                : "workflows:workflowInstructionsCollapsed",
            )}
            testId={isMove ? "workflow-move-instructions-toggle" : "workflow-instructions-toggle"}
            instructions={segment.content}
            promptMentionComponents={promptMentionComponents}
            taskId={taskId}
            worktreePath={worktreePath}
            onOpenFile={onOpenFile}
          />
        );
      })}
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
    const raw = rawContent || content;
    return (
      <BoundedMessagePreview
        source={raw}
        fileName="kandev-message.txt"
        renderContent={(preview) => (
          <pre className="whitespace-pre-wrap font-mono text-xs">{preview}</pre>
        )}
      />
    );
  }
  if (hasContent) {
    return (
      <MessageSegments
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
