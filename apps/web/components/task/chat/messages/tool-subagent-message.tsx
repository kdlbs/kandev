"use client";

import { useState, useCallback, memo } from "react";
import { IconChevronDown, IconChevronRight } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { ToolCallMetadata } from "@/components/task/chat/types";

type ToolSubagentMessageProps = {
  comment: Message;
  childMessages: Message[];
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
    prevProps.worktreePath === nextProps.worktreePath &&
    prevProps.onOpenFile === nextProps.onOpenFile
  );
}

type SubagentHeaderProps = {
  isExpanded: boolean;
  subagentType: string;
  description: string;
  isActive: boolean;
  childCount: number;
  onToggle: () => void;
};

function SubagentHeader({
  isExpanded,
  subagentType,
  description,
  isActive,
  childCount,
  onToggle,
}: SubagentHeaderProps) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        "flex items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded",
        "hover:bg-muted/30 transition-colors cursor-pointer",
      )}
    >
      {isExpanded ? (
        <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
      ) : (
        <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
      )}
      <span className="bg-muted text-muted-foreground text-[10px] px-1.5 rounded font-medium uppercase tracking-wide">
        {subagentType}
      </span>
      <span className="font-mono text-xs truncate text-muted-foreground inline-flex items-center gap-1.5">
        {description}
        {isActive && <GridSpinner className="text-muted-foreground shrink-0" />}
      </span>
      {childCount > 0 && (
        <span className="text-muted-foreground/60 text-xs px-1.5 rounded min-w-[20px] text-center font-mono">
          {childCount} tool call{childCount !== 1 ? "s" : ""}
        </span>
      )}
    </button>
  );
}

type SubagentContentProps = {
  isExpanded: boolean;
  childMessages: Message[];
  isActive: boolean;
  renderChild: (message: Message) => React.ReactNode;
};

function SubagentContent({
  isExpanded,
  childMessages,
  isActive,
  renderChild,
}: SubagentContentProps) {
  if (!isExpanded) return null;
  if (childMessages.length > 0) {
    return (
      <div className="ml-2 pl-4 border-l-2 border-border/30 mt-1 space-y-2">
        {childMessages.map((child) => (
          <div key={child.id}>{renderChild(child)}</div>
        ))}
      </div>
    );
  }
  if (isActive) {
    return (
      <div className="ml-2 pl-4 border-l-2 border-border/30 mt-1">
        <span className="text-xs text-muted-foreground italic">Subagent working...</span>
      </div>
    );
  }
  return null;
}

export const ToolSubagentMessage = memo(function ToolSubagentMessage({
  comment,
  childMessages,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  worktreePath,
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  onOpenFile,
  renderChild,
}: ToolSubagentMessageProps) {
  const metadata = comment.metadata as ToolCallMetadata | undefined;
  const status = metadata?.status;
  const normalized = metadata?.normalized;
  const subagentTask = normalized?.subagent_task;

  const description = subagentTask?.description || comment.content || "Subagent";
  const subagentType = subagentTask?.subagent_type || "Task";
  const childCount = childMessages.length;

  const isActive = status === "running";

  // Track manual override state - null means "use auto behavior"
  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);

  // Auto behavior: expand if active, collapse otherwise
  const autoExpanded = isActive;

  // Derive expanded state: manual override takes precedence, otherwise use auto
  const isExpanded = manualExpandState ?? autoExpanded;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  return (
    <div className="w-full">
      <SubagentHeader
        isExpanded={isExpanded}
        subagentType={subagentType}
        description={description}
        isActive={isActive}
        childCount={childCount}
        onToggle={handleToggle}
      />
      <SubagentContent
        isExpanded={isExpanded}
        childMessages={childMessages}
        isActive={isActive}
        renderChild={renderChild}
      />
    </div>
  );
}, arePropsEqual);
