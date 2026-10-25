import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";

export type TasksListReadFeedbackProps = {
  readError?: boolean;
  unavailableFilter?: string | null;
  onRetry?: () => void;
};

export function TaskListResultAnnouncement({
  busy,
  issue,
  total,
}: {
  busy: boolean;
  issue: boolean;
  total: number;
}) {
  const { t } = useTranslation();
  if (busy || issue || total === 0) return null;
  return (
    <span className="sr-only" role="status">
      {t("tasks:filteredCount", { count: total })}
    </span>
  );
}

export function TaskListResultBoundary({
  readError,
  unavailableFilter,
  onRetry,
  onClearFilters,
  children,
}: TasksListReadFeedbackProps & { onClearFilters?: () => void; children: ReactNode }) {
  const { t } = useTranslation();
  if (!readError && !unavailableFilter) return children;
  let message = t("tasks:failedToLoadTasks");
  if (unavailableFilter)
    message = t(
      unavailableFilter === "workflow"
        ? "tasks:workflowUnavailable"
        : "tasks:repositoryUnavailable",
    );
  return (
    <div
      role="alert"
      className="rounded-lg border border-border p-8 text-center text-sm text-muted-foreground"
      data-testid="tasks-list-error"
    >
      <p>{message}</p>
      <Button
        variant="outline"
        className="mt-3 cursor-pointer"
        onClick={unavailableFilter ? onClearFilters : onRetry}
      >
        {unavailableFilter ? t("tasks:clearFilters") : t("task:retry")}
      </Button>
    </div>
  );
}
