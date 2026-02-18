'use client';

import { useState, useCallback, memo } from 'react';
import { IconChevronDown, IconChevronRight } from '@tabler/icons-react';
import { GridSpinner } from '@/components/grid-spinner';
import { cn, transformPathsInText } from '@/lib/utils';
import type { Message } from '@/lib/types/http';
import type { TurnGroup } from '@/hooks/use-processed-messages';
import type { ToolCallMetadata } from '@/components/task/chat/types';
import { MessageRenderer } from '@/components/task/chat/message-renderer';

type TurnGroupMessageProps = {
  group: TurnGroup;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  /** Whether this is the last turn group in the current turn */
  isLastGroup?: boolean;
  /** Whether the turn is still active (agent is running) */
  isTurnActive?: boolean;
  allMessages?: Message[];
  onScrollToMessage?: (messageId: string) => void;
};

function countMessageTypes(messages: Message[]): { toolCalls: number; subagents: number } {
  let toolCalls = 0;
  let subagents = 0;
  for (const msg of messages) {
    const metadata = msg.metadata as ToolCallMetadata | undefined;
    if (metadata?.normalized?.kind === 'subagent_task') {
      subagents++;
    } else {
      toolCalls++;
    }
  }
  return { toolCalls, subagents };
}

function getActiveGroupDescription(messages: Message[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    const msg = messages[i];
    const metadata = msg.metadata as ToolCallMetadata | undefined;
    if (metadata?.title) return metadata.title;
    if (msg.content && msg.content.length > 0) {
      return msg.content.slice(0, 60) + (msg.content.length > 60 ? '...' : '');
    }
  }
  return 'Working...';
}

function getCompletedGroupDescription(messages: Message[]): string {
  const { toolCalls, subagents } = countMessageTypes(messages);
  if (subagents === 0) return `tool call${toolCalls !== 1 ? 's' : ''}`;
  if (toolCalls === 0) return `subagent${subagents !== 1 ? 's' : ''}`;
  return `tool call${toolCalls !== 1 ? 's' : ''}, ${subagents} subagent${subagents !== 1 ? 's' : ''}`;
}

function getGroupDescription(messages: Message[], isActive: boolean): string {
  if (isActive) return getActiveGroupDescription(messages);
  return getCompletedGroupDescription(messages);
}

function hasPendingPermission(
  messages: Message[],
  permissionsByToolCallId: Map<string, Message>
): boolean {
  for (const msg of messages) {
    if (msg.type !== 'tool_call') continue;
    const toolCallId = (msg.metadata as { tool_call_id?: string } | undefined)?.tool_call_id;
    if (!toolCallId) continue;
    const permissionMsg = permissionsByToolCallId.get(toolCallId);
    if (!permissionMsg) continue;
    const permStatus = (permissionMsg.metadata as { status?: string } | undefined)?.status;
    if (permStatus !== 'approved' && permStatus !== 'rejected') return true;
  }
  return false;
}

// Tool message types that have status tracking
const TOOL_MESSAGE_TYPES = new Set(['tool_call', 'tool_edit', 'tool_read', 'tool_execute', 'tool_search']);

/**
 * Check if any tool or subagent in the group is still running.
 * A tool/subagent is considered running if it's not in a terminal state (complete or error).
 */
function hasRunningTool(messages: Message[]): boolean {
  for (const msg of messages) {
    const metadata = msg.metadata as ToolCallMetadata | undefined;
    const isToolMessage = msg.type && TOOL_MESSAGE_TYPES.has(msg.type);
    const isSubagent = metadata?.normalized?.kind === 'subagent_task';
    if (!isToolMessage && !isSubagent) continue;
    const status = metadata?.status;
    if (status !== 'complete' && status !== 'error') return true;
  }
  return false;
}

type TurnGroupHeaderProps = {
  isExpanded: boolean;
  count: number;
  description: string;
  isGroupRunning: boolean;
  onToggle: () => void;
};

function TurnGroupHeader({ isExpanded, count, description, isGroupRunning, onToggle }: TurnGroupHeaderProps) {
  return (
    <button
      type="button"
      onClick={onToggle}
      className={cn(
        'flex items-center gap-2 w-full text-left px-2 py-1.5 -mx-2 rounded',
        'hover:bg-muted/30 transition-colors cursor-pointer'
      )}
    >
      {isExpanded
        ? <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
        : <IconChevronRight className="h-3.5 w-3.5 text-muted-foreground/60 flex-shrink-0" />
      }
      <span className="bg-muted text-muted-foreground text-xs px-1.5 rounded min-w-[20px] text-center font-mono">
        {count}
      </span>
      <span className="font-mono text-xs truncate text-muted-foreground inline-flex items-center gap-1.5">
        {description}
        {isGroupRunning && <GridSpinner className="text-muted-foreground shrink-0" />}
      </span>
    </button>
  );
}

type TurnGroupContentProps = {
  group: TurnGroup;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId?: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  allMessages?: Message[];
  onScrollToMessage?: (messageId: string) => void;
};

function TurnGroupContent({ group, sessionId, permissionsByToolCallId, childrenByParentToolCallId, taskId, worktreePath, onOpenFile, allMessages, onScrollToMessage }: TurnGroupContentProps) {
  return (
    <div className="ml-2 pl-4 border-l-2 border-border/30 mt-1 space-y-2">
      {group.messages.map((msg) => (
        <MessageRenderer
          key={msg.id}
          comment={msg}
          isTaskDescription={false}
          taskId={taskId}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          worktreePath={worktreePath}
          sessionId={sessionId ?? undefined}
          onOpenFile={onOpenFile}
          allMessages={allMessages}
          onScrollToMessage={onScrollToMessage}
        />
      ))}
    </div>
  );
}

export const TurnGroupMessage = memo(function TurnGroupMessage({
  group,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup = false,
  isTurnActive = false,
  allMessages,
  onScrollToMessage,
}: TurnGroupMessageProps) {
  const isGroupRunning = hasRunningTool(group.messages);
  const hasPending = hasPendingPermission(group.messages, permissionsByToolCallId);

  const [manualExpandState, setManualExpandState] = useState<boolean | null>(null);

  // Auto behavior: expand if running, has pending, or is the last group while turn is active
  const autoExpanded = isGroupRunning || hasPending || (isTurnActive && isLastGroup);
  const isExpanded = manualExpandState ?? autoExpanded;

  const handleToggle = useCallback(() => {
    setManualExpandState((prev) => !(prev ?? autoExpanded));
  }, [autoExpanded]);

  const rawDescription = getGroupDescription(group.messages, isGroupRunning);
  const description = transformPathsInText(rawDescription, worktreePath);
  const count = group.messages.length;

  return (
    <div className="w-full">
      <TurnGroupHeader
        isExpanded={isExpanded}
        count={count}
        description={description}
        isGroupRunning={isGroupRunning}
        onToggle={handleToggle}
      />
      {isExpanded && (
        <TurnGroupContent
          group={group}
          sessionId={sessionId}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          allMessages={allMessages}
          onScrollToMessage={onScrollToMessage}
        />
      )}
    </div>
  );
});
