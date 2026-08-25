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
  getLastUserMessageId,
  getFirstUserMessageId,
  resolveLastPromptControls,
} from "@/components/task/chat/message-list-shared";
import { AnchoredLastPromptBar } from "@/components/task/chat/anchored-last-prompt-bar";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useIsTaskArchived } from "./task-archived-context";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { ChatInputArea, useSubmitHandler, useChatPanelHandlers } from "./chat/chat-input-area";
import { ClarificationPanelSection } from "./chat/clarification-panel-section";
import { useComposerAgentStartHint } from "./chat/use-composer-agent-start-hint";
import { PanelSearchBar } from "@/components/search/panel-search-bar";
import { SessionSearchHits } from "@/components/task/chat/session-search-hits";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { useSessionSearch } from "@/hooks/domains/session/use-session-search";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { findUnreadDividerItemId, lastRenderedMessageId } from "@/lib/session-unread-divider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useSessionReadTracking } from "./chat/use-session-read-tracking";
import { useDrainOlderMessages } from "@/components/task/chat/use-drain-older-messages";
import { useAppStore } from "@/components/state-provider";
import { getSessionWorkspacePath } from "@/lib/session-workspace-path";
import { routePanelMouseDown } from "./chat/route-panel-mouse-down";
import { useTranslation } from "react-i18next";
import { TaskChatLaunchError } from "./simple/components/task-chat-launch-error";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";

/** Returns a `clarificationKey` that increments each time a pending
 * clarification is resolved, letting the composer reset its input state for
 * the next clarification round. */
function useClarificationKey(agentMessageCount: number) {
  const lastCountRef = useRef(agentMessageCount);
  const [clarificationKey, setClarificationKey] = useState(0);
  useEffect(() => {
    lastCountRef.current = agentMessageCount;
  }, [agentMessageCount]);
  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  return { clarificationKey, handleClarificationResolved };
}

/** Scrolls a non-Dockview host target after the message row becomes rendered. */
function usePendingMessageScroll(
  messageListRef: RefObject<MessageListHandle | null>,
  messageId: string | null | undefined,
  onConsumed: ((messageId: string) => void) | undefined,
  readinessKey: string,
) {
  useEffect(() => {
    if (!messageId) return;
    let attempts = 0;
    let frame = 0;
    const attemptScroll = () => {
      if (
        messageListRef.current?.scrollToMessage(messageId, {
          align: "start",
          behavior: "auto",
        })
      ) {
        onConsumed?.(messageId);
        return;
      }
      attempts += 1;
      if (attempts < 30) frame = requestAnimationFrame(attemptScroll);
    };
    frame = requestAnimationFrame(attemptScroll);
    return () => cancelAnimationFrame(frame);
  }, [messageId, messageListRef, onConsumed, readinessKey]);
}

/** Computes the render-item key the unread "New" divider should appear
 * immediately before: tracks the latest rendered message id for session read
 * tracking, then maps the resulting divider anchor onto the grouped items. */
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

/** Floating session-search overlay over the transcript: the search bar plus
 * its hits list, with next/prev cycling through hits. Renders nothing while
 * the search is closed. */
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
  /**
   * Task this panel belongs to, independent of whether it has a session yet.
   * Only the status row uses it, so a task with no session still shows its
   * dependency and autopilot chips. `taskId` above stays session-gated because
   * it also drives plan mode, the composer, and read tracking.
   */
  statusTaskId?: string | null;
  onOpenFile?: (path: string, repo?: string) => void;
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
  panelId?: string | null;
  /** Message ID requested by a non-Dockview host, such as the phone history surface. */
  pendingScrollToMessageId?: string | null;
  /** Called after a non-Dockview scroll target reaches a rendered row. */
  onPendingScrollConsumed?: (messageId: string) => void;
};

type ScrollTargetConsumptionParams = {
  resolvedSessionId: string | null;
  isVisible: boolean;
  panelId: string | null;
  messageListRef: React.RefObject<MessageListHandle | null>;
  /** Message-list readiness: a no-op `scrollToMessage` (row not rendered yet)
   * is retried when this flips, instead of clearing the target. */
  isInitialMessagesLoading: boolean;
  /** Rendered-transcript revision: retry a retained target when a row mounts
   * after the initial load or when older pages are prepended. */
  renderedMessageCount: number;
};

/**
 * Prompt-history transcript-jump consumption (Task 03 contract).
 *
 * Two deliberately separate effects:
 * - LIFECYCLE, keyed ONLY by `resolvedSessionId`: on a session change it
 *   clears a retained target for the PREVIOUS session (the canonical `chat`
 *   host swaps sessions without ever unmounting), and its unmount cleanup
 *   clears by the last resolved session (canonical-host removal during a
 *   layout restore keeps the component mounted, so the removal-path clear in
 *   dockview-layout-setup is the always-running fallback). It must NOT depend
 *   on `isVisible`, so an inactive→active transition never triggers it.
 * - CONSUMPTION, keyed by `scrollTarget`, `resolvedSessionId`, `isVisible`,
 *   and message-list readiness, with NO cleanup: it consumes on activation
 *   and clears only on a successful scroll (compare-and-clear by token).
 */
export function useScrollTargetConsumption({
  resolvedSessionId,
  isVisible,
  panelId,
  messageListRef,
  isInitialMessagesLoading,
  renderedMessageCount,
}: ScrollTargetConsumptionParams) {
  const scrollTarget = useDockviewStore((state) => state.scrollTarget);
  const clearScrollTarget = useDockviewStore((state) => state.clearScrollTarget);
  const clearScrollTargetForOwner = useDockviewStore((state) => state.clearScrollTargetForOwner);
  const previousSessionId = useRef<string | null>(null);

  useEffect(() => {
    // Only a DOCKVIEW host (defined panelId) may invalidate or clear a
    // retained target: non-dockview callers (preview-session-tabs,
    // task-center-panel, mobile) mount/unmount and swap resolved sessions
    // freely, and their cleanup must not clear a target that a real dockview
    // host (canonical `chat` / `session:<id>`) is waiting to consume.
    if (!panelId) return;
    const previous = previousSessionId.current;
    previousSessionId.current = resolvedSessionId;
    if (previous && previous !== resolvedSessionId) {
      if (panelId) clearScrollTargetForOwner(previous, panelId);
    }
    return () => {
      if (resolvedSessionId && panelId) {
        clearScrollTargetForOwner(resolvedSessionId, panelId);
      }
    };
  }, [clearScrollTargetForOwner, panelId, resolvedSessionId]);

  useEffect(() => {
    if (
      !scrollTarget ||
      !panelId ||
      !isVisible ||
      scrollTarget.sessionId !== resolvedSessionId ||
      scrollTarget.hostPanelId !== panelId
    ) {
      return;
    }
    // Defer the scroll two animation frames: re-showing a dockview panel
    // (this jump's own `setActive` while the history panel was up) makes
    // SessionPanelContent restore the saved scrollTop in a rAF that can land
    // anywhere in the frame after the show — an immediate smooth scroll (or
    // one deferred a single frame) is canceled by it. Two frames guarantee
    // the restore has run before the jump scroll starts. Re-check the live
    // target by token before scrolling so a superseded/cleared intent can
    // never scroll a stale row.
    let frame = requestAnimationFrame(() => {
      frame = requestAnimationFrame(() => {
        const latest = useDockviewStore.getState().scrollTarget;
        if (!latest || latest.token !== scrollTarget.token) return;
        if (messageListRef.current?.scrollToMessage(scrollTarget.messageId, { align: "start" })) {
          clearScrollTarget(scrollTarget.token);
        }
      });
    });
    return () => cancelAnimationFrame(frame);
  }, [
    clearScrollTarget,
    isInitialMessagesLoading,
    isVisible,
    messageListRef,
    panelId,
    renderedMessageCount,
    resolvedSessionId,
    scrollTarget,
  ]);
}

// eslint-disable-next-line complexity, max-lines-per-function -- composes many sub-panels; each concern already factored into its own hook
export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  taskId: taskIdHint = null,
  statusTaskId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  hideSessionsDropdown,
  isVisible = true,
  panelId = null,
  pendingScrollToMessageId = null,
  onPendingScrollConsumed,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const launchErrorContext = useTaskLaunchErrorContext();
  const launchStatusSummary = useTaskStatusSummary(
    launchErrorContext?.taskId,
    launchErrorContext?.statusSummary,
  );

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
  const showAgentStartHint = useComposerAgentStartHint(
    resolvedSessionId,
    session?.state,
    allMessages,
    footerActionMessages,
  );
  const { handleCancelTurn } = useChatPanelHandlers(resolvedSessionId, chatInputRef);
  const { clarificationKey, handleClarificationResolved } = useClarificationKey(agentMessageCount);

  const panelRef = useRef<HTMLDivElement>(null);
  const messageListRef = useRef<MessageListHandle>(null);
  useScrollTargetConsumption({
    resolvedSessionId,
    isVisible,
    panelId,
    messageListRef,
    isInitialMessagesLoading,
    renderedMessageCount: allMessages.length,
  });
  usePendingMessageScroll(
    messageListRef,
    pendingScrollToMessageId,
    onPendingScrollConsumed,
    `${allMessages.length}:${isInitialMessagesLoading}`,
  );
  const lastPromptMessageId = useMemo(() => getLastUserMessageId(allMessages), [allMessages]);
  const lastPromptMessage = useMemo(
    () =>
      lastPromptMessageId
        ? (allMessages.find((message) => message.id === lastPromptMessageId) ?? null)
        : null,
    [allMessages, lastPromptMessageId],
  );
  const [lastPromptEdge, setLastPromptEdge] = useState<LastPromptEdge>("visible");
  const showAnchoredPromptBar = useAppStore((state) => state.userSettings.showAnchoredPromptBar);
  const showScrollToLastPrompt = useAppStore((state) => state.userSettings.showScrollToLastPrompt);
  const showScrollToStart = useAppStore((state) => state.userSettings.showScrollToStart);
  const { isMobile } = useResponsiveBreakpoint();
  // The anchored bar is a desktop-only, opt-in affordance; mobile always
  // falls back to the scroll button.
  const showAnchoredBar = !isMobile && showAnchoredPromptBar;
  const [anchoredBarHeight, setAnchoredBarHeight] = useState(0);
  const { anchoredBarVisible, scrollButtonEligible, scrollDirection } =
    resolveLastPromptControls(lastPromptEdge);
  const showScrollButton =
    showScrollToLastPrompt && Boolean(lastPromptMessageId) && scrollButtonEligible;
  const scrollToLastPrompt = useCallback(() => {
    if (lastPromptMessageId) {
      messageListRef.current?.scrollToMessage(lastPromptMessageId, { align: "start" });
    }
  }, [lastPromptMessageId]);
  const firstMessageId = useMemo(() => getFirstUserMessageId(allMessages), [allMessages]);
  const [isFirstMessageHidden, setIsFirstMessageHidden] = useState(false);
  const showScrollToStartButton =
    showScrollToStart && Boolean(firstMessageId) && isFirstMessageHidden;
  const { loadMoreRaw, hasMore } = useLazyLoadMessages(resolvedSessionId);
  // A paginated session's `firstMessageId` only reflects the oldest message in
  // the currently loaded page while `hasMore` is true — jumping there directly
  // lands on a partial-page boundary, not the transcript's real start. Drain
  // older pages first so `firstMessageId` (derived from `allMessages` above,
  // which grows as pages prepend) has settled on the true first prompt by the
  // time the scroll fires.
  const [pendingScrollToStart, setPendingScrollToStart] = useState(false);
  useDrainOlderMessages(resolvedSessionId, pendingScrollToStart && hasMore);
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
  // Search can target backend rows before the visible transcript boundary.
  const search = useSessionSearch(resolvedSessionId, loadMoreRaw);
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
        {launchErrorContext && (
          <TaskChatLaunchError
            taskId={launchErrorContext.taskId}
            workspaceId={launchErrorContext.workspaceId}
            statusSummary={launchStatusSummary}
            runErrors={[]}
            repositories={launchErrorContext.repositories}
          />
        )}
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
          worktreePath={getSessionWorkspacePath(session)}
          onOpenFile={onOpenFile}
          dividerBeforeItemKey={dividerBeforeItemKey}
          lastPromptMessageId={lastPromptMessageId}
          onLastPromptEdgeChange={setLastPromptEdge}
          firstMessageId={firstMessageId}
          onFirstMessageHiddenChange={setIsFirstMessageHidden}
          anchoredBarHeight={showAnchoredBar && lastPromptMessage ? anchoredBarHeight : 0}
          stickyPromptBar={
            showAnchoredBar && lastPromptMessage ? (
              <AnchoredLastPromptBar
                promptText={lastPromptMessage.content}
                isVisible={anchoredBarVisible}
                onScrollUp={scrollToLastPrompt}
                showScrollToLastPrompt={showScrollToLastPrompt}
                onHeightChange={setAnchoredBarHeight}
              />
            ) : undefined
          }
        />
        <SessionSearchOverlay search={search} agentLabel={agentLabel} agentName={agentName} />
      </PanelBody>
      {!isArchived && (
        <ClarificationPanelSection
          pending={Boolean(pendingClarification)}
          messages={pendingClarificationGroup}
          onResolved={handleClarificationResolved}
          shortcutScopeRef={panelRef}
          maxHeightVh={50}
        />
      )}
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
        statusTaskId={statusTaskId ?? taskIdHint}
        showAgentStartHint={showAgentStartHint}
      />
    </PanelRoot>
  );
});

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
  statusTaskId: string | null;
  /** Recovered-idle sessions render the composer hint (see ChatInputArea). */
  showAgentStartHint: boolean;
};

/**
 * Composer footer: renders the chat input area (or the read-only archived
 * banner) and forwards the recovered-idle agent-start-hint visibility from
 * the panel down to the input.
 */
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
  statusTaskId,
  showAgentStartHint,
}: ChatFooterProps) {
  const { t } = useTranslation();
  if (isArchived) {
    return (
      <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
        {t("task:thisTaskIsArchivedAndRead")}
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
      statusTaskId={statusTaskId}
      showAgentStartHint={showAgentStartHint}
    />
  );
}
