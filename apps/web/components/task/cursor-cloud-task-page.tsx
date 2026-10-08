import type { Task } from "@/lib/types/http";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { TaskArchivedProvider } from "./task-archived-context";
import { TaskCommands } from "@/components/task-commands";
import { TaskLaunchErrorProvider } from "@/components/task/task-launch-error-context";
import { CursorCloudTaskSurface } from "@/components/task/cursor-cloud-task-surface";
import { buildArchivedValue } from "@/components/task/task-page-content-helpers";
import type { TaskPageInnerProps } from "@/components/task/task-page-inner";
import type { ResolvedRemoteExecutor } from "@/components/task/remote-executor-view";

export function CursorCloudTaskPage({
  page,
  task,
  remote,
  repositoryLabel,
}: {
  page: Pick<TaskPageInnerProps, "connectionStatus" | "resumption"> & {
    archivedValue: ReturnType<typeof buildArchivedValue>;
    effectiveSessionId: string | null;
  };
  task: Task;
  remote: ResolvedRemoteExecutor;
  repositoryLabel: string | null;
}) {
  return (
    <TooltipProvider>
      <TaskArchivedProvider value={page.archivedValue}>
        <TaskCommands task={task} />
        <TaskLaunchErrorProvider
          value={{
            taskId: task.id,
            workspaceId: task.workspace_id,
            statusSummary: task.status_summary,
            repositories: task.repositories,
            automaticRecovery: page.resumption,
          }}
        >
          <CursorCloudTaskSurface
            task={task}
            sessionId={page.effectiveSessionId}
            status={{
              remote_state: remote.remoteState,
              remote_status_error: remote.remoteStatusError,
              remote_repository_id: remote.remoteRepositoryID,
              remote_branch: remote.remoteBranch,
              remote_pull_request_url: remote.remotePullRequestURL,
              remote_agent_url: remote.remoteAgentURL,
              remote_history_gap: remote.remoteHistoryGap,
            }}
            repositoryLabel={repositoryLabel}
            connectionStatus={page.connectionStatus}
            resumption={page.resumption}
          />
        </TaskLaunchErrorProvider>
      </TaskArchivedProvider>
    </TooltipProvider>
  );
}
