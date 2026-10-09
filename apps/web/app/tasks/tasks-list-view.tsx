"use client";

import { useMemo, useRef, useState } from "react";
import type { PaginationState } from "@tanstack/react-table";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { IconArchive, IconArchiveOff, IconLoader, IconTrash } from "@tabler/icons-react";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
import { cleanupSharesParentWorkspace } from "@/components/task/task-cleanup-summary";
import { TaskDeleteConfirmDialog } from "@/components/task/task-delete-confirm-dialog";
import { type Repository, type Task } from "@/lib/types/http";
import { isTaskInFlight } from "@/lib/ui/state-icons";
import { formatRelativeTime } from "@/lib/utils";
import { TasksPagination } from "./tasks-pagination";
import { TaskListRowPrimaryContent } from "./rich-task-list-row";
import { PullToRefresh } from "@/components/mobile/pull-to-refresh";
import { TasksListControls } from "./tasks-list-controls";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import { workspaceModeFromMetadata } from "@/lib/kanban/map-task";

import {
  buildTaskSections,
  type TaskTreeNode,
  type TaskListSection,
  type TaskListStepPreviews,
  type TaskListWorkflow,
} from "@/lib/tasks/task-list-sections";

export type TasksListViewProps = {
  total: number;
  showArchived: boolean;
  setShowArchived: (show: boolean) => void;
  tasksListSort: string;
  onTasksListSortChange: (sort: string) => void;
  tasksListGroup: string;
  onTasksListGroupChange: (group: string) => void;
  facetOptions?: ReadonlyArray<{ value: string; label: string }>;
  facetValues?: Record<string, readonly TaskListFacetValue[]>;
  workflowStepPreviews?: TaskListStepPreviews;
  tasks: Task[];
  workflows: TaskListWorkflow[];
  repositories: Repository[];
  showTaskDetails: boolean;
  pageCount: number;
  pagination: PaginationState;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  isLoading: boolean;
  handleRowClick: (task: Task) => void;
  deletingTaskId: string | null;
  handleArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  handleUnarchive: (taskId: string) => Promise<void>;
  handleDelete: (
    taskId: string,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean },
  ) => Promise<void>;
  onRefresh?: () => void | Promise<void>;
};

export function TasksListView({
  total,
  showArchived,
  setShowArchived,
  tasksListSort,
  onTasksListSortChange,
  tasksListGroup,
  onTasksListGroupChange,
  tasks,
  workflows,
  repositories,
  showTaskDetails,
  pageCount,
  pagination,
  setPagination,
  isLoading,
  handleRowClick,
  deletingTaskId,
  handleArchive,
  handleUnarchive,
  handleDelete,
  onRefresh,
  facetOptions,
  facetValues,
  workflowStepPreviews,
}: TasksListViewProps) {
  // Not a <main>: AppShell owns that landmark, one per page.
  const content = (
    <div className="flex-1 overflow-auto px-4 py-4 sm:px-6 sm:py-6">
      <div className="space-y-4">
        <TasksListControls
          showArchived={showArchived}
          onShowArchivedChange={setShowArchived}
          tasksListSort={tasksListSort}
          onTasksListSortChange={onTasksListSortChange}
          tasksListGroup={tasksListGroup}
          onTasksListGroupChange={onTasksListGroupChange}
          facetOptions={facetOptions}
        />
        <TaskRows
          tasks={tasks}
          workflows={workflows}
          repositories={repositories}
          showTaskDetails={showTaskDetails}
          tasksListGroup={tasksListGroup}
          isLoading={isLoading}
          deletingTaskId={deletingTaskId}
          onArchive={handleArchive}
          onUnarchive={handleUnarchive}
          onDelete={handleDelete}
          onRowClick={handleRowClick}
          facetValues={facetValues}
          workflowStepPreviews={workflowStepPreviews}
        />
        <TasksPagination
          total={total}
          pageCount={pageCount}
          pagination={pagination}
          onPaginationChange={setPagination}
        />
      </div>
    </div>
  );
  return onRefresh ? <PullToRefresh onRefresh={onRefresh}>{content}</PullToRefresh> : content;
}

function TaskRows({
  tasks,
  workflows,
  repositories,
  showTaskDetails,
  tasksListGroup,
  isLoading,
  deletingTaskId,
  onArchive,
  onUnarchive,
  onDelete,
  onRowClick,
  facetValues = {},
  workflowStepPreviews = {},
}: {
  tasks: Task[];
  workflows: TaskListWorkflow[];
  repositories: Repository[];
  showTaskDetails: boolean;
  tasksListGroup: string;
  isLoading: boolean;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onUnarchive: (taskId: string) => Promise<void>;
  onDelete: (
    taskId: string,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean; confirmationId?: string },
  ) => Promise<void>;
  onRowClick: (task: Task) => void;
  facetValues?: Record<string, readonly TaskListFacetValue[]>;
  workflowStepPreviews?: TaskListStepPreviews;
}) {
  const { t, i18n } = useTranslation();
  const workflowMap = useMemo(() => new Map(workflows.map((w) => [w.id, w.name])), [workflows]);
  const repoMap = useMemo(() => new Map(repositories.map((r) => [r.id, r.name])), [repositories]);
  // Section headings resolve from the active locale even when the tasks do not change.
  const sections = useMemo(
    () =>
      buildTaskSections(tasks, {
        groupBy: tasksListGroup,
        workflowMap,
        repoMap,
        facetValues,
        workflows,
        workflowStepPreviews,
      }),
    [
      facetValues,
      repoMap,
      tasks,
      tasksListGroup,
      workflowMap,
      workflows,
      workflowStepPreviews,
      i18n.language,
    ],
  );

  if (isLoading) {
    return (
      <div className="rounded-lg border border-border p-8 text-center text-sm text-muted-foreground">
        {t("tasks:loadingTasks")}
      </div>
    );
  }
  if (tasks.length === 0) {
    return (
      <div className="rounded-lg border border-border p-8 text-center text-sm text-muted-foreground">
        {t("tasks:noTasksFound")}
      </div>
    );
  }

  return (
    <div className="space-y-5" data-testid="tasks-list">
      {sections.map((section) => (
        <TaskListSectionView
          key={section.key}
          section={section}
          deletingTaskId={deletingTaskId}
          onArchive={onArchive}
          onUnarchive={onUnarchive}
          onDelete={onDelete}
          onRowClick={onRowClick}
          repositories={repositories}
          parentTasks={tasks}
          showTaskDetails={showTaskDetails}
        />
      ))}
    </div>
  );
}

function flattenTaskTree(nodes: TaskTreeNode[]): TaskTreeNode[] {
  return nodes.flatMap((node) => [node, ...flattenTaskTree(node.children)]);
}

function TaskListRow({
  task,
  level,
  repositories,
  parentTasks,
  showTaskDetails,
  deletingTaskId,
  onArchive,
  onUnarchive,
  onDelete,
  onRowClick,
}: {
  task: Task;
  level: number;
  repositories: Repository[];
  parentTasks: Task[];
  showTaskDetails: boolean;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onUnarchive: (taskId: string) => Promise<void>;
  onDelete: (
    taskId: string,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean; confirmationId?: string },
  ) => Promise<void>;
  onRowClick: (task: Task) => void;
}) {
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [showArchiveConfirm, setShowArchiveConfirm] = useState(false);
  const isDeleting = deletingTaskId === task.id;
  const isArchived = !!task.archived_at;

  return (
    <div
      role="button"
      tabIndex={0}
      data-testid="tasks-list-row"
      data-level={level}
      className="grid min-h-12 grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 py-2 text-sm transition-colors hover:bg-muted/60 cursor-pointer"
      onClick={() => onRowClick(task)}
      onKeyDown={(event) => {
        if (event.target !== event.currentTarget) return;
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onRowClick(task);
        }
      }}
    >
      <TaskListRowPrimaryContent
        task={task}
        level={level}
        repositories={repositories}
        parentTasks={parentTasks}
        showTaskDetails={showTaskDetails}
      />
      <div className="flex items-center justify-between gap-3 md:justify-end">
        <span className="hidden text-xs text-muted-foreground sm:inline">
          {formatRelativeTime(task.updated_at)}
        </span>
        <TaskRowActions
          task={task}
          isArchived={isArchived}
          isDeleting={isDeleting}
          showDeleteConfirm={showDeleteConfirm}
          showArchiveConfirm={showArchiveConfirm}
          onDeleteOpenChange={setShowDeleteConfirm}
          onArchiveOpenChange={setShowArchiveConfirm}
          onArchive={onArchive}
          onUnarchive={onUnarchive}
          onDelete={onDelete}
        />
      </div>
    </div>
  );
}

function TaskListSectionView({
  section,
  repositories,
  parentTasks,
  showTaskDetails,
  deletingTaskId,
  onArchive,
  onUnarchive,
  onDelete,
  onRowClick,
}: {
  section: TaskListSection;
  repositories: Repository[];
  parentTasks: Task[];
  showTaskDetails: boolean;
  deletingTaskId: string | null;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onUnarchive: (taskId: string) => Promise<void>;
  onDelete: (
    taskId: string,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean; confirmationId?: string },
  ) => Promise<void>;
  onRowClick: (task: Task) => void;
}) {
  const rows = flattenTaskTree(section.nodes);
  return (
    <section className="space-y-2" data-testid="tasks-list-section">
      {section.title && (
        <div className="flex items-center gap-2 px-1 text-xs font-semibold uppercase tracking-normal text-muted-foreground">
          {section.color && (
            <span
              className="h-2 w-2 shrink-0 rounded-full"
              style={{ backgroundColor: section.color }}
            />
          )}
          <span>{section.title}</span>
          <span className="text-muted-foreground/70">{rows.length}</span>
        </div>
      )}
      <div className="rounded-lg border border-border divide-y divide-border overflow-hidden">
        {rows.map(({ task, level }) => (
          <TaskListRow
            key={task.id}
            task={task}
            level={level}
            repositories={repositories}
            parentTasks={parentTasks}
            showTaskDetails={showTaskDetails}
            deletingTaskId={deletingTaskId}
            onArchive={onArchive}
            onUnarchive={onUnarchive}
            onDelete={onDelete}
            onRowClick={onRowClick}
          />
        ))}
      </div>
    </section>
  );
}

// Holds its own in-flight state so rapid clicks can't fire duplicate
// unarchive POSTs (each producing a toast + refetch).
function UnarchiveRowAction({
  taskId,
  onUnarchive,
}: {
  taskId: string;
  onUnarchive: (taskId: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [isPending, setIsPending] = useState(false);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={isPending ? 0 : -1} className="inline-flex">
          <Button
            variant="ghost"
            size="icon"
            className="cursor-pointer"
            data-testid="tasks-list-unarchive"
            disabled={isPending}
            onClick={async () => {
              setIsPending(true);
              try {
                await onUnarchive(taskId);
              } finally {
                setIsPending(false);
              }
            }}
          >
            {isPending ? (
              <IconLoader className="h-4 w-4 animate-spin" />
            ) : (
              <IconArchiveOff className="h-4 w-4 text-muted-foreground" />
            )}
            <span className="sr-only">{t("tasks:unarchiveTask")}</span>
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{t("tasks:unarchive")}</TooltipContent>
    </Tooltip>
  );
}

function TaskRowActions({
  task,
  isArchived,
  isDeleting,
  showDeleteConfirm,
  showArchiveConfirm,
  onDeleteOpenChange,
  onArchiveOpenChange,
  onArchive,
  onUnarchive,
  onDelete,
}: {
  task: Task;
  isArchived: boolean;
  isDeleting: boolean;
  showDeleteConfirm: boolean;
  showArchiveConfirm: boolean;
  onDeleteOpenChange: (open: boolean) => void;
  onArchiveOpenChange: (open: boolean) => void;
  onArchive: (taskId: string, opts?: { cascade?: boolean }) => Promise<void>;
  onUnarchive: (taskId: string) => Promise<void>;
  onDelete: (
    taskId: string,
    opts?: { cascade?: boolean; discardWorktreeChanges?: boolean; confirmationId?: string },
  ) => Promise<void>;
}) {
  const { t } = useTranslation();
  const workspaceMode = workspaceModeFromMetadata(task.metadata);
  const archiveAnchorRef = useRef<HTMLButtonElement>(null);
  const { isFinePointer } = useResponsiveBreakpoint();
  return (
    <div
      className={`flex items-center gap-1 ${showArchiveConfirm && !isFinePointer ? "flex-wrap" : ""}`}
      onClick={(event) => event.stopPropagation()}
    >
      {!isArchived && (!showArchiveConfirm || isFinePointer) && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              ref={archiveAnchorRef}
              variant="ghost"
              size="icon"
              className="cursor-pointer"
              onClick={() => onArchiveOpenChange(true)}
            >
              <IconArchive className="h-4 w-4 text-muted-foreground" />
              <span className="sr-only">{t("tasks:archiveTask")}</span>
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("tasks:archive")}</TooltipContent>
        </Tooltip>
      )}
      {isArchived && <UnarchiveRowAction taskId={task.id} onUnarchive={onUnarchive} />}
      <Tooltip>
        <TooltipTrigger asChild>
          <span tabIndex={isDeleting ? 0 : -1} className="inline-flex">
            <Button
              variant="ghost"
              size="icon"
              className="cursor-pointer"
              disabled={isDeleting}
              onClick={() => onDeleteOpenChange(true)}
            >
              {isDeleting ? (
                <IconLoader className="h-4 w-4 animate-spin" />
              ) : (
                <IconTrash className="h-4 w-4 text-destructive" />
              )}
              <span className="sr-only">{t("tasks:deleteTask")}</span>
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>{t("tasks:delete")}</TooltipContent>
      </Tooltip>
      <TaskDeleteConfirmDialog
        open={showDeleteConfirm}
        onOpenChange={onDeleteOpenChange}
        taskTitle={task.title}
        taskId={task.id}
        isInFlight={isTaskInFlight(task.foreground_activity)}
        executorType={task.primary_executor_type}
        sharesParentWorkspace={cleanupSharesParentWorkspace(workspaceMode)}
        isDeleting={isDeleting}
        onConfirm={({ cascade, discardWorktreeChanges, confirmationId }) =>
          onDelete(task.id, { cascade, discardWorktreeChanges, confirmationId })
        }
      />
      <TaskArchiveConfirmation
        open={showArchiveConfirm}
        anchorRef={archiveAnchorRef}
        onOpenChange={onArchiveOpenChange}
        taskTitle={task.title}
        taskId={task.id}
        isInFlight={isTaskInFlight(task.foreground_activity)}
        executorType={task.primary_executor_type}
        onConfirm={({ cascade }) => onArchive(task.id, { cascade })}
      />
    </div>
  );
}
