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
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { dismissLastAgentError } from "@/lib/api/domains/session-api";
import {
  type LastAgentError,
  lastAgentErrorStamp,
  readLastAgentError,
} from "@/lib/session-last-agent-error";

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
  if (
    item.type === "turn_group" ||
    item.type === "prepare_progress" ||
    item.type === "agent_error_notice"
  )
    return item.id;
  return item.message.id;
}

export function getMessageDomId(messageId: string) {
  return `msg-${messageId}`;
}

export type UserMessageRenderStop = {
  messageId: string;
  itemIndex: number;
};

export function getUserMessageRenderStops(items: RenderItem[]): UserMessageRenderStop[] {
  const stops: UserMessageRenderStop[] = [];
  items.forEach((item, itemIndex) => {
    if (item.type === "message") {
      if (item.message.author_type === "user") {
        stops.push({ messageId: item.message.id, itemIndex });
      }
    }
  });
  return stops;
}

export function findUserMessageElement(
  scrollElement: HTMLElement,
  messageId: string,
): HTMLElement | null {
  for (const element of scrollElement.querySelectorAll<HTMLElement>("[data-user-message-id]")) {
    if (element.dataset.userMessageId === messageId) return element;
  }
  return null;
}

export function replayMessageHighlight(element: HTMLElement) {
  element.classList.remove("search-flash");
  void element.offsetWidth;
  element.classList.add("search-flash");
  window.setTimeout(() => element.classList.remove("search-flash"), 1400);
}

export function getNavigationScrollBehavior(): "auto" | "smooth" {
  return window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth";
}

function isAtNavigationPosition(scrollElement: HTMLElement, element: HTMLElement) {
  if (scrollElement.clientHeight <= 0) return true;
  const viewport = scrollElement.getBoundingClientRect();
  const target = element.getBoundingClientRect();
  const viewportCenter = viewport.top + viewport.height / 2;
  const targetCenter = target.top + target.height / 2;
  const isCentered = Math.abs(viewportCenter - targetCenter) <= 40;
  const maxScrollTop = Math.max(0, scrollElement.scrollHeight - scrollElement.clientHeight);
  const isAtTopBoundary = scrollElement.scrollTop <= 1 && targetCenter <= viewportCenter;
  const isAtBottomBoundary =
    maxScrollTop - scrollElement.scrollTop <= 1 && targetCenter >= viewportCenter;
  return isCentered || isAtTopBoundary || isAtBottomBoundary;
}

export async function waitForUserMessageElement(
  scrollElement: HTMLElement,
  messageId: string,
  shouldContinue: () => boolean,
): Promise<HTMLElement | null> {
  const maxFrames = 30;
  const requiredStableFrames = 8;
  let candidate: HTMLElement | null = null;
  let stableFrames = 0;
  for (let frame = 0; frame < maxFrames && shouldContinue(); frame++) {
    const mounted = findUserMessageElement(scrollElement, messageId);
    const element = mounted && isAtNavigationPosition(scrollElement, mounted) ? mounted : null;
    if (element === candidate && element) {
      stableFrames++;
    } else {
      candidate = element;
      stableFrames = element ? 1 : 0;
    }
    if (candidate && stableFrames >= requiredStableFrames) return candidate;
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
  }
  return null;
}

export function getSessionRunningState(sessionState: string | null | undefined) {
  return sessionState === "CREATED" || sessionState === "STARTING" || sessionState === "RUNNING";
}

/** Only the latest ordinary agent row in the active turn can be the streaming reply. */
export function getStreamingAgentMessageId(messages: Message[]): string | null {
  let latestUserIndex = -1;
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].author_type === "user") {
      latestUserIndex = i;
      break;
    }
  }
  for (let i = messages.length - 1; i > latestUserIndex; i--) {
    const message = messages[i];
    if (
      message.author_type === "agent" &&
      (message.type === "message" || message.type === "content" || !message.type)
    ) {
      return message.id;
    }
  }
  return null;
}

export function getLastTurnGroupId(items: RenderItem[]) {
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.type === "turn_group") return item.id;
  }
  return null;
}

export function getConversationLoadingState(params: {
  messagesLoading: boolean;
  messagesCount: number;
  isWorking: boolean;
  sessionState?: TaskSessionState | null;
}) {
  const isInitialLoading = params.messagesLoading && params.messagesCount === 0;
  const isNonLoadableSession =
    !params.sessionState ||
    params.sessionState === "CREATED" ||
    params.sessionState === "FAILED" ||
    params.sessionState === "COMPLETED" ||
    params.sessionState === "CANCELLED";
  // CREATED sessions are prepare-only: the agent hasn't been started yet, so
  // there is no conversation to load and the "Start agent" button is the
  // primary CTA. Suppress the spinner unconditionally to avoid a misleading
  // overlay racing with that button (synthetic task-description counts as a
  // message and would otherwise trip the messagesCount > 0 branch).
  if (params.sessionState === "CREATED") {
    return { isInitialLoading, showLoadingState: false };
  }
  return {
    isInitialLoading,
    showLoadingState:
      params.messagesLoading &&
      !params.isWorking &&
      (params.messagesCount > 0 || !isNonLoadableSession),
  };
}

// The chat banner stays visible until the user explicitly dismisses it, even
// after the agent resumes — so the user can read the full error message at
// their own pace. The sidebar icon, by contrast, also auto-hides once the
// agent posts a new message (see agentErrorMessageForTask).
export function LastAgentErrorNotice({
  sessionId,
  error,
}: {
  sessionId: string | null;
  error: LastAgentError | null;
}) {
  const stamp = error ? lastAgentErrorStamp(error) : "";
  const dismissedStamp = useAppStore((state) =>
    sessionId ? state.dismissedAgentErrors[sessionId] : undefined,
  );
  const dismissAgentError = useAppStore((state) => state.dismissAgentError);
  const setTaskSession = useAppStore((state) => state.setTaskSession);
  const store = useAppStoreApi();

  const dismiss = useCallback(() => {
    if (!sessionId || !stamp) return;
    void dismissLastAgentError(sessionId, stamp)
      .then((resp) => {
        const current = readLastAgentError(
          store.getState().taskSessions.items[sessionId]?.metadata,
        );
        if (current && lastAgentErrorStamp(current) !== stamp) return;
        dismissAgentError(sessionId, stamp);
        setTaskSession(resp.session);
      })
      .catch((err: unknown) => {
        console.error("Failed to dismiss last agent error", err);
      });
  }, [dismissAgentError, sessionId, stamp, setTaskSession, store]);

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
        <div
          className="flex items-center justify-center py-8 text-muted-foreground"
          data-testid="conversation-loading-state"
        >
          <GridSpinner className="text-primary mr-2" />
          <span>Loading conversation...</span>
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
  streamingMessageId,
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
  streamingMessageId?: string | null;
}) {
  if (item.type === "prepare_progress") {
    return <PrepareProgress sessionId={item.sessionId} />;
  }
  if (item.type === "agent_error_notice") {
    return <LastAgentErrorNotice sessionId={item.sessionId} error={item.error} />;
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
        isTurnActive={
          isTurnActive &&
          (streamingMessageId
            ? item.messages.some((message) => message.id === streamingMessageId)
            : false)
        }
        streamingMessageId={streamingMessageId}
      />
    );
  }
  return (
    <div
      id={getMessageDomId(item.message.id)}
      data-user-message-id={item.message.author_type === "user" ? item.message.id : undefined}
    >
      <MessageRenderer
        comment={item.message}
        isTaskDescription={item.message.id === "task-description"}
        taskId={taskId}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        worktreePath={worktreePath}
        sessionId={sessionId ?? undefined}
        isTurnActive={isTurnActive && item.message.id === streamingMessageId}
        onOpenFile={onOpenFile}
      />
    </div>
  );
});
