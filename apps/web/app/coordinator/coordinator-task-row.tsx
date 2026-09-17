import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { statusSummaryActiveErrorPreview } from "@/lib/task-status-summary";
import Link from "@/components/routing/app-link";
import { TaskStateIcon } from "@/components/task/task-state-icon";
import { Badge } from "@kandev/ui/badge";
import { formatRelativeTime } from "@/lib/utils";
import type { Task } from "@/lib/types/http";
import type { CoordinatorWorkspace } from "@/hooks/domains/orchestration/use-coordinator-workspace";
import type { CoordinatorTaskGroup } from "@/lib/orchestration/coordinator-task-groups";

function TaskEvidence({ task }: { task: Task }) {
  const { t } = useTranslation();
  const summary = task.status_summary;
  const pr = summary?.pull_request;
  const href = safeExternalPR(pr?.url);
  return (
    <div className="flex flex-wrap items-center gap-3 px-3 pb-3 text-xs text-muted-foreground">
      <TaskNotices task={task} />
      {!summary && <span>{t("orchestration:statusUnavailable")}</span>}
      {summary?.last_activity_at && (
        <time dateTime={summary.last_activity_at}>
          {t("orchestration:lastActivity", { time: formatRelativeTime(summary.last_activity_at) })}
        </time>
      )}
      {href && (
        <a
          href={href}
          target="_blank"
          rel="noreferrer"
          className="cursor-pointer underline max-md:min-h-11 inline-flex items-center"
        >
          {t("orchestration:pullRequest", { number: pr?.number })}
        </a>
      )}
      {pr?.state && <span>{pr.state}</span>}
      {summary?.git?.changed_files !== undefined && !summary.git.comparison_unavailable && (
        <span>{t("orchestration:changedFiles", { count: summary.git.changed_files })}</span>
      )}
      {task.parked_on_background_work && <span>{t("task:backgroundWorkIsRunning")}</span>}
    </div>
  );
}

function TaskNotices({ task }: { task: Task }) {
  const { t } = useTranslation();
  const summary = task.status_summary;
  const acknowledged = useAppStore((s) => s.acknowledgedAgentErrors);
  const dismissed = useAppStore((s) => s.dismissedAgentErrors);
  const error = statusSummaryActiveErrorPreview(summary, acknowledged, dismissed);
  const pending = summary ? summary.pending_action : task.task_pending_action;
  return (
    <>
      {pending && (
        <Badge variant="outline">
          {t(
            pending === "permission"
              ? "orchestration:needsPermission"
              : "orchestration:needsClarification",
          )}
        </Badge>
      )}
      {error && <p className="text-destructive break-words w-full">{error}</p>}
    </>
  );
}

export function CoordinatorTaskRow({
  task,
  group,
  catalog,
}: {
  task: Task;
  group: CoordinatorTaskGroup;
  catalog: CoordinatorWorkspace;
}) {
  const { t } = useTranslation();
  const summary = task.status_summary;
  const pending = summary ? summary.pending_action : task.task_pending_action;
  const workflow = catalog.workflows.find((item) => item.id === task.workflow_id)?.name;
  const step = catalog.steps.find((item) => item.id === task.workflow_step_id)?.name;
  const repositories = (task.repositories ?? [])
    .map((repo) => catalog.repositories.find((item) => item.id === repo.repository_id)?.name)
    .filter(Boolean);
  return (
    <article
      data-testid={`coordinator-task-${task.id}`}
      className="rounded-lg border bg-card shadow-sm"
    >
      <Link
        href={`/t/${encodeURIComponent(task.id)}`}
        className="cursor-pointer block rounded-t-lg p-3 hover:bg-muted/50 focus-visible:outline-ring"
      >
        <div className="flex items-start gap-2">
          <TaskStateIcon
            state={task.state}
            sessionState={
              summary ? summary.primary_session?.state : (task.primary_session_state ?? undefined)
            }
            foregroundActivity={summary ? summary.foreground_activity : task.foreground_activity}
            hasPendingClarification={pending === "clarification"}
            hasPendingPermission={pending === "permission"}
            interrupted={task.interrupted}
            parkedOnBackgroundWork={task.parked_on_background_work}
            accessibleLabel={t(`orchestration:group_${group}`)}
          />
          <span className="min-w-0 flex-1 break-words font-medium">{task.title}</span>
          <Badge variant="secondary" className="shrink-0 text-xs">
            {task.state}
          </Badge>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {task.identifier && <span>{task.identifier}</span>}
          <span>{workflow}</span>
          <span>{step}</span>
          <span>{repositories.join(", ")}</span>
          <span>{task.primary_agent_name}</span>
        </div>
      </Link>
      <TaskEvidence task={task} />
    </article>
  );
}

function safeExternalPR(value?: string) {
  return value && /^https?:\/\//i.test(value) ? value : undefined;
}
