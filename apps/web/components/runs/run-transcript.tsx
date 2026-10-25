"use client";

import { useTranslation } from "react-i18next";
import { useCallback, useEffect, useRef, useState } from "react";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useEnsureTaskSession } from "@/hooks/use-ensure-task-session";
import { type ChatInputContainerHandle } from "@/components/task/chat/chat-input-container";
import { MessageList } from "@/components/task/chat/message-list";
import { useChatPanelState } from "@/components/task/chat/use-chat-panel-state";
import {
  ChatInputArea,
  useSubmitHandler,
  useChatPanelHandlers,
} from "@/components/task/chat/chat-input-area";
import { routePanelMouseDown } from "@/components/task/chat/route-panel-mouse-down";

export function focusRunTranscriptTurn(root: HTMLElement | null, turnId?: string): boolean {
  if (!root || !turnId) return false;
  const target = Array.from(root.querySelectorAll<HTMLElement>("[data-turn-id]")).find(
    (element) => element.dataset.turnId === turnId,
  );
  if (!target) return false;

  target.focus({ preventScroll: true });
  target.scrollIntoView?.({ behavior: "auto", block: "center" });
  return true;
}

/**
 * A run's conversation, in place.
 *
 * An automation run is a thread that happens on a schedule, so the reader lands
 * on what it said and can answer it — the reply already works end to end, and
 * sending them to the task page to use it made the automation surface a
 * read-only log. Composed from the same primitives Quick Chat uses rather than
 * mounting the full task panel: this surface wants the transcript and the
 * composer, not the sessions dropdown, plan mode, or the file editors.
 */
export function RunTranscript({
  sessionId,
  taskId,
  turnId,
}: {
  sessionId: string;
  taskId: string | null;
  turnId?: string;
}) {
  const { t } = useTranslation();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const scopeRef = useRef<HTMLDivElement>(null);
  const focusedTurnKeyRef = useRef<string | null>(null);
  const [clarificationKey, setClarificationKey] = useState(0);

  useSettingsData(true);
  // Automation tasks are hidden from the boot payload by their origin, so on a
  // direct load of /automations/<id> the session row is not in the store:
  // useSession never subscribes and a reply is rejected as "session
  // unavailable". Every other surface reaches a session by way of a list that
  // carried it; this one is reachable by URL alone, so it has to fetch its own.
  useEnsureTaskSession(sessionId);
  const panelState = useChatPanelState({
    sessionId,
    taskId,
    onOpenFile: undefined,
    onOpenFileAtLine: undefined,
  });
  const { isSending, handleSubmit } = useSubmitHandler(panelState, undefined);
  const { handleCancelTurn } = useChatPanelHandlers(panelState.resolvedSessionId, chatInputRef);

  useEffect(() => {
    if (!turnId) return;
    const turnKey = `${sessionId}:${turnId}`;
    if (focusedTurnKeyRef.current === turnKey) return;
    if (focusRunTranscriptTurn(scopeRef.current, turnId)) {
      focusedTurnKeyRef.current = turnKey;
    }
  }, [panelState.groupedItems, sessionId, turnId]);

  // An automation run is not a session anyone is sitting in: replying starts
  // the agent, it works the prompt, and it shuts down again. Controls that talk
  // to a live ACP session — the model picker above all, but also mode, MCP and
  // reset context — are meaningless once that has happened, and worse than
  // meaningless while they still look operable, since changing a model on a
  // process that no longer exists silently does nothing. `isWorking` is the
  // signal rather than the session row, which parks in WAITING_FOR_INPUT
  // precisely so the run stays repliable — parked is exactly the state with no
  // agent behind it.
  const agentIsLive = panelState.isWorking;

  const handleClarificationResolved = useCallback(() => setClarificationKey((k) => k + 1), []);
  const handleScopeMouseDown = useCallback(
    (event: React.MouseEvent<HTMLDivElement>) => routePanelMouseDown(event, scopeRef),
    [],
  );

  return (
    <div
      ref={scopeRef}
      tabIndex={-1}
      onMouseDown={handleScopeMouseDown}
      className="flex min-h-0 flex-1 flex-col outline-none"
      data-testid="run-transcript"
      data-session-id={sessionId}
      data-turn-id={turnId ?? ""}
    >
      <div className="min-h-0 flex-1 overflow-hidden">
        <MessageList
          items={panelState.groupedItems}
          messages={panelState.messages}
          permissionsByToolCallId={panelState.permissionsByToolCallId}
          childrenByParentToolCallId={panelState.childrenByParentToolCallId}
          taskId={panelState.taskId ?? undefined}
          sessionId={panelState.resolvedSessionId}
          messagesLoading={panelState.messagesLoading}
          isWorking={panelState.isWorking}
          sessionState={panelState.session?.state}
          worktreePath={panelState.session?.worktree_path}
          onOpenFile={undefined}
        />
      </div>
      <ChatInputArea
        chatInputRef={chatInputRef}
        clarificationKey={clarificationKey}
        onClarificationResolved={handleClarificationResolved}
        handleSubmit={handleSubmit}
        handleCancelTurn={handleCancelTurn}
        showRequestChangesTooltip={false}
        onRequestChangesTooltipDismiss={undefined}
        panelState={panelState}
        isSending={isSending}
        hideSessionsDropdown
        hideAgentControls={!agentIsLive}
        hidePlanMode
        placeholderOverride={t("automations:replyToThisRun")}
        // The composer's default `bg-card` is a lighter plate meant to lift it
        // off the task workbench. This surface is the run's own page, which is
        // `bg-background` from the topbar down, and a lighter strip along the
        // bottom read as a panel that had been left behind.
        surfaceClassName="bg-background"
      />
    </div>
  );
}
