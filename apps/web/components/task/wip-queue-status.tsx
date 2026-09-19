"use client";

import { IconListNumbers } from "@tabler/icons-react";
import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import Link from "@/components/routing/app-link";
import { useAppStore } from "@/components/state-provider";
import { settingsActionClassName } from "@/components/settings/settings-control";
import { buildWipQueueViewModel } from "@/lib/tasks/wip-queue-view-model";

export function WipQueueStatus({ taskId }: { taskId: string | null | undefined }) {
  const { t } = useTranslation();
  const snapshots = useAppStore((state) => state.kanbanMulti.snapshots);
  const activeWorkflowId = useAppStore((state) => state.kanban.workflowId);
  const activeTasks = useAppStore((state) => state.kanban.tasks);
  const activeSteps = useAppStore((state) => state.kanban.steps);
  const workflows = useAppStore((state) => state.workflows.items);
  const view = useMemo(
    () =>
      taskId
        ? buildWipQueueViewModel({
            taskId,
            snapshots,
            activeWorkflowId,
            activeTasks,
            activeSteps,
            workflows,
          })
        : null,
    [activeSteps, activeTasks, activeWorkflowId, snapshots, taskId, workflows],
  );

  if (!view) return null;
  const hasIdentity = view.workflowName !== null && view.destinationTitle !== null;
  const hasPosition = view.position !== null && view.total !== null;
  const hasAdmittedCount = view.admittedCount !== null;
  const hasLimit = view.wipLimit !== null && view.wipLimit > 0;

  return (
    <section
      data-testid="task-wip-queue-status"
      className="min-w-0 shrink-0 border-b border-border/70 bg-muted/10 px-3 py-2"
    >
      <div className="flex min-w-0 items-start gap-2">
        <IconListNumbers
          className="mt-0.5 size-4 shrink-0 text-blue-600 dark:text-blue-400"
          aria-hidden="true"
        />
        <div className="min-w-0 flex-1 space-y-0.5 text-xs">
          <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-0.5">
            <span className="font-medium text-foreground">{t("task:wipQueueTitle")}</span>
            <span className="rounded bg-blue-500/15 px-1.5 py-0.5 font-medium text-blue-700 dark:text-blue-300">
              {t("task:launchQueueLabel")}
            </span>
          </div>
          {hasIdentity ? (
            <p className="min-w-0 break-words text-foreground">
              {t("task:wipQueueDestination", {
                workflow: view.workflowName,
                step: view.destinationTitle,
              })}
            </p>
          ) : (
            <p className="min-w-0 break-words text-muted-foreground">
              {t("task:wipQueueUnavailable")}
            </p>
          )}
          {hasPosition && (
            <p className="min-w-0 break-words tabular-nums text-muted-foreground">
              {t("task:wipQueuePosition", { position: view.position, total: view.total })}
            </p>
          )}
          {hasAdmittedCount && (
            <p className="min-w-0 break-words tabular-nums text-muted-foreground">
              {hasLimit
                ? t("task:wipQueueAdmittedCount", {
                    count: view.admittedCount,
                    limit: view.wipLimit,
                  })
                : t("task:wipQueueAdmittedUnlimited", { count: view.admittedCount })}
            </p>
          )}
          {hasIdentity && view.settingsHref ? (
            <Link
              href={view.settingsHref}
              data-testid="wip-queue-settings-link"
              className={settingsActionClassName(
                "mt-1 inline-flex max-w-full whitespace-normal text-left leading-tight",
              )}
            >
              {t("task:wipQueueConfigure")}
            </Link>
          ) : (
            <p className="min-w-0 break-words text-muted-foreground">
              {t("task:wipQueueConfigurationUnavailable")}
            </p>
          )}
        </div>
      </div>
    </section>
  );
}
