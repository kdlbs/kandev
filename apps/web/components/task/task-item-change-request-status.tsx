import { RegisteredChangeRequestTaskIcon } from "@/components/integrations/registered-change-request-task-icon";
import { TaskContributionIcons } from "./task-contribution-icons";

export function TaskItemChangeRequestStatus({
  taskId,
  prInfo,
}: {
  taskId?: string;
  prInfo?: { number: number; state: string; aggregateState?: string };
}) {
  if (!taskId && !prInfo) return null;
  return (
    <span
      data-testid="sidebar-task-change-request-status"
      className="inline-flex shrink-0 items-center gap-1"
    >
      <TaskContributionIcons taskId={taskId} prInfo={prInfo} />
      {taskId ? <RegisteredChangeRequestTaskIcon taskId={taskId} /> : null}
    </span>
  );
}
