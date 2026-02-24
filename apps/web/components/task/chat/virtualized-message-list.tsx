"use client";

import type React from "react";
import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import { GridSpinner } from "@/components/grid-spinner";
import { SessionPanelContent } from "@kandev/ui/pannel-session";
import type { Message, TaskSessionState } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import { MessageRenderer } from "@/components/task/chat/message-renderer";
import { TurnGroupMessage } from "@/components/task/chat/messages/turn-group-message";
import { AgentStatus } from "@/components/task/chat/messages/agent-status";
import { PrepareProgress } from "@/components/session/prepare-progress";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";

const FIRST_INDEX_BASE = 100_000;

type VirtualizedMessageListProps = {
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

function getSessionRunningState(sessionState: string | null | undefined) {
  return sessionState === "CREATED" || sessionState === "STARTING" || sessionState === "RUNNING";
}

function getLastTurnGroupId(items: RenderItem[]) {
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.type === "turn_group") return item.id;
  }
  return null;
}

function MessageListStatus({
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

function VirtualItem({
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

type MessageListBodyProps = {
  items: RenderItem[];
  messages: Message[];
  permissionsByToolCallId: Map<string, Message>;
  childrenByParentToolCallId: Map<string, Message[]>;
  taskId?: string;
  sessionId: string | null;
  sessionState?: TaskSessionState;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
  scrollParent: HTMLDivElement;
  isRunning: boolean;
  lastTurnGroupId: string | null;
  hasMore: boolean;
  isLoadingMore: boolean;
  loadMore: () => Promise<number>;
  Header: () => React.ReactNode;
  Footer: () => React.ReactNode;
};

function getItemKey(item: RenderItem): string {
  return item.type === "turn_group" ? item.id : item.message.id;
}

function computeFirstItemIndex(prevKeys: string[], prevIndex: number, keys: string[]): number {
  if (prevKeys.length > 0 && keys.length > prevKeys.length) {
    const oldFirstKey = prevKeys[0];
    const newPos = keys.indexOf(oldFirstKey);
    console.log("[VirtualList] items grew", {
      prevCount: prevKeys.length,
      newCount: keys.length,
      delta: keys.length - prevKeys.length,
      oldFirstKey,
      newPos,
      prevIndex,
      newFirst3Keys: keys.slice(0, 3),
      oldFirst3Keys: prevKeys.slice(0, 3),
    });
    if (newPos > 0) {
      const result = prevIndex - newPos;
      console.log("[VirtualList] prepend detected → firstItemIndex", prevIndex, "→", result, `(shifted by ${newPos})`);
      return result;
    }
    if (newPos === -1) {
      // Old first key absorbed into a turn group — find first surviving key
      for (let i = 0; i < prevKeys.length; i++) {
        const idx = keys.indexOf(prevKeys[i]);
        if (idx >= 0) {
          const result = prevIndex - (idx - i);
          console.log("[VirtualList] key absorbed → first surviving key", prevKeys[i], "at old pos", i, "now at", idx, "→ firstItemIndex", result);
          return result;
        }
      }
      console.warn("[VirtualList] no surviving keys found! Keeping prevIndex", prevIndex);
    }
    return prevIndex;
  }
  if (prevKeys.length === 0 && keys.length > 0) {
    const result = FIRST_INDEX_BASE - keys.length + 1;
    console.log("[VirtualList] initial mount → firstItemIndex", result, "itemCount", keys.length);
    return result;
  }
  return prevIndex;
}

type IndexState = { keys: string[]; firstItemIndex: number };

/** Compute a stable firstItemIndex by tracking the key of the item that was
 *  previously at the top. When items change, we find where that key ended up
 *  and adjust firstItemIndex so Virtuoso keeps scroll position stable. */
function useStableFirstItemIndex(items: RenderItem[]) {
  const keys = useMemo(() => items.map(getItemKey), [items]);

  const [state, setState] = useState<IndexState>(() => ({
    keys,
    firstItemIndex: FIRST_INDEX_BASE - keys.length + 1,
  }));

  // When keys change, compute new firstItemIndex from previous state
  // Return the computed value immediately so Virtuoso sees correct firstItemIndex
  // on the SAME render that items change (not one render later).
  if (keys !== state.keys) {
    const nextIndex = computeFirstItemIndex(state.keys, state.firstItemIndex, keys);
    console.log("[VirtualList] useStableFirstItemIndex update", {
      prevKeyCount: state.keys.length,
      newKeyCount: keys.length,
      prevFirstItemIndex: state.firstItemIndex,
      newFirstItemIndex: nextIndex,
    });
    setState({ keys, firstItemIndex: nextIndex });
    return nextIndex;
  }

  return state.firstItemIndex;
}

function useVirtuosoCallbacks(props: MessageListBodyProps) {
  const { items, sessionId, permissionsByToolCallId, childrenByParentToolCallId, taskId } = props;
  const { worktreePath, onOpenFile, lastTurnGroupId, isRunning, messages, sessionState } = props;
  const { hasMore, isLoadingMore, loadMore } = props;
  const virtuosoRef = useRef<VirtuosoHandle>(null);
  const itemCount = items.length;
  const firstItemIndex = useStableFirstItemIndex(items);

  // Cooldown prevents startReached from rapid-firing after each prepend.
  // After items are prepended, the viewport is still near the start so Virtuoso
  // fires startReached again immediately — the cooldown breaks the loop.
  const loadCooldownRef = useRef(false);
  const handleStartReached = useCallback(() => {
    console.log("[VirtualList] startReached", { hasMore, isLoadingMore, cooldown: loadCooldownRef.current });
    if (hasMore && !isLoadingMore && !loadCooldownRef.current) {
      console.log("[VirtualList] → triggering loadMore");
      loadCooldownRef.current = true;
      loadMore().finally(() => {
        setTimeout(() => { loadCooldownRef.current = false; }, 500);
      });
    }
  }, [hasMore, isLoadingMore, loadMore]);

  const handleScrollToMessage = useCallback(
    (messageId: string) => {
      const idx = items.findIndex((item) => {
        if (item.type === "turn_group") return item.messages.some((m) => m.id === messageId);
        return item.message?.id === messageId;
      });
      if (idx >= 0) virtuosoRef.current?.scrollToIndex({ index: firstItemIndex + idx, align: "center" });
    },
    [items, firstItemIndex],
  );

  const computeItemKey = useCallback(
    (index: number) => {
      const item = items[index - firstItemIndex];
      if (!item) return index;
      return item.type === "turn_group" ? item.id : item.message.id;
    },
    [items, firstItemIndex],
  );

  const renderItem = useCallback(
    (index: number) => {
      const item = items[index - firstItemIndex];
      if (!item) {
        console.warn("[VirtualList] renderItem: no item at index", index, "firstItemIndex", firstItemIndex, "arrayIndex", index - firstItemIndex, "itemCount", items.length);
        return <div />;
      }

      return (
        <div className="pb-2">
          <VirtualItem
            item={item}
            sessionId={sessionId}
            permissionsByToolCallId={permissionsByToolCallId}
            childrenByParentToolCallId={childrenByParentToolCallId}
            taskId={taskId}
            worktreePath={worktreePath}
            onOpenFile={onOpenFile}
            isLastGroup={item.type === "turn_group" && item.id === lastTurnGroupId}
            isTurnActive={isRunning}
            messages={messages}
            sessionState={sessionState}
            onScrollToMessage={handleScrollToMessage}
          />
        </div>
      );
    },
    [items, firstItemIndex, sessionId, permissionsByToolCallId, childrenByParentToolCallId, taskId, worktreePath, onOpenFile, lastTurnGroupId, isRunning, messages, sessionState, handleScrollToMessage],
  );

  return { virtuosoRef, itemCount, firstItemIndex, handleStartReached, computeItemKey, renderItem };
}

const FOLLOW_SMOOTH = "smooth" as const;
const followOutput = (isAtBottom: boolean) => (isAtBottom ? FOLLOW_SMOOTH : false);

function MessageListBody(props: MessageListBodyProps) {
  const { scrollParent, Header, Footer } = props;
  const { virtuosoRef, itemCount, firstItemIndex, handleStartReached, computeItemKey, renderItem } =
    useVirtuosoCallbacks(props);

  console.log("[VirtualList] MessageListBody render", { itemCount, firstItemIndex, lastIndex: firstItemIndex + itemCount - 1 });

  return (
    <Virtuoso
      ref={virtuosoRef}
      logLevel={Number.MAX_SAFE_INTEGER}
      customScrollParent={scrollParent}
      totalCount={itemCount}
      firstItemIndex={firstItemIndex}
      initialTopMostItemIndex={itemCount - 1}
      computeItemKey={computeItemKey}
      itemContent={renderItem}
      followOutput={followOutput}
      startReached={handleStartReached}
      increaseViewportBy={200}
      atBottomThreshold={100}
      components={{ Header, Footer }}
    />
  );
}

/** Defer providing scroll parent to Virtuoso until the element has non-zero size.
 *  Dockview tabs can mount hidden (display:none), which causes zero-size errors. */
function useVisibleScrollParent() {
  const [scrollParent, setScrollParent] = useState<HTMLDivElement | null>(null);
  const nodeRef = useRef<HTMLDivElement | null>(null);
  const setScrollRef = useCallback((node: HTMLDivElement | null) => {
    nodeRef.current = node;
    if (node && node.offsetHeight > 0) setScrollParent(node);
  }, []);
  useEffect(() => {
    const node = nodeRef.current;
    if (!node || scrollParent) return;
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.contentRect.height > 0) {
          setScrollParent(node);
          ro.disconnect();
          return;
        }
      }
    });
    ro.observe(node);
    return () => ro.disconnect();
  }, [scrollParent]);
  return { scrollParent, setScrollRef };
}

export const VirtualizedMessageList = memo(function VirtualizedMessageList({
  items,
  messages,
  permissionsByToolCallId,
  childrenByParentToolCallId,
  taskId,
  sessionId,
  messagesLoading,
  isWorking,
  sessionState,
  worktreePath,
  onOpenFile,
}: VirtualizedMessageListProps) {
  const { scrollParent, setScrollRef } = useVisibleScrollParent();
  const isInitialLoading = messagesLoading && messages.length === 0;
  const isNonLoadableSession =
    !sessionState || ["CREATED", "FAILED", "COMPLETED", "CANCELLED"].includes(sessionState);
  const showLoadingState =
    (messagesLoading || isInitialLoading) && !isWorking && !isNonLoadableSession;
  const { loadMore, hasMore, isLoading: isLoadingMore } = useLazyLoadMessages(sessionId);
  const isRunning = getSessionRunningState(sessionState);
  const lastTurnGroupId = useMemo(() => getLastTurnGroupId(items), [items]);

  const Header = useCallback(
    () => (
      <MessageListStatus
        isLoadingMore={isLoadingMore}
        hasMore={hasMore}
        showLoadingState={showLoadingState}
        messagesLoading={messagesLoading}
        isInitialLoading={isInitialLoading}
        messagesCount={messages.length}
      />
    ),
    [isLoadingMore, hasMore, showLoadingState, messagesLoading, isInitialLoading, messages.length],
  );

  const Footer = useCallback(
    () => (
      <>
        {sessionId && <PrepareProgress sessionId={sessionId} />}
        <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
      </>
    ),
    [sessionId, sessionState, messages],
  );

  if (isInitialLoading || items.length === 0) {
    return (
      <SessionPanelContent className="relative p-4 chat-message-list">
        <MessageListStatus
          isLoadingMore={isLoadingMore}
          hasMore={hasMore}
          showLoadingState={showLoadingState}
          messagesLoading={messagesLoading}
          isInitialLoading={isInitialLoading}
          messagesCount={messages.length}
        />
        {sessionId && <PrepareProgress sessionId={sessionId} />}
        <AgentStatus sessionState={sessionState} sessionId={sessionId} messages={messages} />
      </SessionPanelContent>
    );
  }

  return (
    <SessionPanelContent ref={setScrollRef} className="relative p-4 chat-message-list">
      {scrollParent && (
        <MessageListBody
          items={items}
          messages={messages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId}
          sessionId={sessionId}
          sessionState={sessionState}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          scrollParent={scrollParent}
          isRunning={isRunning}
          lastTurnGroupId={lastTurnGroupId}
          hasMore={hasMore}
          isLoadingMore={isLoadingMore}
          loadMore={loadMore}
          Header={Header}
          Footer={Footer}
        />
      )}
    </SessionPanelContent>
  );
});
