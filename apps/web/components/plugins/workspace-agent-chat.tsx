"use client";

import { useMemo, useState } from "react";
import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { PluginConversationScopeProvider } from "@/lib/plugins/conversation-scope";
import {
  canRecoverManagedConversation,
  canSubmitManagedConversationInput,
} from "@/lib/plugins/managed-conversation";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useManagedConversationChat } from "@/hooks/use-managed-conversation-chat";
import type { PluginTranslationOptions, PluginWorkspaceAgentChatProps } from "@kandev/plugin-sdk";
import { generateUUID } from "@/lib/uuid";
import {
  EMPTY_WORKSPACE_AGENT_CHAT_COMPOSER_STATE,
  WorkspaceAgentChatComposer,
  type WorkspaceAgentChatComposerState,
} from "./workspace-agent-chat-composer";
import { WorkspaceAgentChatInstancePicker } from "./workspace-agent-chat-instance-picker";
import { WorkspaceAgentChatTranscript } from "./workspace-agent-chat-transcript";
import {
  ConversationControls,
  ManagedChatEmptyPanel,
  ManagedChatQueueCount,
  MobileChatPanels,
} from "./workspace-agent-chat-panels";
import { WorkspaceAgentChatRecoveryConfirmation } from "./workspace-agent-chat-recovery-confirmation";

type Props = PluginWorkspaceAgentChatProps & {
  pluginId: string;
  onOpenSettings(): void;
};

export function WorkspaceAgentChat({ ...props }: Props) {
  const identity = JSON.stringify([
    props.pluginId,
    props.conversation.workspaceId,
    props.conversation.instanceKey,
  ]);
  const [composerStates, setComposerStates] = useState<
    Record<string, WorkspaceAgentChatComposerState>
  >({});
  const composerState = composerStates[identity] ?? EMPTY_WORKSPACE_AGENT_CHAT_COMPOSER_STATE;
  const updateComposerState = (patch: Partial<WorkspaceAgentChatComposerState>) => {
    setComposerStates((current) => ({
      ...current,
      [identity]: { ...(current[identity] ?? EMPTY_WORKSPACE_AGENT_CHAT_COMPOSER_STATE), ...patch },
    }));
  };

  return (
    <WorkspaceAgentChatView
      key={identity}
      {...props}
      composerState={composerState}
      updateComposerState={updateComposerState}
    />
  );
}

type WorkspaceAgentChatViewProps = Props & {
  composerState: WorkspaceAgentChatComposerState;
  updateComposerState(patch: Partial<WorkspaceAgentChatComposerState>): void;
};

// eslint-disable-next-line max-lines-per-function, complexity -- This view binds one conversation's state to desktop and mobile surfaces.
function WorkspaceAgentChatView({
  pluginId,
  conversation,
  controller,
  instances = [],
  onSelectInstance,
  tasksPanel,
  outcomesPanel,
  onStatus,
  onOpenSettings,
  composerState,
  updateComposerState,
}: WorkspaceAgentChatViewProps) {
  const { t } = useTranslation("plugins");
  const { isMobile, isFinePointer } = useResponsiveBreakpoint();
  const touchTargets = isMobile || !isFinePointer;
  const [mobileTab, setMobileTab] = useState("chat");
  const [recoveryConfirmationOpen, setRecoveryConfirmationOpen] = useState(false);
  const chat = useManagedConversationChat(conversation, controller, onStatus);
  const { status, inputs, busy, transportFailed, actionError } = chat;
  const canSubmit = useMemo(
    () =>
      canSubmitManagedConversationInput({
        state: transportFailed ? "disconnected" : status.state,
        readOnly: status.readOnly ?? false,
      }),
    [status.readOnly, status.state, transportFailed],
  );
  const pendingCount = inputs.filter(
    (input) => input.state === "accepted" || input.state === "running",
  ).length;
  const canRecover = canRecoverManagedConversation({
    supported: status.recoverySupported ?? false,
    readOnly: status.readOnly ?? false,
    sessionState: status.sessionState,
    sessionResourceVersion: status.sessionResourceVersion,
    executionId: status.executionId,
  });
  const tasks = (tasksPanel as ReactNode) ?? <ManagedChatEmptyPanel kind="tasks" />;
  const outcomes = (outcomesPanel as ReactNode) ?? <ManagedChatEmptyPanel kind="outcomes" />;
  const transcript = (
    <WorkspaceAgentChatTranscript
      taskId={status.taskId}
      sessionId={status.sessionId}
      touchTargets={touchTargets}
    />
  );
  const controls = (
    <ConversationControls
      status={status}
      inputs={inputs}
      touchTargets={touchTargets}
      busy={busy}
      cancelInput={chat.cancelInput}
      permission={chat.respondPermission}
      clarification={chat.respondClarification}
    />
  );
  const composer = (
    <WorkspaceAgentChatComposer
      enabled={canSubmit}
      touchTargets={touchTargets}
      generateKey={generateUUID}
      onSubmit={chat.queueMessage}
      state={composerState}
      updateState={updateComposerState}
    />
  );
  const statusMessage = getStatusMessage(status.state, transportFailed, pendingCount, t);

  return (
    <PluginConversationScopeProvider
      pluginId={pluginId}
      taskId={status.taskId}
      sessionId={status.sessionId}
      presentation={isMobile ? "mobile" : "desktop"}
    >
      <div
        className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden"
        data-testid="workspace-agent-chat"
      >
        <header className="z-20 flex shrink-0 flex-wrap items-center gap-2 border-b bg-background p-2">
          <WorkspaceAgentChatInstancePicker
            instances={instances}
            selectedKey={status.instanceKey}
            touchTargets={touchTargets}
            onSelect={(key) => onSelectInstance?.(key)}
          />
          <div className="ml-auto flex items-center gap-2">
            <ManagedChatQueueCount count={pendingCount} />
            <Button
              type="button"
              variant="outline"
              className={touchTargets ? "min-h-11" : "min-h-7"}
              disabled={busy || status.readOnly || !["ready", "paused"].includes(status.state)}
              onClick={chat.togglePaused}
            >
              {status.state === "paused" ? t("managedChatResume") : t("managedChatPause")}
            </Button>
          </div>
        </header>

        {statusMessage ? (
          <div
            role="status"
            className="flex shrink-0 flex-wrap items-center gap-2 border-b bg-muted/50 px-3 py-2 text-sm"
          >
            <span>{statusMessage}</span>
            {status.state === "revoked" || status.state === "unsupported" ? (
              <Button
                type="button"
                size="sm"
                variant="link"
                className={touchTargets ? "min-h-11" : "min-h-7"}
                onClick={onOpenSettings}
              >
                {t("managedChatOpenPluginSettings")}
              </Button>
            ) : null}
            {canRecover ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className={touchTargets ? "min-h-11" : "min-h-7"}
                disabled={busy}
                onClick={() => setRecoveryConfirmationOpen(true)}
              >
                {t("managedChatRecover")}
              </Button>
            ) : null}
          </div>
        ) : null}
        {actionError ? (
          <p role="alert" className="shrink-0 px-3 py-2 text-sm text-destructive">
            {t("managedChatActionFailed")}
          </p>
        ) : null}

        {isMobile ? (
          <MobileChatPanels
            tab={mobileTab}
            onTab={setMobileTab}
            tasks={tasks}
            outcomes={outcomes}
            transcript={transcript}
            controls={controls}
            composer={composer}
          />
        ) : (
          <div className="flex min-h-0 min-w-0 flex-1">
            <section
              className="flex min-h-0 min-w-0 flex-1 flex-col"
              aria-label={t("managedChatChatTab")}
            >
              {controls}
              {transcript}
              {composer}
            </section>
            <aside className="grid w-[min(38%,28rem)] min-w-[18rem] min-h-0 grid-rows-[minmax(0,1.5fr)_minmax(0,1fr)] border-l">
              <div className="min-h-0 overflow-y-auto p-3">{tasks}</div>
              <div className="min-h-0 overflow-y-auto border-t p-3">{outcomes}</div>
            </aside>
          </div>
        )}
      </div>
      <WorkspaceAgentChatRecoveryConfirmation
        open={recoveryConfirmationOpen}
        onOpenChange={setRecoveryConfirmationOpen}
        onConfirm={chat.recover}
      />
    </PluginConversationScopeProvider>
  );
}

function getStatusMessage(
  state: PluginWorkspaceAgentChatProps["conversation"]["state"],
  disconnected: boolean,
  queuedCount: number,
  t: (key: string, options?: PluginTranslationOptions) => string,
) {
  if (disconnected || state === "disconnected") return t("managedChatDisconnected");
  if (state === "ready") return "";
  if (state === "paused") return t("managedChatPaused", { count: queuedCount });
  return t(`managedChatState_${state}`);
}
