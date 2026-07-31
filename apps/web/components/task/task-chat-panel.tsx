"use client";

import { useCallback, useEffect, useMemo, useRef, useState, memo, type RefObject } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import {
  type ChatInputContainerHandle,
  type ChatSubmitPayload,
  type ChatSubmitResult,
} from "@/components/task/chat/chat-input-container";
import { MessageList } from "@/components/task/chat/message-list";
import {
  type MessageListHandle,
  type LastPromptEdge,
  getFirstUserMessageId,
  resolveLastPromptControls,
  resolveEffectiveLastPromptEdge,
} from "@/components/task/chat/message-list-shared";
import { AnchoredLastPromptBar } from "@/components/task/chat/anchored-last-prompt-bar";
import { useLastUserMessage } from "@/hooks/domains/session/use-last-user-message";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useIsTaskArchived } from "./task-archived-context";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { ChatInputArea, useSubmitHandler, useChatPanelHandlers } from "./chat/chat-input-area";
import { ClarificationInputOverlay } from "./chat/clarification-input-overlay";
import { ResizeHandle } from "./chat/resize-handle";
import { useResizableClarificationOverlay } from "@/hooks/use-resizable-clarification-overlay";
import { PanelSearchBar } from "@/components/search/panel-search-bar";
import { SessionSearchHits } from "@/components/task/chat/session-search-hits";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { useSessionSearch } from "@/hooks/domains/session/use-session-search";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { findUnreadDividerItemId, lastRenderedMessageId } from "@/lib/session-unread-divider";
import { useSessionReadTracking } from "./chat/use-session-read-tracking";
import { useDrainOlderMessages } from "@/components/task/chat/use-drain-older-messages";
import { useAppStore } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";
import { routePanelMouseDown } from "./chat/route-panel-mouse-down";

function useClarificationKey(agentMessageCount: number) {
  const lastCountRef = useRef(agentMessageCount);
  const [clarificationKey, setClarificationKey] = useState(0);
  useEffect(() => {
    lastCountRef.current = agentMessageCount;
  }, [agentMessageCount]);
  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  return { clarificationKey, handleClarificationResolved };
}

function useUnreadDividerBeforeItemKey(
  sessionId: string | null,
  isVisible: boolean,
  groupedItems: Parameters<typeof lastRenderedMessageId>[0],
  isInitialMessagesLoading: boolean,
) {
  const latestMessageId = useMemo(() => lastRenderedMessageId(groupedItems), [groupedItems]);
  const dividerAnchor = useSessionReadTracking(
    sessionId,
    isVisible,
    latestMessageId,
    isInitialMessagesLoading,
  );
  return useMemo(
    () => findUnreadDividerItemId(groupedItems, dividerAnchor),
    [groupedItems, dividerAnchor],
  );
}

function SessionSearchOverlay({
  search,
  agentLabel,
  agentName,
}: {
  search: ReturnType<typeof useSessionSearch>;
  agentLabel: string | null;
  agentName: string | null;
}) {
  const currentIdx = search.activeHitId
    ? search.hits.findIndex((h) => h.id === search.activeHitId)
    : -1;
  const total = search.hits.length;
  const handleNext = useCallback(() => {
    if (!total) return;
    const next = search.hits[(Math.max(currentIdx, -1) + 1) % total];
    if (next) search.setActiveHit(next.id);
  }, [search, currentIdx, total]);
  const handlePrev = useCallback(() => {
    if (!total) return;
    const prevIdx = (Math.max(currentIdx, 0) - 1 + total) % total;
    const prev = search.hits[prevIdx];
    if (prev) search.setActiveHit(prev.id);
  }, [search, currentIdx, total]);
  if (!search.isOpen) return null;
  return (
    <div className="absolute top-2 right-2 z-20 flex flex-col items-end gap-1">
      <PanelSearchBar
        className="static"
        value={search.query}
        onChange={search.setQuery}
        onNext={handleNext}
        onPrev={handlePrev}
        onClose={search.close}
        matchInfo={{ current: currentIdx >= 0 ? currentIdx + 1 : 0, total }}
        isLoading={search.isSearching}
        // Session search already debounces in useDebouncedSearch; skip the
        // bar's debounce so we don't stack 150ms + 180ms per keystroke.
        debounceMs={0}
      />
      <SessionSearchHits
        hits={search.hits}
        query={search.query}
        activeHitId={search.activeHitId}
        onSelect={search.setActiveHit}
        isSearching={search.isSearching}
        agentLabel={agentLabel}
        agentName={agentName}
      />
    </div>
  );
}

/** Returns the AgentProfileOption for the session's profile, or null. Uses
 * primitive profile id to avoid getSnapshot-cache errors from returning
 * fresh objects on every selector call. */
function useSessionAgentProfile(sessionId: string | null | undefined) {
  const profileId = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.agent_profile_id ?? null) : null,
  );
  return useAppStore((state) =>
    profileId
      ? (state.agentProfiles.items.find((p: { id: string }) => p.id === profileId) ?? null)
      : null,
  );
}

/** Resolves the agent profile name + registry slug for the given session.
 * Label is "Profile Name" from the "Agent • Profile Name" store label; slug
 * feeds <AgentLogo> which fetches the logo by agent type. */
function useSessionAgentIdentity(sessionId: string | null | undefined): {
  label: string | null;
  name: string | null;
} {
  const profile = useSessionAgentProfile(sessionId);
  // User-supplied session name wins over the derived profile label,
  // matching the session tab title precedence (resolveSessionTabTitle).
  const sessionName = useAppStore((state) =>
    sessionId ? (state.taskSessions.items[sessionId]?.name ?? null) : null,
  );
  return useMemo(() => {
    if (!profile) return { label: sessionName, name: null };
    const parts = profile.label.split(" \u2022 ");
    const label = sessionName || parts[1] || parts[0] || profile.label;
    return { label, name: profile.agent_name ?? null };
  }, [profile, sessionName]);
}

type TaskChatPanelProps = {
  onSend?: (payload: ChatSubmitPayload) => ChatSubmitResult;
  sessionId?: string | null;
  taskId?: string | null;
  onOpenFile?: (path: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
  /** Hide the sessions dropdown (session tabs in dockview replace it) */
  hideSessionsDropdown?: boolean;
  /**
   * Whether this panel is the one actually on screen — gates the
   * Slack-style unread-divider read tracking (see
   * chat/use-session-read-tracking.ts). Dockview-hosted callers must pass
   * real tab-activation state (see hooks/use-panel-active.ts); other hosts
   * (mobile, quick chat, kanban preview) default to true since mounting
   * already implies visibility for them.
   */
  isVisible?: boolean;
};

// eslint-disable-next-line complexity, max-lines-per-function -- composes many sub-panels; each concern already factored into its own hook
export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  taskId: taskIdHint = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  hideSessionsDropdown,
  isVisible = true,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);

  useSettingsData(true);
  const panelState = useChatPanelState({
    sessionId,
    taskId: taskIdHint,
    onOpenFile,
    onOpenFileAtLine,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, onSend);
  const {
    resolvedSessionId,
    session,
    taskId,
    isWorking,
    messagesLoading,
    isInitialMessagesLoading,
    groupedItems,
    allMessages,
    footerActionMessages,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    agentMessageCount,
    pendingClarification,
    pendingClarificationGroup,
  } = panelState;
  const dividerBeforeItemKey = useUnreadDividerBeforeItemKey(
    resolvedSessionId,
    isVisible,
    groupedItems,
    isInitialMessagesLoading,
  );
  const { handleCancelTurn } = useChatPanelHandlers(resolvedSessionId, chatInputRef);
  const { clarificationKey, handleClarificationResolved } = useClarificationKey(agentMessageCount);
  const {
    height: clarificationHeight,
    containerRef: clarificationContainerRef,
    resetHeight: clarificationResetHeight,
    resizeHandleProps: clarificationResizeProps,
  } = useResizableClarificationOverlay();

  // Reset the dragged height when the overlay closes so a fresh
  // clarification starts auto-sized instead of inheriting a stale value.
  useEffect(() => {
    if (!pendingClarification) clarificationResetHeight();
  }, [pendingClarification, clarificationResetHeight]);

  const panelRef = useRef<HTMLDivElement>(null);
  const messageListRef = useRef<MessageListHandle>(null);
  // The last prompt may sit far above the initially loaded window (autonomous
  // session whose only user prompt is its task description). Resolve it with a
  // targeted fetch when the loaded window has no user message, rather than
  // paginating the whole transcript in the background.
  const { lastPromptMessage } = useLastUserMessage(resolvedSessionId, allMessages);
  const lastPromptMessageId = lastPromptMessage?.id ?? null;
  const [lastPromptEdge, setLastPromptEdge] = useState<LastPromptEdge>("visible");
  const showAnchoredPromptBar = useAppStore((state) => state.userSettings.showAnchoredPromptBar);
  const showScrollToLastPrompt = useAppStore((state) => state.userSettings.showScrollToLastPrompt);
  const showScrollToStart = useAppStore((state) => state.userSettings.showScrollToStart);
  const { isMobile } = useResponsiveBreakpoint();
  // The anchored bar is a desktop-only, opt-in affordance; mobile always
  // falls back to the scroll button.
  const showAnchoredBar = !isMobile && showAnchoredPromptBar;
  const firstMessageId = useMemo(() => getFirstUserMessageId(allMessages), [allMessages]);
  const [isFirstMessageHidden, setIsFirstMessageHidden] = useState(false);
  const showScrollToStartButton =
    showScrollToStart && Boolean(firstMessageId) && isFirstMessageHidden;
  const { loadMore, hasMore } = useLazyLoadMessages(resolvedSessionId);
  const lastPromptRendered = useMemo(
    () =>
      lastPromptMessageId
        ? allMessages.some((message) => message.id === lastPromptMessageId)
        : false,
    [allMessages, lastPromptMessageId],
  );
  // While the last prompt is resolved but not mounted and older pages remain,
  // it deterministically sits above the viewport (the loaded window is the
  // newest content), so report "above" instead of the renderer's "visible"
  // default. Once the prompt row mounts, the tracked edge takes over.
  const effectiveLastPromptEdge = resolveEffectiveLastPromptEdge({
    trackedEdge: lastPromptEdge,
    lastPromptMessageId,
    lastPromptRendered,
    hasMore,
  });
  const { anchoredBarVisible, scrollButtonEligible, scrollDirection } =
    resolveLastPromptControls(effectiveLastPromptEdge);
  const showScrollButton =
    showScrollToLastPrompt && Boolean(lastPromptMessageId) && scrollButtonEligible;
  // Scroll-to-last-prompt: a resolved prompt that is not mounted needs older
  // pages drained before the row exists to scroll to — the same drain-then-
  // scroll pattern scroll-to-start uses below.
  const [pendingScrollToLastPrompt, setPendingScrollToLastPrompt] = useState(false);
  const [pendingScrollToStart, setPendingScrollToStart] = useState(false);
  // Reset both pending navigations when the session changes so a drain started
  // on a previous tab can't keep paginating the new session.
  useEffect(() => {
    setPendingScrollToLastPrompt(false);
    setPendingScrollToStart(false);
  }, [resolvedSessionId]);
  useDrainOlderMessages(
    resolvedSessionId,
    (pendingScrollToLastPrompt || pendingScrollToStart) && hasMore,
  );
  useEffect(() => {
    if (!pendingScrollToLastPrompt) return;
    if (!lastPromptRendered && hasMore) return;
    setPendingScrollToLastPrompt(false);
    if (lastPromptMessageId) {
      messageListRef.current?.scrollToMessage(lastPromptMessageId, { align: "start" });
    }
  }, [pendingScrollToLastPrompt, lastPromptRendered, hasMore, lastPromptMessageId]);
  const scrollToLastPrompt = useCallback(() => {
    if (!lastPromptMessageId) return;
    if (hasMore && !lastPromptRendered) {
      setPendingScrollToLastPrompt(true);
      return;
    }
    messageListRef.current?.scrollToMessage(lastPromptMessageId, { align: "start" });
  }, [hasMore, lastPromptRendered, lastPromptMessageId]);
  // A paginated session's `firstMessageId` only reflects the oldest message in
  // the currently loaded page while `hasMore` is true — jumping there directly
  // lands on a partial-page boundary, not the transcript's real start. Drain
  // older pages first so `firstMessageId` (derived from `allMessages` above,
  // which grows as pages prepend) has settled on the true first prompt by the
  // time the scroll fires.
  useEffect(() => {
    if (!pendingScrollToStart || hasMore) return;
    setPendingScrollToStart(false);
    if (firstMessageId) {
      messageListRef.current?.scrollToMessage(firstMessageId, { align: "start" });
    }
  }, [pendingScrollToStart, hasMore, firstMessageId]);
  const scrollToStart = useCallback(() => {
    if (hasMore) {
      setPendingScrollToStart(true);
      return;
    }
    if (firstMessageId) {
      messageListRef.current?.scrollToMessage(firstMessageId, { align: "start" });
    }
  }, [hasMore, firstMessageId]);
  const search = useSessionSearch(resolvedSessionId, loadMore);
  const { label: agentLabel, name: agentName } = useSessionAgentIdentity(resolvedSessionId);
  usePanelSearch({
    containerRef: panelRef,
    isOpen: search.isOpen,
    onOpen: search.open,
    onClose: search.close,
  });

  // The message list has no focus-capturing child (unlike TipTap/xterm in the
  // plan/terminal panels), so clicking a message leaves focus on <body>. Make
  // the panel root itself focusable and route non-interactive clicks to it so
  // Ctrl+F can detect focus within the session panel.
  const handlePanelMouseDown = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(e, panelRef),
    [],
  );

  return (
    <PanelRoot
      ref={panelRef}
      data-testid="session-chat"
      data-panel-kind="session"
      tabIndex={-1}
      onMouseDown={handlePanelMouseDown}
      className="outline-none"
    >
      <PanelBody padding={false} className="relative">
        <MessageList
          ref={messageListRef}
          items={groupedItems}
          messages={allMessages}
          footerActionMessages={footerActionMessages}
          permissionsByToolCallId={permissionsByToolCallId}
          childrenByParentToolCallId={childrenByParentToolCallId}
          taskId={taskId ?? undefined}
          sessionId={resolvedSessionId}
          messagesLoading={messagesLoading}
          isWorking={isWorking}
          sessionState={session?.state}
          worktreePath={session?.worktree_path}
          onOpenFile={onOpenFile}
          dividerBeforeItemKey={dividerBeforeItemKey}
          lastPromptMessageId={lastPromptMessageId}
          onLastPromptEdgeChange={setLastPromptEdge}
          firstMessageId={firstMessageId}
          onFirstMessageHiddenChange={setIsFirstMessageHidden}
          stickyPromptBar={
            showAnchoredBar && lastPromptMessage ? (
              <AnchoredLastPromptBar
                promptText={lastPromptMessage.content}
                isVisible={anchoredBarVisible}
                onScrollUp={scrollToLastPrompt}
                showScrollToLastPrompt={showScrollToLastPrompt}
              />
            ) : undefined
          }
        />
        <SessionSearchOverlay search={search} agentLabel={agentLabel} agentName={agentName} />
      </PanelBody>
      <ClarificationSection
        pendingClarification={Boolean(pendingClarification)}
        isArchived={isArchived}
        containerRef={clarificationContainerRef}
        resizeProps={clarificationResizeProps}
        height={clarificationHeight}
        messages={pendingClarificationGroup}
        onResolved={handleClarificationResolved}
        shortcutScopeRef={panelRef}
      />
      <ChatFooter
        isArchived={isArchived}
        chatInputRef={chatInputRef}
        clarificationKey={clarificationKey}
        onClarificationResolved={handleClarificationResolved}
        handleSubmit={handleSubmit}
        handleCancelTurn={handleCancelTurn}
        showRequestChangesTooltip={showRequestChangesTooltip}
        onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
        panelState={panelState}
        isSending={isSending}
        hideSessionsDropdown={hideSessionsDropdown}
        showScrollToLastPrompt={showScrollButton}
        onScrollToLastPrompt={scrollToLastPrompt}
        lastPromptScrollDirection={scrollDirection}
        showScrollToStart={showScrollToStartButton}
        onScrollToStart={scrollToStart}
      />
    </PanelRoot>
  );
});

type ClarificationSectionProps = {
  pendingClarification: boolean;
  isArchived: boolean;
  containerRef: RefObject<HTMLDivElement | null>;
  resizeProps: { onMouseDown: (e: React.MouseEvent) => void; onDoubleClick: () => void };
  height: number | null;
  messages: readonly Message[] | null | undefined;
  onResolved: () => void;
  shortcutScopeRef: RefObject<HTMLElement | null>;
};

function ClarificationSection({
  pendingClarification,
  isArchived,
  containerRef,
  resizeProps,
  height,
  messages,
  onResolved,
  shortcutScopeRef,
}: ClarificationSectionProps) {
  if (!pendingClarification || isArchived) return null;
  return (
    <div className="relative flex-shrink-0 border-t border-sky-400/30 bg-card">
      <ResizeHandle {...resizeProps} />
      <div
        ref={containerRef}
        data-testid="clarification-overlay-container"
        className="px-1 overflow-y-scroll overscroll-contain max-h-[50vh]"
        style={height === null ? undefined : { height }}
      >
        <ClarificationInputOverlay
          messages={messages}
          onResolved={onResolved}
          shortcutScopeRef={shortcutScopeRef}
        />
      </div>
    </div>
  );
}

type ChatFooterProps = {
  isArchived: boolean;
  chatInputRef: RefObject<
    import("@/components/task/chat/chat-input-container").ChatInputContainerHandle | null
  >;
  clarificationKey: number;
  onClarificationResolved: () => void;
  handleSubmit: ReturnType<typeof useSubmitHandler>["handleSubmit"];
  handleCancelTurn: () => Promise<void>;
  showRequestChangesTooltip: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  panelState: ReturnType<typeof useChatPanelState>;
  isSending: boolean;
  hideSessionsDropdown?: boolean;
  showScrollToLastPrompt: boolean;
  onScrollToLastPrompt: () => void;
  lastPromptScrollDirection: "up" | "down";
  showScrollToStart: boolean;
  onScrollToStart: () => void;
};

function ChatFooter({
  isArchived,
  chatInputRef,
  clarificationKey,
  onClarificationResolved,
  handleSubmit,
  handleCancelTurn,
  showRequestChangesTooltip,
  onRequestChangesTooltipDismiss,
  panelState,
  isSending,
  hideSessionsDropdown,
  showScrollToLastPrompt,
  onScrollToLastPrompt,
  lastPromptScrollDirection,
  showScrollToStart,
  onScrollToStart,
}: ChatFooterProps) {
  if (isArchived) {
    return (
      <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
        This task is archived and read-only.
      </div>
    );
  }
  return (
    <ChatInputArea
      chatInputRef={chatInputRef}
      clarificationKey={clarificationKey}
      onClarificationResolved={onClarificationResolved}
      handleSubmit={handleSubmit}
      handleCancelTurn={handleCancelTurn}
      showRequestChangesTooltip={showRequestChangesTooltip}
      onRequestChangesTooltipDismiss={onRequestChangesTooltipDismiss}
      panelState={panelState}
      isSending={isSending}
      hideSessionsDropdown={hideSessionsDropdown}
      showScrollToLastPrompt={showScrollToLastPrompt}
      onScrollToLastPrompt={onScrollToLastPrompt}
      lastPromptScrollDirection={lastPromptScrollDirection}
      showScrollToStart={showScrollToStart}
      onScrollToStart={onScrollToStart}
    />
  );
}
