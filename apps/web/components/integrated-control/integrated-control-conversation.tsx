import { useTranslation } from "react-i18next";
import { QuickChatSessionView } from "@/components/quick-chat/quick-chat-session-view";
import { TaskWorkspaceProvider } from "@/components/task/task-workspace-context";
import { WorkspaceCollaborationAction } from "@/components/task/workspace-collaboration-action";
import { ScopedTaskPRProvider } from "@/hooks/domains/github/scoped-task-pr-context";
import { MessageTaskOriginProvider } from "@/components/task/chat/messages/message-task-origin-context";
import { CoordinatorProposalProvider } from "@/components/task/chat/messages/kandev/coordinator-proposal-context";
import { useExistingControlConversation } from "@/hooks/domains/coordinator/use-existing-control-conversation";
import { hasScope, SCOPE } from "@/lib/types/team-access";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";
import type { ControlWorkspace } from "@/hooks/domains/coordinator/use-integrated-control-data";
import { useIntegratedControl } from "./integrated-control-provider";

export function IntegratedControlConversation({
  workspace,
  coordinator,
}: {
  workspace: ControlWorkspace;
  coordinator: Coordinator;
}) {
  const { t } = useTranslation();
  if (!hasScope(workspace.scopes, SCOPE.workspaceManage)) {
    return (
      <p className="p-6 text-sm text-muted-foreground">
        {t("sidebar:integratedControl.manageRequired")}
      </p>
    );
  }
  if (!coordinator.conversation_task_id)
    return <p className="p-6">{t("sidebar:integratedControl.noConversation")}</p>;
  return (
    <ExistingConversation
      key={coordinator.conversation_task_id}
      workspace={workspace}
      coordinator={coordinator}
      taskId={coordinator.conversation_task_id}
    />
  );
}

function ExistingConversation({
  workspace,
  coordinator,
  taskId,
}: {
  workspace: ControlWorkspace;
  coordinator: Coordinator;
  taskId: string;
}) {
  const { t } = useTranslation();
  const control = useIntegratedControl();
  const result = useExistingControlConversation(workspace.id, coordinator.id, taskId);
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
  return (
    <section
      className="flex min-h-0 flex-1 flex-col"
      data-testid="integrated-control-conversation"
      data-task-id={taskId}
      data-session-id={session.id}
      data-workspace-id={workspace.id}
    >
      <div className="flex shrink-0 flex-wrap items-center gap-2 border-b px-4 py-2 text-xs text-muted-foreground">
        <span className="break-all">
          {t("sidebar:integratedControl.task")}: {taskId}
        </span>
        <label className="ml-auto flex min-w-0 max-w-full items-center gap-2">
          {t("sidebar:integratedControl.runtimeSession")}
          <select
            value={session.id}
            onChange={(event) => control?.selectSession(taskId, event.target.value)}
            className="min-w-0 max-w-64 rounded border bg-background p-1 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          >
            {result.sessions.map((item) => (
              <option key={item.id} value={item.id}>
                {item.id}
              </option>
            ))}
          </select>
        </label>
        {!result.task.archived_at && (
          <TaskWorkspaceProvider taskId={taskId} workspaceId={workspace.id} sessionId={session.id}>
            <WorkspaceCollaborationAction
              taskId={taskId}
              title={result.task.title}
              workspace={workspace}
            />
          </TaskWorkspaceProvider>
        )}
      </div>
      <MessageTaskOriginProvider value="coordinator">
        <TaskWorkspaceProvider taskId={taskId} workspaceId={workspace.id} sessionId={session.id}>
          <ScopedTaskPRProvider taskId={taskId} workspaceId={workspace.id}>
            <CoordinatorProposalProvider
              value={{
                workspaceId: workspace.id,
                coordinatorId: coordinator.id,
                closePopover: () => undefined,
              }}
            >
              <QuickChatSessionView
                key={session.id}
                session={{ kind: "chat", sessionId: session.id, workspaceId: workspace.id, taskId }}
                automaticRecovery={false}
                loadSettings={false}
                hideSessionSelectors
                activityDisplay
                taskArchiveState={Boolean(result.task.archived_at)}
              />
            </CoordinatorProposalProvider>
          </ScopedTaskPRProvider>
        </TaskWorkspaceProvider>
      </MessageTaskOriginProvider>
    </section>
  );
}
