import { useTranslation } from "react-i18next";
import { QuickChatSessionView } from "@/components/quick-chat/quick-chat-session-view";
import { TaskWorkspaceProvider } from "@/components/task/task-workspace-context";
import { ScopedTaskPRProvider } from "@/hooks/domains/github/scoped-task-pr-context";
import { MessageTaskOriginProvider } from "@/components/task/chat/messages/message-task-origin-context";
import { CoordinatorProposalProvider } from "@/components/task/chat/messages/kandev/coordinator-proposal-context";
import { useExistingControlConversation } from "@/hooks/domains/coordinator/use-existing-control-conversation";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import { useIntegratedControl } from "./integrated-control-provider";
import { ControlConversationHeader } from "./control-conversation-header";

export function IntegratedControlConversation({
  workspace,
  coordinator,
  taskId = coordinator?.conversation_task_id ?? undefined,
}: {
  workspace: ControlWorkspace;
  coordinator?: Coordinator;
  taskId?: string;
}) {
  const { t } = useTranslation();
  if (coordinator && !hasScope(workspace.scopes, SCOPE.workspaceManage)) {
    return (
      <p className="p-6 text-sm text-muted-foreground">
        {t("sidebar:integratedControl.manageRequired")}
      </p>
    );
  }
  if (!taskId) return <p className="p-6">{t("sidebar:integratedControl.noConversation")}</p>;
  return (
    <ExistingConversation
      key={taskId}
      workspace={workspace}
      coordinator={coordinator}
      taskId={taskId}
    />
  );
}

function ExistingConversation({
  workspace,
  coordinator,
  taskId,
}: {
  workspace: ControlWorkspace;
  coordinator?: Coordinator;
  taskId: string;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const result = useExistingControlConversation(workspace.id, coordinator?.id, taskId);
  const selected = control?.selectedSessions[taskId];
  const session =
    result.sessions.find((item) => item.id === selected) ??
    result.sessions.find((item) => item.is_primary) ??
    result.sessions[0];
  if (result.taskError || result.error)
    return (
      <p role="alert" className="p-6 text-destructive">
        {t("sidebar:integratedControl.conversationUnavailable")}
      </p>
    );
  if (result.taskLoading || result.isLoading || !result.isLoaded)
    return (
      <p role="status" className="p-6">
        {t("common:loading")}
      </p>
    );
  if (!result.task || !session || session.task_id !== taskId)
    return <p className="p-6 text-muted-foreground">{t("sidebar:integratedControl.noSession")}</p>;
  const archived = Boolean(result.task.archived_at);
  const conversation = (
    <fieldset disabled={archived} className="flex min-h-0 min-w-0 flex-1 flex-col">
      <QuickChatSessionView
        key={session.id}
        session={{ kind: "chat", sessionId: session.id, workspaceId: workspace.id, taskId }}
        automaticRecovery={false}
        loadSettings={false}
        hideSessionSelectors
        activityDisplay={Boolean(coordinator)}
        taskArchiveState={archived}
        readOnly={archived}
      />
    </fieldset>
  );
  return (
    <section
      className="flex min-h-0 flex-1 flex-col"
      data-testid="integrated-control-conversation"
      data-task-id={taskId}
      data-session-id={session.id}
      data-workspace-id={workspace.id}
    >
      <ControlConversationHeader
        workspace={workspace}
        taskId={taskId}
        title={result.task.title}
        sessionId={session.id}
        sessions={result.sessions}
        archived={archived}
      />
      <MessageTaskOriginProvider value={coordinator ? "coordinator" : undefined}>
        <TaskWorkspaceProvider taskId={taskId} workspaceId={workspace.id} sessionId={session.id}>
          <ScopedTaskPRProvider taskId={taskId} workspaceId={workspace.id}>
            {coordinator ? (
              <CoordinatorProposalProvider
                value={{
                  workspaceId: workspace.id,
                  coordinatorId: coordinator.id,
                  closePopover: () => undefined,
                }}
              >
                {conversation}
              </CoordinatorProposalProvider>
            ) : (
              conversation
            )}
          </ScopedTaskPRProvider>
        </TaskWorkspaceProvider>
      </MessageTaskOriginProvider>
    </section>
  );
}
