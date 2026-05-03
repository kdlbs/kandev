"use client";

import { useCallback, useEffect, useMemo, useRef, useState, memo } from "react";
import { PanelRoot, PanelBody } from "./panel-primitives";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import {
  QueuedMessageIndicator,
  type QueuedMessageIndicatorHandle,
} from "@/components/task/chat/queued-message-indicator";
import { MessageList } from "@/components/task/chat/message-list";
import { useIsTaskArchived } from "./task-archived-context";
import { useChatPanelState } from "./chat/use-chat-panel-state";
import { ChatInputArea, useSubmitHandler, useChatPanelHandlers } from "./chat/chat-input-area";
import { ClarificationInputOverlay } from "./chat/clarification-input-overlay";
import { PanelSearchBar } from "@/components/search/panel-search-bar";
import { SessionSearchHits } from "@/components/task/chat/session-search-hits";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { useSessionSearch } from "@/hooks/domains/session/use-session-search";
import { useLazyLoadMessages } from "@/hooks/use-lazy-load-messages";
import { useAppStore } from "@/components/state-provider";

type QueuedOverlayProps = {
  isQueued: boolean;
  queuedMessage: { content: string } | null | undefined;
  isArchived: boolean;
  indicatorRef: React.RefObject<QueuedMessageIndicatorHandle | null>;
  onCancel: () => void;
  onUpdate: (content: string) => Promise<void>;
  onEditComplete: () => void;
};

function QueuedMessageOverlay({
  isQueued,
  queuedMessage,
  isArchived,
  indicatorRef,
  onCancel,
  onUpdate,
  onEditComplete,
}: QueuedOverlayProps) {
  if (!isQueued || !queuedMessage || isArchived) return null;
  return (
    <div className="flex-shrink-0 bg-card px-3 pt-1.5">
      <QueuedMessageIndicator
        ref={indicatorRef}
        content={queuedMessage.content}
        onCancel={onCancel}
        onUpdate={onUpdate}
        isVisible={true}
        onEditComplete={onEditComplete}
      />
    </div>
  );
}

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
  const queuedMessageRef = useRef<QueuedMessageIndicatorHandle>(null);

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
    cancelQueue,
    pendingClarification,
    isQueued,
    queuedMessage,
    updateQueueContent,
  } = panelState;
  const { handleCancelTurn, handleCancelQueue, handleQueueEditComplete } = useChatPanelHandlers(
    resolvedSessionId,
    cancelQueue,
    chatInputRef,
  );
  const { clarificationKey, handleClarificationResolved } = useClarificationKey(agentMessageCount);

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
  const handlePanelMouseDown = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const target = e.target as HTMLElement | null;
    if (!target) return;
    // Exclude `tabindex="-1"` so we don't match PanelRoot itself (which is
    // marked focus-receivable but shouldn't short-circuit this handler).
    if (
      target.closest(
        "input, textarea, select, button, a, [contenteditable], [tabindex]:not([tabindex='-1'])",
      )
    ) {
      return;
    }
    panelRef.current?.focus({ preventScroll: true });
  }, []);

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
      {pendingClarification && !isArchived && (
        <div className="flex-shrink-0 border-t border-sky-400/30 bg-card px-1">
          <ClarificationInputOverlay
            message={pendingClarification}
            onResolved={handleClarificationResolved}
          />
        </div>
      )}
      <QueuedMessageOverlay
        isQueued={isQueued}
        queuedMessage={queuedMessage}
        isArchived={isArchived}
        indicatorRef={queuedMessageRef}
        onCancel={handleCancelQueue}
        onUpdate={updateQueueContent}
        onEditComplete={handleQueueEditComplete}
      />
      {isArchived ? (
        <div className="bg-muted/50 flex-shrink-0 px-4 py-3 text-center text-sm text-muted-foreground border-t">
          This task is archived and read-only.
        </div>
      ) : (
        <ChatInputArea
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
      )}
    </PanelRoot>
  );
});
