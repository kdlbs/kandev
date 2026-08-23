"use client";

import { useRef, useState } from "react";
import type { Row, ColumnDef } from "@tanstack/react-table";
import type { Task, Workflow, WorkflowStep, Repository } from "@/lib/types/http";
import Link from "@/components/routing/app-link";
import { IconTrash, IconLoader, IconArchive } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { formatTimeDistance, useDateLocale } from "@/lib/i18n/date-locale";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { linkToTask } from "@/lib/links";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type TaskWithResolution = Task & {
  workflowName?: string;
  stepName?: string;
  repositoryNames?: string[];
};

interface ColumnsConfig {
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => void;
  onDelete: (taskId: string, opts?: { cascade?: boolean }) => void;
  deletingTaskId: string | null;
}

function TitleCell({
  row,
  repoMap,
}: {
  row: Row<TaskWithResolution>;
  repoMap: Map<string, string>;
}) {
  const { t } = useTranslation();
  const task = row.original;
  const isArchived = !!task.archived_at;
  const repoName = task.repositories?.[0]
    ? repoMap.get(task.repositories[0].repository_id)
    : undefined;
  return (
    <div className="flex flex-col gap-0.5 py-0.5">
      <div className="flex items-center gap-2">
        <Link href={linkToTask(task.id)} className="text-primary font-medium text-sm">
          {task.title}
        </Link>
        {isArchived && (
          <Badge
            variant="outline"
            className="text-[10px] px-1.5 py-0 text-amber-500 border-amber-500/30"
          >
            {t("tasks:archived")}
          </Badge>
        )}
      </div>
      {repoName && <span className="text-xs text-muted-foreground/60">{repoName}</span>}
    </div>
  );
}

type ActionsCtx = {
  onArchive: (id: string, opts?: { cascade?: boolean }) => void;
  onDelete: (id: string, opts?: { cascade?: boolean }) => void;
  deletingTaskId: string | null;
};

function ActionsCell({ row, ctx }: { row: Row<TaskWithResolution>; ctx: ActionsCtx }) {
  const { t } = useTranslation();
  const task = row.original;
  const isDeleting = ctx.deletingTaskId === task.id;
  const isArchived = !!task.archived_at;
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const archiveAnchorRef = useRef<HTMLButtonElement>(null);
  const { isFinePointer } = useResponsiveBreakpoint();
  return (
    <div
      className={`flex items-center justify-end gap-0.5 ${showArchiveConfirm && !isFinePointer ? "flex-wrap" : ""}`}
    >
      {!isArchived && (!showArchiveConfirm || isFinePointer) && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              ref={archiveAnchorRef}
              variant="ghost"
              size="sm"
              className="cursor-pointer h-7 w-7 p-0"
              onClick={(e) => {
                e.stopPropagation();
                setShowArchiveConfirm(true);
              }}
            >
              <IconArchive className="h-3.5 w-3.5 text-muted-foreground" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("tasks:archive")}</TooltipContent>
        </Tooltip>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="cursor-pointer h-7 w-7 p-0"
            disabled={isDeleting}
            onClick={(e) => {
              e.stopPropagation();
              setShowDeleteConfirm(true);
            }}
          >
            {isDeleting ? (
              <IconLoader className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <IconTrash className="h-3.5 w-3.5 text-destructive" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("tasks:delete")}</TooltipContent>
      </Tooltip>
      <TaskDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={setShowDeleteConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primary_executor_type}
        isDeleting={isDeleting}
        onConfirm={({ cascade }) => ctx.onDelete(task.id, { cascade })}
      />
      <TaskArchiveConfirmation
        open={showArchiveConfirm}
        anchorRef={archiveAnchorRef}
        onOpenChange={setShowArchiveConfirm}
        taskTitle={task.title}
        taskId={task.id}
        executorType={task.primary_executor_type}
        onConfirm={({ cascade }) => ctx.onArchive(task.id, { cascade })}
      />
    </div>
  );
}

// A component rather than an inline cell expression so it can subscribe to the
// date locale: `getColumns` is a plain builder and cannot call hooks, so an
// inline cell would keep the `enUS` fallback until some unrelated render.
function UpdatedAtCell({ updatedAt }: { updatedAt?: string }) {
  return (
    <span className="text-xs text-muted-foreground">
      {formatTimeDistance(updatedAt, useDateLocale())}
    </span>
  );
}

export function getColumns({
  workflows,
  steps,
  repositories,
  onArchive,
  onDelete,
  deletingTaskId,
}: ColumnsConfig): ColumnDef<TaskWithResolution>[] {
  const workflowMap = new Map(workflows.map((w) => [w.id, w.name]));
  const stepMap = new Map(steps.map((s) => [s.id, s.name]));
  const repoMap = new Map(repositories.map((r) => [r.id, r.name]));
  const actionsCtx: ActionsCtx = { onArchive, onDelete, deletingTaskId };

  return [
    {
      accessorKey: "title",
      header: t("tasks:columnTask"),
      cell: ({ row }) => <TitleCell row={row} repoMap={repoMap} />,
    },
    {
      accessorKey: "workflow_id",
      header: t("tasks:columnWorkflow"),
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">
          {workflowMap.get(row.original.workflow_id) || "-"}
        </span>
      ),
    },
    {
      accessorKey: "workflow_step_id",
      header: t("tasks:columnStep"),
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground bg-foreground/[0.06] px-2 py-0.5 rounded-md">
          {stepMap.get(row.original.workflow_step_id) || "-"}
        </span>
      ),
    },
    {
      accessorKey: "updated_at",
      header: t("tasks:columnUpdated"),
      cell: ({ row }) => <UpdatedAtCell updatedAt={row.original.updated_at} />,
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => <ActionsCell row={row} ctx={actionsCtx} />,
    },
  ];
}
