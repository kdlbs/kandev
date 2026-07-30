"use client";

import { useEffect, useMemo, useRef, memo, forwardRef, useImperativeHandle } from "react";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useSessionTurn } from "@/hooks/domains/session/use-session-turn";
import { MessageListFooter } from "./message-list-footer";
import { useNativeScrollManagement } from "./message-list-native-scroll";
import { useTranscriptAutoScrollEnabled } from "./use-transcript-auto-scroll-enabled";
import {
  type MessageListProps,
  type MessageListHandle,
  type LastPromptEdge,
  MessageListStatus,
  MessageItem,
  getItemKey,
  getConversationLoadingState,
  getEffectiveActiveTurnId,
  getLastTurnGroupId,
  getStreamingAgentMessageId,
  resolveLastPromptEdge,
  isElementFullyVisible,
} from "./message-list-shared";

/** Notifies `onLastPromptEdgeChange`/`onFirstMessageHiddenChange` whenever the
 * last-prompt or first message crosses the container's viewport edges, so
 * the composer's scroll buttons and the anchored-bar affordance know when to
 * show themselves. A single scroll/resize listener drives both checks. */
function useTranscriptEdgeTracking(
  scrollRef: React.RefObject<HTMLDivElement | null>,
  lastPromptMessageId: string | null | undefined,
  onLastPromptEdgeChange: ((edge: LastPromptEdge) => void) | undefined,
  firstMessageId: string | null | undefined,
  onFirstMessageHiddenChange: ((isHidden: boolean) => void) | undefined,
) {
  useEffect(() => {
    const container = scrollRef.current;
    const lastTarget = lastPromptMessageId
      ? document.getElementById(`msg-${lastPromptMessageId}`)
      : null;
    const firstTarget = firstMessageId ? document.getElementById(`msg-${firstMessageId}`) : null;
    if (!container || !lastTarget) onLastPromptEdgeChange?.("visible");
    if (!container || !firstTarget) onFirstMessageHiddenChange?.(false);
    if (!container) return;

    const update = () => {
      if (lastTarget) onLastPromptEdgeChange?.(resolveLastPromptEdge(container, lastTarget));
      if (firstTarget) onFirstMessageHiddenChange?.(!isElementFullyVisible(container, firstTarget));
    };
    update();
    container.addEventListener("scroll", update, { passive: true });
    const resizeObserver = new ResizeObserver(update);
    resizeObserver.observe(container);
    if (lastTarget) resizeObserver.observe(lastTarget);
    if (firstTarget && firstTarget !== lastTarget) resizeObserver.observe(firstTarget);
    return () => {
      container.removeEventListener("scroll", update);
      resizeObserver.disconnect();
    };
  }, [
    scrollRef,
    lastPromptMessageId,
    onLastPromptEdgeChange,
    firstMessageId,
    onFirstMessageHiddenChange,
  ]);
}

type MessageRowProps = {
  item: RenderItem;
  sessionId: string | null;
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  isLastGroup: boolean;
  activeTurnId: string | null;
  streamingMessageId: string | null;
  onScrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
};

/** One transcript row, keyed and DOM-id'd by `getItemKey` so the scroll
 * affordances (and `scrollToMessage`) can locate it directly. */
function MessageRow({
  item,
  sessionId,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  worktreePath,
  onOpenFile,
  isLastGroup,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
}: MessageRowProps) {
  const key = getItemKey(item);
  return (
    <div id={`msg-${key}`} className="pb-2" style={{ overflowAnchor: "none" }}>
      <MessageItem
        item={item}
        sessionId={sessionId}
        permissionsByToolCallId={permissionsByToolCallId}
        childrenByParentToolCallId={childrenByParentToolCallId}
        taskId={taskId}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile}
        isLastGroup={isLastGroup}
        activeTurnId={activeTurnId}
        streamingMessageId={streamingMessageId}
        onScrollToMessage={onScrollToMessage}
      />
    </div>
  );
}

type NativeMessageListBodyProps = {
  items: RenderItem[];
  messages: Message[];
  footerActionMessages?: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  isWorking: boolean;
  messagesLoading: boolean;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  hasMore: boolean;
  isLoadingMore: boolean;
  isInitialLoading: boolean;
  showLoadingState: boolean;
  loadMore: () => Promise<number>;
  sentinelRef: (node: HTMLDivElement | null) => void;
  lastTurnGroupId: string | null;
  activeTurnId: string | null;
  streamingMessageId: string | null;
  onScrollToMessage: (messageId: string, options?: { align?: "start" | "center" }) => void;
  autoScrollEnabled: boolean;
};

/** Sentinel, status/footer, and transcript rows — everything below the
 * (optional) anchored prompt bar inside the scroll container. */
function NativeMessageListBody({
  items,
  messages,
  footerActionMessages,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  sessionId,
  isWorking,
  messagesLoading,
  sessionState,
  worktreePath,
  onOpenFile,
  hasMore,
  isLoadingMore,
  isInitialLoading,
  showLoadingState,
  loadMore,
  sentinelRef,
  lastTurnGroupId,
  activeTurnId,
  streamingMessageId,
  onScrollToMessage,
  autoScrollEnabled,
}: NativeMessageListBodyProps) {
  return (
    <div className="p-4">
      {/* Sentinel for lazy loading older messages */}
      {hasMore && <div ref={sentinelRef} className="h-px" />}

      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
        onLoadMore={loadMore}
      />

      {items.map((item) => (
        <MessageRow
          key={getItemKey(item)}
          item={item}
          sessionId={sessionId}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          isLastGroup={item.type === "turn_group" && item.id === lastTurnGroupId}
          activeTurnId={activeTurnId}
          streamingMessageId={streamingMessageId}
          onScrollToMessage={onScrollToMessage}
        />
      ))}

      <MessageListFooter
        sessionState={sessionState}
        sessionId={sessionId}
        messages={messages}
        isWorking={isWorking}
        footerActionMessages={footerActionMessages}
      />

      {/* Bottom anchor keeps the view pinned while auto-scroll is enabled.
          The scroll container disables anchoring entirely while it is off,
          so status/footer updates cannot choose a different anchor and move
          the frozen transcript. */}
      <div style={{ overflowAnchor: autoScrollEnabled ? "auto" : "none", height: 1 }} />
    </div>
  );
}

/**
 * Renders the transcript as plain DOM nodes with `overflow-anchor` for
 * scroll pinning. Wires together lazy-loading of older messages, the
 * scroll-position-on-prepend fix-up, last-prompt/first-message edge
 * tracking, and the session's auto-scroll toggle (freeze/resume/catch-up)
 * via {@link useNativeScrollManagement}.
 */
export const NativeMessageList = memo(
  forwardRef<MessageListHandle, MessageListProps>(function NativeMessageList(
    {
      items,
      messages,
      footerActionMessages,
      permissionsByToolCallId,
      childrenByParentToolCallId,
      taskId,
      sessionId,
      messagesLoading,
      isWorking,
      sessionState,
      worktreePath,
      onOpenFile,
      lastPromptMessageId,
      onLastPromptEdgeChange,
      firstMessageId,
      onFirstMessageHiddenChange,
      stickyPromptBar,
    }: MessageListProps,
    ref,
  ) {
    const scrollRef = useRef<HTMLDivElement>(null);

    const { isInitialLoading, showLoadingState } = getConversationLoadingState({
      messagesLoading,
      messagesCount: messages.length,
      isWorking,
      sessionState,
    });
    const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
    const { activeTurnId } = useSessionTurn(sessionId);
    const effectiveActiveTurnId = getEffectiveActiveTurnId(activeTurnId, isWorking);
    const streamingMessageId = getStreamingAgentMessageId(messages);
    const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);
    const autoScrollEnabled = useTranscriptAutoScrollEnabled(sessionId);
    const { handleScrollToMessage, sentinelRef } = useNativeScrollManagement({
      scrollRef,
      items,
      messages,
      isWorking,
      sessionId,
      enabled: autoScrollEnabled,
      hasMore,
      isLoadingMore,
      loadMore,
    });
    useImperativeHandle(ref, () => ({ scrollToMessage: handleScrollToMessage }), [
      handleScrollToMessage,
    ]);
    useTranscriptEdgeTracking(
      scrollRef,
      lastPromptMessageId,
      onLastPromptEdgeChange,
      firstMessageId,
      onFirstMessageHiddenChange,
    );

    return (
      <SessionPanelContent
        ref={scrollRef}
        className={`relative chat-message-list p-0 ${
          autoScrollEnabled ? "[overflow-anchor:auto]" : "[overflow-anchor:none]"
        }`}
      >
        {stickyPromptBar}
        <NativeMessageListBody
          items={items}
          messages={messages}
          footerActionMessages={footerActionMessages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId}
          sessionId={sessionId}
          isWorking={isWorking}
          messagesLoading={messagesLoading}
          sessionState={sessionState}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          hasMore={hasMore}
          isLoadingMore={isLoadingMore}
          isInitialLoading={isInitialLoading}
          showLoadingState={showLoadingState}
          loadMore={loadMore}
          sentinelRef={sentinelRef}
          lastTurnGroupId={lastTurnGroupId}
          activeTurnId={effectiveActiveTurnId}
          streamingMessageId={streamingMessageId}
          onScrollToMessage={handleScrollToMessage}
          autoScrollEnabled={autoScrollEnabled}
        />
      </SessionPanelContent>
    );
  }),
);
