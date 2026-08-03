"use client";

import { useTranslation } from "react-i18next";
import { useCallback, useRef, useState } from "react";
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
export function RunTranscript({ sessionId, taskId }: { sessionId: string; taskId: string | null }) {
  const { t } = useTranslation();
  const chatInputRef = useRef<ChatInputContainerHandle>(null);
  const scopeRef = useRef<HTMLDivElement>(null);
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
    >
      <div className="min-h-0 flex-1 overflow-hidden">
        <MessageList
          items={panelState.groupedItems}
          messages={panelState.allMessages}
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
        hidePlanMode
        placeholderOverride={t("automations:replyToThisRun")}
      />
    </div>
  );
}
