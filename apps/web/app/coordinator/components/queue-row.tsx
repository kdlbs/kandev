import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { getPrimaryTaskPR } from "@/hooks/domains/github/use-task-pr";
import TaskLink from "@/components/routing/task-link";
import type { QueueItem } from "@/lib/coordinator/attention";
import { formatAge } from "@/lib/coordinator/format";
import { agentStateLabel, otherRowStatusText } from "@/lib/coordinator/queue-text";
import type { TaskPR } from "@/lib/types/github";
import { ReadyToMergeActions } from "./ready-to-merge-row";

export type QueueRowProps = {
  item: QueueItem;
  stepNameByTaskId: Map<string, string>;
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
  /** Phase 2: a Ready to merge row carries Open the PR and Send it back beside the task link. */
  phase2?: boolean;
  canManage?: boolean;
};

function PullRequestStatus({
  item,
  prsByTaskId,
}: {
  item: QueueItem;
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
}) {
  const { t } = useTranslation();
  const pr = getPrimaryTaskPR(prsByTaskId.get(item.task.id));

  if (!pr) {
    const state = item.task.statusSummary?.pull_request?.state;
    return (
      <span className="text-muted-foreground flex items-center gap-1 text-xs">
        {state && <span>{state}</span>}
        <span>{t("coordinator:prDetailUnavailable")}</span>
      </span>
    );
  }

  return (
    <span className="text-muted-foreground flex items-center gap-1 text-xs">
      <span>{pr.state}</span>
      <span>{t("coordinator:unresolvedThreads", { count: pr.unresolved_review_threads })}</span>
      <span>{pr.checks_state}</span>
    </span>
  );
}

/**
 * A Queue row's status column, per group: Working shows the agent state
 * (AC-004.3), In review/Ready to merge show the PR detail (AC-004.4), Other
 * shows its exception text when applicable, Done shows nothing extra
 * (neither is required by the frozen ACs).
 */
function QueueRowStatus({
  item,
  prsByTaskId,
}: {
  item: QueueItem;
  prsByTaskId: ReadonlyMap<string, TaskPR[]>;
}) {
  const { t } = useTranslation();
  if (item.group === "working") {
    return (
      <span className="text-muted-foreground text-xs">
        {agentStateLabel(t, item.task.statusSummary?.primary_session?.state)}
      </span>
    );
  }
  if (item.group === "in_review" || item.group === "ready_to_merge") {
    return <PullRequestStatus item={item} prsByTaskId={prsByTaskId} />;
  }
  const otherText = otherRowStatusText(t, item, item.task);
  return otherText ? <span className="text-muted-foreground text-xs">{otherText}</span> : null;
}

/**
 * One Queue row: card identifier, step, group-specific status, last
 * activity. Opening the task is its only action (AC-COORDINATOR-NEEDS-YOU-004.5),
 * except a Ready to merge row with phase 2 on, which adds row actions beside the link.
 */
export function QueueRow({
  item,
  stepNameByTaskId,
  prsByTaskId,
  phase2 = false,
  canManage = false,
}: QueueRowProps) {
  const stepName = stepNameByTaskId.get(item.task.id);
  const withActions = phase2 && item.group === "ready_to_merge";
  const link = (
    <TaskLink
      taskId={item.task.id}
      className={cn(
        "flex flex-wrap items-center gap-2 rounded-md p-2 hover:bg-accent",
        withActions && "min-w-0 flex-1",
      )}
      data-testid={`queue-row-${item.task.id}`}
    >
      <span className="font-medium">{item.task.identifier ?? item.task.title}</span>
      {stepName && <span className="text-muted-foreground text-xs">{stepName}</span>}
      <QueueRowStatus item={item} prsByTaskId={prsByTaskId} />
      <span className="text-muted-foreground ml-auto text-xs">{formatAge(item.ageMs)}</span>
    </TaskLink>
  );
  if (withActions) {
    return (
      <ReadyToMergeActions item={item} prsByTaskId={prsByTaskId} canManage={canManage}>
        {(actions) => (
          <div className="flex flex-wrap items-center gap-2">
            {link}
            <div className="p-2">{actions}</div>
          </div>
        )}
      </ReadyToMergeActions>
    );
  }
  return link;
}
