"use client";

import { GridSpinner } from "@/components/grid-spinner";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { TurnGroupMessage } from "@/components/task/chat/messages/turn-group-message";

export type MessageListProps = {
  items: RenderItem[];
  messages: Message[];
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
  return item.type === "turn_group" ? item.id : item.message.id;
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

export function MessageListStatus({
  isLoadingMore,
  hasMore,
  showLoadingState,
  messagesLoading,
  isInitialLoading,
  messagesCount,
}: {
  isLoadingMore: boolean;
  hasMore: boolean;
  showLoadingState: boolean;
  messagesLoading: boolean;
  isInitialLoading: boolean;
  messagesCount: number;
}) {
  return (
    <>
      {isLoadingMore && hasMore && (
        <div className="text-center text-xs text-muted-foreground py-2">
          Loading older messages...
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

export function MessageItem({
  item,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup,
  isTurnActive,
  messages,
  sessionState,
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
  messages: Message[];
  sessionState?: TaskSessionState;
  onScrollToMessage: (id: string) => void;
}) {
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
        allMessages={messages}
        onScrollToMessage={onScrollToMessage}
      />
    );
  }
  return (
    <MessageRenderer
      comment={item.message}
      isTaskDescription={item.message.id === "task-description"}
      sessionState={sessionState}
      taskId={taskId}
      permissionsByToolCallId={permissionsByToolCallId}
      childrenByParentToolCallId={childrenByParentToolCallId}
      worktreePath={worktreePath}
      sessionId={sessionId ?? undefined}
      onOpenFile={onOpenFile}
      allMessages={messages}
      onScrollToMessage={onScrollToMessage}
    />
  );
}
