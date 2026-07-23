"use client";

import { useState, useCallback, memo } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { SubagentTaskPayload, ToolCallMetadata } from "@/components/task/chat/types";
import { SubagentMetaRow } from "@/components/task/chat/messages/subagent-meta-row";

type ToolSubagentMessageProps = {
  comment: Message;
  childMessages: Message[];
  isContainingTurnActive?: boolean;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  renderChild: (message: Message) => React.ReactNode;
};

// Custom comparison function that ignores renderChild (it's always recreated but referentially stable in behavior)
function arePropsEqual(
  prevProps: ToolSubagentMessageProps,
  nextProps: ToolSubagentMessageProps,
): boolean {
  return (
    prevProps.comment === nextProps.comment &&
    prevProps.childMessages === nextProps.childMessages &&
    prevProps.isContainingTurnActive === nextProps.isContainingTurnActive &&
    prevProps.worktreePath === nextProps.worktreePath &&
    prevProps.onOpenFile === nextProps.onOpenFile
  );
}

const TERMINAL_TOOL_STATUSES = new Set([
  "complete",
  "completed",
  "success",
  "error",
  "failed",
  "cancelled",
]);

export function isSubagentEffectivelyActive(
  metadata: ToolCallMetadata | undefined,
  isContainingTurnActive: boolean,
): boolean {
  const status = metadata?.status;
  if (status && TERMINAL_TOOL_STATUSES.has(status)) return false;
  if (status === "running") return true;
  const payloadStatus = metadata?.normalized?.subagent_task?.status;
  return isContainingTurnActive && (status === "in_progress" || payloadStatus === "started");
}

function deriveSubagentBody(
  childCount: number,
  subagentTask: SubagentTaskPayload | undefined,
): { resultText?: string; prompt?: string } {
  if (childCount > 0) return {};
  if (subagentTask?.result_text) return { resultText: subagentTask.result_text };
  if (subagentTask?.prompt) return { prompt: subagentTask.prompt };
  return {};
}

function deriveSubagentDisplay(
  metadata: ToolCallMetadata | undefined,
  childCount: number,
  isContainingTurnActive: boolean,
) {
  const subagentTask = metadata?.normalized?.subagent_task;
  const isActive = isSubagentEffectivelyActive(metadata, isContainingTurnActive);
  const { resultText, prompt } = deriveSubagentBody(childCount, subagentTask);
  return {
    subagentTask,
    isActive,
    resultText,
    prompt,
    hasExpandableContent: childCount > 0 || Boolean(resultText) || Boolean(prompt) || isActive,
  };
}

type SubagentHeaderProps = {
  isExpanded: boolean;
  subagentType: string;
  description: string;
  isActive: boolean;
  childCount: number;
  hasExpandableContent: boolean;
  onToggle: () => void;
};

function SubagentHeader({
  isExpanded,
  subagentType,
  description,
  isActive,
  childCount,
  hasExpandableContent,
  onToggle,
}: SubagentHeaderProps) {
  const content = (
    <>
      {hasExpandableContent &&
        (isExpanded ? (
          <IconChevronDown
            data-testid="subagent-chevron"
            className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0"
          />
        ) : (
          <IconChevronRight
            data-testid="subagent-chevron"
            className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0"
          />
        ))}
      <span
        data-testid="subagent-type"
        className="bg-muted text-muted-foreground text-[10px] px-1.5 rounded font-medium uppercase tracking-wide whitespace-nowrap flex-shrink-0"
      >
        {subagentType}
      </span>
      <span
        data-testid="subagent-description"
        title={description}
        className="font-mono text-xs truncate text-muted-foreground inline-flex items-center gap-1.5 min-w-0"
      >
        {description}
        {isActive && <GridSpinner className="text-muted-foreground shrink-0" />}
      </span>
      {childCount > 0 && (
        <span
          data-testid="subagent-child-count"
          className="text-muted-foreground/60 text-xs px-1.5 rounded min-w-[20px] text-center font-mono whitespace-nowrap"
        >
          {childCount} tool call{childCount !== 1 ? "s" : ""}
        </span>
      )}
    </>
  );
  if (!hasExpandableContent) {
    return (
      <div
        data-testid="subagent-header"
        className="flex items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded"
      >
        {content}
      </div>
    );
  }
  return (
    <button
      type="button"
      aria-expanded={isExpanded}
      onClick={onToggle}
      className={cn(
        "flex min-h-11 items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded sm:min-h-0",
        "hover:bg-muted/30 transition-colors cursor-pointer",
      )}
    >
      {content}
    </button>
  );
}

const NESTED_BORDER = "ml-2 pl-4 border-l-2 border-border/30 mt-1";

type SubagentContentProps = {
  isExpanded: boolean;
  childMessages: Message[];
  isActive: boolean;
  prompt?: string;
  resultText?: string;
  renderChild: (message: Message) => React.ReactNode;
};

function SubagentContent({
  isExpanded,
  childMessages,
  isActive,
  prompt,
  resultText,
  renderChild,
}: SubagentContentProps) {
  if (!isExpanded) return null;
  if (childMessages.length > 0) {
    return (
      <div className={cn(NESTED_BORDER, "space-y-2")}>
        {childMessages.map((child) => (
          <div key={child.id}>{renderChild(child)}</div>
        ))}
      </div>
    );
  }
  if (isActive) {
    return (
      <div className={NESTED_BORDER}>
        <span className="text-xs text-muted-foreground italic">Subagent working...</span>
      </div>
    );
  }
  if (resultText) {
    return (
      <div className={NESTED_BORDER}>
        <p
          data-testid="subagent-result-text"
          className="text-xs text-foreground/80 whitespace-pre-wrap break-words"
        >
          {resultText}
        </p>
      </div>
    );
  }
  if (prompt) {
    return (
      <div className={NESTED_BORDER}>
        <p className="text-xs text-muted-foreground whitespace-pre-wrap break-words">{prompt}</p>
      </div>
    );
  }
  return null;
}

export const ToolSubagentMessage = memo(function ToolSubagentMessage({
  comment,
  childMessages,
  isContainingTurnActive = false,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  worktreePath,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  onOpenFile,
  renderChild,
}: ToolSubagentMessageProps) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const childCount = childMessages.length;
  const { subagentTask, isActive, resultText, prompt, hasExpandableContent } =
    deriveSubagentDisplay(metadata, childCount, isContainingTurnActive);
  const description = subagentTask?.description || comment.content || "Subagent";
  const subagentType = subagentTask?.subagent_type || "Task";

  // Track manual override state - null means "use auto behavior"
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);

  // Auto behavior: expand if active or if we have a result text to surface
  // (silent Auggie-style subagents have no child messages, so we want the
  // final summary visible without an extra click).
  const autoExpanded = isActive || Boolean(resultText);

  // Reset the manual override the first time a result_text arrives so a card
  // the user collapsed while the subagent was running auto-opens to show the
  // summary. "Adjust state during render" pattern (preferred over useEffect):
  // https://react.dev/learn/you-might-not-need-an-effect — only fires on the
  // empty -> non-empty transition; later user collapses persist.
  const [prevResultText, setPrevResultText] = useState(resultText);
  if (resultText !== prevResultText) {
    setPrevResultText(resultText);
    if (resultText && !prevResultText) {
      setManualExpandState(null);
    }
  }

  // Derive expanded state: manual override takes precedence, otherwise use auto
  const isExpanded = hasExpandableContent && (manualExpandState ?? autoExpanded);

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  return (
    <div className="w-full" data-testid="subagent-card">
      <SubagentHeader
        isExpanded={isExpanded}
        subagentType={subagentType}
        description={description}
        isActive={isActive}
        childCount={childCount}
        hasExpandableContent={hasExpandableContent}
        onToggle={handleToggle}
      />
      {!isActive && <SubagentMetaRow subagentTask={subagentTask} />}
      <SubagentContent
        isExpanded={isExpanded}
        childMessages={childMessages}
        isActive={isActive}
        prompt={prompt}
        resultText={resultText}
        renderChild={renderChild}
      />
    </div>
  );
}, arePropsEqual);
