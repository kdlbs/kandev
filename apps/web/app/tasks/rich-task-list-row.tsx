"use client";

import { IconAlertCircle, IconSubtask } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { PRTaskIcon } from "@/components/github/pr-task-icon";
import { RegisteredChangeRequestTaskIcon } from "@/components/integrations/registered-change-request-task-icon";
import { TaskRowMetadata } from "@/components/task/task-row-plugin-slots";
import { MRTaskIcon } from "@/components/gitlab/mr-task-icon";
import { useTaskPendingInput, type PendingInput } from "@/hooks/use-task-pending-input";
import { getTaskStateIcon } from "@/lib/ui/state-icons";
import type { Repository, Task } from "@/lib/types/http";
import { resolveRichTaskRowDetails } from "./rich-task-row-details";
import { useTranslation } from "react-i18next";

type RichTaskRowDetails = ReturnType<typeof resolveRichTaskRowDetails>;

function ArchivedBadge({ task }: { task: Task }) {
  const { t } = useTranslation();
  if (!task.archived_at) return null;
  return (
    <Badge
      variant="outline"
      className="shrink-0 border-amber-500/30 px-1.5 py-0 text-[10px] text-amber-500"
    >
      {t("tasks:archived")}
    </Badge>
  );
}

function PrimaryTaskLine({
  task,
  pendingInput,
  showContributions,
}: {
  task: Task;
  pendingInput: PendingInput;
  showContributions: boolean;
}) {
  return (
    <>
      {getTaskStateIcon(task.state, "h-4 w-4 shrink-0", {
        hasPendingClarification: pendingInput.clarification,
        foregroundActivity: task.foreground_activity,
        hasPendingPermission: pendingInput.permission,
        interrupted: task.interrupted,
      })}
      <span className="min-w-0 truncate font-medium" data-testid="tasks-list-row-title">
        {task.title}
      </span>
      {showContributions && (
        <span
          className="inline-flex items-center gap-1"
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
        >
          <PRTaskIcon taskId={task.id} />
          <MRTaskIcon taskId={task.id} />
        </span>
      )}
      <RegisteredChangeRequestTaskIcon taskId={task.id} />
      <ArchivedBadge task={task} />
    </>
  );
}

function RichMetadataBadges({ details }: { details: RichTaskRowDetails }) {
  if (details.repositoryLabels.length === 0 && !details.sessionCount && !details.parentTitle) {
    return null;
  }
  return (
    <div className="mt-1 flex min-w-0 flex-wrap items-center gap-1.5 pl-6">
      {details.repositoryLabels.map((label) => (
        <Badge
          key={label}
          variant="secondary"
          className="max-w-full truncate px-1.5 py-0 text-[10px]"
        >
          {label}
        </Badge>
      ))}
      {details.parentTitle && (
        <Badge variant="outline" className="max-w-full gap-1 truncate px-1.5 py-0 text-[10px]">
          <IconSubtask className="h-3 w-3 shrink-0" />
          <span className="truncate">{details.parentTitle}</span>
        </Badge>
      )}
      {details.sessionCount && (
        <Badge variant="secondary" className="px-1.5 py-0 text-[10px]">
          {details.sessionCount} sessions
        </Badge>
      )}
    </div>
  );
}

function ReviewAttention({ attention }: { attention: RichTaskRowDetails["reviewAttention"] }) {
  const { t } = useTranslation();
  if (attention === "approval_required") {
    return (
      <div className="mt-1 flex items-center gap-1 pl-6 text-amber-700 dark:text-amber-600">
        <IconAlertCircle className="h-3.5 w-3.5 shrink-0" />
        <span className="text-[10px] font-medium">{t("tasks:approvalRequired")}</span>
      </div>
    );
  }
  if (attention !== "changes_requested") return null;
  return (
    <Badge
      className="mt-1 ml-6 border-amber-500 bg-amber-50 px-1.5 py-0 text-[10px] text-amber-600 dark:bg-amber-950/50"
      variant="outline"
    >
      {t("tasks:changesRequested")}
    </Badge>
  );
}

function RichTaskContent({
  task,
  details,
  level,
  pendingInput,
}: {
  task: Task;
  details: RichTaskRowDetails;
  level: number;
  pendingInput: PendingInput;
}) {
  return (
    <div
      className="min-w-0 py-1"
      data-testid="tasks-list-row-content"
      style={{ paddingLeft: `${level * 28}px` }}
    >
      <div className="flex min-w-0 items-center gap-2">
        <PrimaryTaskLine task={task} pendingInput={pendingInput} showContributions />
      </div>
      <RichMetadataBadges details={details} />
      <TaskRowMetadata
        taskId={task.id}
        workflowStepId={task.workflow_step_id}
        surface="task-list"
        className="mt-1 pl-6"
      />
      {details.description && (
        <p className="mt-1 line-clamp-2 pl-6 text-xs leading-tight text-muted-foreground">
          {details.description}
        </p>
      )}
      <ReviewAttention attention={details.reviewAttention} />
    </div>
  );
}

export function TaskListRowPrimaryContent({
  task,
  level,
  repositories,
  parentTasks,
  showTaskDetails,
}: {
  task: Task;
  level: number;
  repositories: Repository[];
  parentTasks: Task[];
  showTaskDetails: boolean;
}) {
  const pendingInput = useTaskPendingInput(task.primary_session_id, {
    taskId: task.id,
    taskPendingAction: task.task_pending_action,
    primarySessionState: task.primary_session_state,
    primarySessionPendingAction: task.primary_session_pending_action,
  });

  if (showTaskDetails) {
    const details = resolveRichTaskRowDetails({ task, repositories, parentTasks });
    return (
      <RichTaskContent task={task} details={details} level={level} pendingInput={pendingInput} />
    );
  }
  return (
    <div
      className="flex min-w-0 items-center gap-2"
      data-testid="tasks-list-row-content"
      style={{ paddingLeft: `${level * 28}px` }}
    >
      <PrimaryTaskLine task={task} pendingInput={pendingInput} showContributions={false} />
      <TaskRowMetadata
        taskId={task.id}
        workflowStepId={task.workflow_step_id}
        surface="task-list"
        className="flex min-w-0 shrink items-center gap-1 overflow-hidden"
      />
    </div>
  );
}
