"use client";

import { memo, useCallback } from "react";
import { Button } from "@kandev/ui/button";
import { IconAlertCircle, IconX } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { TurnGroupMessage } from "@/components/task/chat/messages/turn-group-message";
import { PrepareProgress } from "@/components/session/prepare-progress";
import { useAppStore } from "@/components/state-provider";
import { lastAgentErrorStamp, readLastAgentError } from "@/lib/session-last-agent-error";

export type MessageListProps = {
  items: RenderItem[];
  messages: Message[];
  /** Action messages rendered after the env prep error status in the footer. */
  footerActionMessages?: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  messagesLoading: boolean;
  isWorking: boolean;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

export function getItemKey(item: RenderItem): string {
  if (item.type === "turn_group" || item.type === "prepare_progress") return item.id;
  return item.message.id;
}

export function getSessionRunningState(sessionState: string | null | undefined) {
  return sessionState === "CREATED" || sessionState === "STARTING" || sessionState === "RUNNING";
}

export function getLastTurnGroupId(items: RenderItem[]) {
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.type === "turn_group") return item.id;
  }
  return null;
}

export function LastAgentErrorNotice({ sessionId }: { sessionId: string | null }) {
  const metadata = useAppStore((state) =>
    sessionId ? state.taskSessions.items[sessionId]?.metadata : null,
  );
  const error = readLastAgentError(metadata);
  const stamp = error ? lastAgentErrorStamp(error) : "";
  const dismissedStamp = useAppStore((state) =>
    sessionId ? state.dismissedAgentErrors[sessionId] : undefined,
  );
  const dismissAgentError = useAppStore((state) => state.dismissAgentError);

  const dismiss = useCallback(() => {
    if (!sessionId || !stamp) return;
    dismissAgentError(sessionId, stamp);
  }, [dismissAgentError, sessionId, stamp]);

  if (!error || dismissedStamp === stamp) return null;

  return (
    <div
      data-testid="last-agent-error-notice"
      className="mb-3 rounded-md border border-destructive/25 bg-destructive/10 text-destructive"
      role="alert"
    >
      <div className="flex items-start gap-2 px-3 py-2">
        <IconAlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <div className="text-xs font-medium">Previous agent error</div>
          <pre className="mt-1 max-h-40 overflow-y-auto whitespace-pre-wrap break-words text-[11px] leading-relaxed text-destructive/85">
            {error.message}
          </pre>
        </div>
        <button
          type="button"
          className="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded hover:bg-destructive/10 cursor-pointer"
          aria-label="Hide previous agent error"
          onClick={dismiss}
        >
          <IconX className="h-3.5 w-3.5" aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

export function MessageListStatus({
  isLoadingMore,
  hasMore,
  showLoadingState,
  messagesLoading,
  isInitialLoading,
  messagesCount,
  onLoadMore,
}: {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messagesCount: number;
  /**
   * Explicitly load the previous page of older messages. Rendered as a button so
   * older history is always reachable even when the scroll-up IntersectionObserver
   * fails to re-arm (e.g. pinned at the very top with the sentinel always in view).
   */
  onLoadMore?: () => void;
}) {
  return (
    <>
      {isLoadingMore && hasMore && (
        <div className="text-center text-xs text-muted-foreground py-2">
          Loading older messages...
        </div>
      )}
      {hasMore && !isLoadingMore && onLoadMore && (
        <div className="flex justify-center py-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="cursor-pointer text-xs text-muted-foreground"
            data-testid="load-older-messages"
            onClick={onLoadMore}
          >
            Load older messages
          </Button>
        </div>
      )}
      {showLoadingState && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <GridSpinner className="text-primary mr-2" />
          <span>Loading messages...</span>
        </div>
      )}
      {!messagesLoading && !isInitialLoading && messagesCount === 0 && (
        <div className="flex items-center justify-center py-8 text-muted-foreground">
          <span>No messages yet. Start the conversation!</span>
        </div>
      )}
    </>
  );
}

export const MessageItem = memo(function MessageItem({
  item,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup,
  isTurnActive,
  onScrollToMessage,
}: {
  item: RenderItem;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  isLastGroup: boolean;
  isTurnActive: boolean;
  onScrollToMessage: (id: string) => void;
}) {
  if (item.type === "prepare_progress") {
    return <PrepareProgress sessionId={item.sessionId} />;
  }
  if (item.type === "turn_group") {
    return (
      <TurnGroupMessage
        group={item}
        sessionId={sessionId}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        isLastGroup={isLastGroup}
        isTurnActive={isTurnActive}
        onScrollToMessage={onScrollToMessage}
      />
    );
  }
  return (
    <MessageRenderer
      comment={item.message}
      isTaskDescription={item.message.id === "task-description"}
      taskId={taskId}
      permissionsByToolCallId={permissionsByToolCallId}
      childrenByParentToolCallId={childrenByParentToolCallId}
      worktreePath={worktreePath}
      sessionId={sessionId ?? undefined}
      onOpenFile={onOpenFile}
      onScrollToMessage={onScrollToMessage}
    />
  );
});
