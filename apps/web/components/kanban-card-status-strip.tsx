"use client";

import { useTranslation } from "react-i18next";
import { IconAlertCircle, IconLock, IconSubtask } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTaskById } from "@/hooks/domains/kanban/use-task-by-id";
import { cn } from "@/lib/utils";
import type { RepositoryChip, Task } from "@/components/kanban-card";

const REPO_CHIPS_VISIBLE = 2;

function RepoChip({ chip }: { chip: RepositoryChip }) {
  const badge = (
    <span
      data-testid="task-repo-chip"
      title={chip.path}
      className="shrink-0 rounded-sm bg-muted/60 px-1 py-px text-[9px] font-medium text-muted-foreground leading-tight max-w-[8rem] truncate"
    >
      {chip.label}
    </span>
  );
  if (!chip.path) return badge;
  return (
    <Tooltip>
      <TooltipTrigger asChild>{badge}</TooltipTrigger>
      <TooltipContent side="top" align="start">
        <span className="max-w-[22rem] break-all text-xs">{chip.path}</span>
      </TooltipContent>
    </Tooltip>
  );
}

function OverflowRepoTooltip({ chips }: { chips: RepositoryChip[] }) {
  return (
    <div className="flex max-w-[24rem] flex-col gap-1 text-xs">
      {chips.map((chip) => (
        <div key={`${chip.label}:${chip.path ?? ""}`} className="min-w-0">
          <div className="font-medium">{chip.label}</div>
          {chip.path && <div className="break-all text-muted-foreground">{chip.path}</div>}
        </div>
      ))}
    </div>
  );
}

export function RepoChipRow({ chips }: { chips: RepositoryChip[] }) {
  if (chips.length === 0) return null;
  const visible = chips.slice(0, REPO_CHIPS_VISIBLE);
  const overflow = chips.slice(REPO_CHIPS_VISIBLE);
  return (
    <div className="mb-1 flex items-center gap-1 min-w-0 overflow-hidden">
      {visible.map((chip) => (
        <RepoChip key={`${chip.label}:${chip.path ?? ""}`} chip={chip} />
      ))}
      {overflow.length > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <span className="shrink-0 rounded-sm bg-muted px-1 py-px text-[9px] font-medium text-muted-foreground/80">
              +{overflow.length}
            </span>
          </TooltipTrigger>
          <TooltipContent side="top" align="start">
            <OverflowRepoTooltip chips={overflow} />
          </TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

export function KanbanCardRelationship({ task }: { task: Task }) {
  const { t } = useTranslation();
  const parentTitle = useTaskById(task.parentTaskId)?.title ?? null;

  if (!task.parentTaskId) return null;
  // Matches ParentSection's fallback (task-title-hover-card.tsx) so the two
  // "show the parent relationship" surfaces don't diverge when the parent
  // title isn't resolvable from the store.
  const relationshipTitle = parentTitle ?? t("task:subtask");

  return (
    <div
      data-testid="task-parent-relationship"
      title={relationshipTitle}
      className="mt-1 flex min-w-0 items-center gap-1 text-[10px] text-muted-foreground"
    >
      <IconSubtask className="h-3 w-3 shrink-0" />
      <span className="shrink-0 font-medium">{t("kanban:subtaskOf")}</span>
      <span className="min-w-0 truncate">{relationshipTitle}</span>
    </div>
  );
}

/**
 * Blocked badge — the card-level signal that this task will not start on its
 * own. Distinguishes a failed predecessor (chain halted, needs a human) from
 * merely pending ones, because those need different actions from the user.
 *
 * The predecessor list is on the payload already, so the title needs no fetch.
 * The count is rendered as text rather than hover-only so the state is readable
 * on a touch device.
 */
function BlockedBadge({ task }: { task: Task }) {
  const { t } = useTranslation();
  const count = task.dependsOn?.length ?? 0;
  const failed = task.blockedReason === "failed";
  const names = (task.dependsOn ?? []).map((ref) => ref.title || ref.id).join(", ");
  return (
    <Badge
      variant="outline"
      className={cn(
        // Same pill formula as the dependency chip above the composer: rounded
        // outline, 10% tint, 35% border, colour as the text. Keeps the two
        // surfaces for one concept looking like one thing.
        "h-5 gap-1 rounded-full px-2 text-xs font-medium leading-none",
        failed
          ? "border-red-500/40 bg-red-500/10 text-red-600 dark:text-red-400"
          : "border-primary/35 bg-primary/10 text-primary",
      )}
      title={
        failed
          ? t("kanban:blockedPredecessorFailed", { tasks: names })
          : t("kanban:blockedByTasksTitle", { tasks: names })
      }
      data-testid="kanban-card-blocked-badge"
    >
      <IconLock className="h-3 w-3" />
      {failed ? t("kanban:blockedFailed") : t("kanban:blockedByCount", { count })}
    </Badge>
  );
}

function hasCardBadges(task: Task): boolean {
  return Boolean(
    (task.sessionCount && task.sessionCount > 1) ||
    task.reviewStatus === "changes_requested" ||
    task.reviewStatus === "pending" ||
    task.queuedForStepId ||
    task.blocked,
  );
}

export function KanbanCardBadges({ task }: { task: Task }) {
  const { t } = useTranslation();
  const showRow = hasCardBadges(task);

  if (!showRow) return null;

  return (
    <div className="flex flex-wrap items-center justify-end gap-2 mt-1 min-w-0">
      {task.blocked && <BlockedBadge task={task} />}
      {task.queuedForStepId && (
        <Badge
          variant="secondary"
          className="text-xs h-5"
          title={t("kanban:queuedForStep", {
            step:
              task.queuedForStepTitle ??
              t("kanban:workflowStepFallback", { stepId: task.queuedForStepId }),
          })}
        >
          {t("kanban:queuedForStep", {
            step: task.queuedForStepTitle ?? t("kanban:nextCapacity"),
          })}
        </Badge>
      )}
      {task.sessionCount && task.sessionCount > 1 && (
        <Badge variant="secondary" className="text-xs h-5">
          {t("kanban:sessionCount", { count: task.sessionCount })}
        </Badge>
      )}
      {task.reviewStatus === "pending" && task.state !== "IN_PROGRESS" && (
        <div className="flex items-center gap-1 text-amber-700 dark:text-amber-600">
          <IconAlertCircle className="h-3.5 w-3.5" />
          <span className="text-[10px] font-medium">{t("kanban:approvalRequired")}</span>
        </div>
      )}
      {task.reviewStatus === "changes_requested" && (
        <Badge
          variant="outline"
          className="border-amber-500 text-amber-600 bg-amber-50 dark:bg-amber-950/50 text-xs h-5"
        >
          {t("kanban:changesRequested")}
        </Badge>
      )}
    </div>
  );
}
