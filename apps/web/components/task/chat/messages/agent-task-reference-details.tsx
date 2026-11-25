"use client";

import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { IconCalendar, IconLayoutKanban, IconMessages, IconSubtask } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { TaskStateIcon, getTaskStateIconLabelKey } from "@/components/task/task-state-icon";
import { useTaskReferenceDetails } from "@/hooks/domains/task/use-task-reference-details";
import { formatDateTime } from "@/lib/i18n/formats";
import type { TaskMentionReference } from "@/lib/tasks/task-mention-references";

function DetailRow({
  icon,
  label,
  children,
}: {
  icon: ReactNode;
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="grid grid-cols-[6rem_minmax(0,1fr)] items-start gap-2">
      <dt className="flex items-center gap-1.5 text-muted-foreground">
        {icon}
        {label}
      </dt>
      <dd className="min-w-0">{children}</dd>
    </div>
  );
}

function ReferenceBadges({ task }: Pick<ReturnType<typeof useTaskReferenceDetails>, "task">) {
  const { t } = useTranslation();
  if (!task) return null;
  const stateProps = { state: task.state, foregroundActivity: task.foregroundActivity };
  return (
    <div className="flex flex-wrap gap-1.5">
      {task.state && (
        <Badge variant="outline" className="h-6 gap-1.5 text-[11px]">
          <TaskStateIcon {...stateProps} showBackgroundTooltip={false} />
          {t(getTaskStateIconLabelKey(stateProps))}
        </Badge>
      )}
      {task.sessionCount != null && (
        <Badge variant="secondary" className="h-6 gap-1.5 text-[11px]">
          <IconMessages aria-hidden="true" />
          {t("task:sessionCount", { count: task.sessionCount })}
        </Badge>
      )}
    </div>
  );
}

function ReferenceHeading({
  reference,
  task,
}: { reference: TaskMentionReference } & Pick<ReturnType<typeof useTaskReferenceDetails>, "task">) {
  const { t } = useTranslation();
  return (
    <div className="space-y-1">
      {task?.identifier && (
        <div className="text-[11px] text-muted-foreground">{task.identifier}</div>
      )}
      <div className="text-sm font-medium leading-snug">{reference.title}</div>
      {task && task.title !== reference.title && (
        <div className="text-muted-foreground">
          {t("task:referenceCurrentTitle", { title: task.title })}
        </div>
      )}
    </div>
  );
}

export function AgentTaskReferenceDetails({ reference }: { reference: TaskMentionReference }) {
  const { t } = useTranslation();
  const { task, workflowName, parentTask, createdTimestamp } = useTaskReferenceDetails(
    reference.taskId,
  );
  return (
    <div className="flex flex-col gap-2.5 whitespace-normal break-words text-xs [overflow-wrap:anywhere]">
      <ReferenceHeading reference={reference} task={task} />
      <ReferenceBadges task={task} />
      <dl className="space-y-2">
        {createdTimestamp !== null && (
          <DetailRow
            label={t("task:created")}
            icon={<IconCalendar className="size-3.5 shrink-0" aria-hidden="true" />}
          >
            <time dateTime={task?.createdAt}>{formatDateTime(createdTimestamp)}</time>
          </DetailRow>
        )}
        {workflowName && (
          <DetailRow
            label={t("common:workflow")}
            icon={<IconLayoutKanban className="size-3.5 shrink-0" aria-hidden="true" />}
          >
            {workflowName}
          </DetailRow>
        )}
        {parentTask?.title && (
          <DetailRow
            label={t("task:parent")}
            icon={<IconSubtask className="size-3.5 shrink-0" aria-hidden="true" />}
          >
            {parentTask.title}
          </DetailRow>
        )}
      </dl>
      {!task && <div className="text-muted-foreground">{t("task:referenceOpenForDetails")}</div>}
    </div>
  );
}
