"use client";

import { useCallback, useEffect, useMemo, useRef, useState, memo, type RefObject } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import { MessageList } from "@/components/task/chat/message-list";
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
import { useAppStore } from "@/components/state-provider";
import type { Message } from "@/lib/types/http";

function useClarificationKey(agentMessageCount: number) {
  const lastCountRef = useRef(agentMessageCount);
  const [clarificationKey, setClarificationKey] = useState(0);
  useEffect(() => {
    lastCountRef.current = agentMessageCount;
  }, [agentMessageCount]);
  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  return { clarificationKey, handleClarificationResolved };
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
  return useMemo(() => {
    if (!profile) return { label: null, name: null };
    const parts = profile.label.split(" \u2022 ");
    const label = parts[1] || parts[0] || profile.label;
    return { label, name: profile.agent_name ?? null };
  }, [profile]);
}

type TaskChatPanelProps = {
  onSend?: (message: string) => void;
  sessionId?: string | null;
  onOpenFile?: (path: string) => void;
  showRequestChangesTooltip?: boolean;
  onRequestChangesTooltipDismiss?: () => void;
  /** Callback to open a file at a specific line (for comment clicks) */
  onOpenFileAtLine?: (filePath: string) => void;
  /** Hide the sessions dropdown (session tabs in dockview replace it) */
  hideSessionsDropdown?: boolean;
};

// eslint-disable-next-line max-lines-per-function -- composes many sub-panels; each concern already factored into its own hook
export const TaskChatPanel = memo(function TaskChatPanel({
  onSend,
  sessionId = null,
  onOpenFile,
  showRequestChangesTooltip = false,
  onRequestChangesTooltipDismiss,
  onOpenFileAtLine,
  hideSessionsDropdown,
}: TaskChatPanelProps) {
  const isArchived = useIsTaskArchived();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);

  useSettingsData(true);
  const panelState = useChatPanelState({ sessionId, onOpenFile, onOpenFileAtLine });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, onSend);
  const {
    resolvedSessionId,
    session,
    task,
    taskId,
    isWorking,
    messagesLoading,
    groupedItems,
    allMessages,
    footerActionMessages,
    permissionsByToolCallId,
    childrenByParentToolCallId,
    agentMessageCount,
    pendingClarification,
    pendingClarificationGroup,
  } = panelState;
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
  const { loadMore } = useLazyLoadMessages(resolvedSessionId);
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
          taskState={task?.state}
          worktreePath={session?.worktree_path}
          onOpenFile={onOpenFile}
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
};

function ClarificationSection({
  pendingClarification,
  isArchived,
  containerRef,
  resizeProps,
  height,
  messages,
  onResolved,
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
        <ClarificationInputOverlay messages={messages} onResolved={onResolved} />
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
    />
  );
}

// routePanelMouseDown routes non-interactive clicks to the panel root so that
// Ctrl+F can detect focus within the session panel. Extracted to keep
// TaskChatPanel's cyclomatic complexity within limits.
const interactiveSelector =
  "input, textarea, select, button, a, [contenteditable], [tabindex]:not([tabindex='-1'])";

function routePanelMouseDown(
  e: React.MouseEvent<HTMLDivElement>,
  ref: React.RefObject<HTMLDivElement | null>,
): void {
  const target = e.target as HTMLElement | null;
  if (!target) return;
  // Exclude `tabindex="-1"` so we don't match PanelRoot itself (which is
  // marked focus-receivable but shouldn't short-circuit this handler).
  if (target.closest(interactiveSelector)) return;
  ref.current?.focus({ preventScroll: true });
}
