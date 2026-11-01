import type { TasksListArchiveMode } from "@/lib/tasks/tasks-list-query";
import type { PaginationState } from "@tanstack/react-table";
import type { ReactNode } from "react";
import type { Repository, Task } from "@/lib/types/http";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import type { TaskListStepPreviews, TaskListWorkflow } from "@/lib/tasks/task-list-sections";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import { TasksListView, type TasksListReadFeedbackProps } from "./tasks-list-view";
import type { TasksListFilterProps } from "./tasks-list-filters";
import type { DeleteTaskParams } from "@/lib/api/domains/kanban-api";

type Props = TasksListReadFeedbackProps & {
  filters: TasksListFilterProps;
  header: React.ComponentProps<typeof KanbanHeader>;
  isMobile: boolean;
  isMobileSearchOpen: boolean;
  tasks: Task[];
  workflows: TaskListWorkflow[];
  repositories: Repository[];
  facetOptions: ReadonlyArray<{ value: string; label: string }>;
  facetValues: Record<string, readonly TaskListFacetValue[]>;
  workflowStepPreviews: TaskListStepPreviews;
  total: number;
  pageCount: number;
  pagination: PaginationState;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  isLoading: boolean;
  archiveMode: TasksListArchiveMode;
  showTaskDetails: boolean;
  sort: string;
  group: string;
  onSortChange: (value: string) => void;
  onGroupChange: (value: string) => void;
  onArchiveModeChange: (mode: TasksListArchiveMode) => void;
  onRowClick: (task: Task) => void;
  deletingTaskId: string | null;
  onArchive: (id: string, opts?: { cascade?: boolean }) => Promise<void>;
  onUnarchive: (id: string) => Promise<void>;
  onDelete: (id: string, opts?: DeleteTaskParams) => Promise<void>;
  onRefresh: () => Promise<void>;
  mobileActions: ReactNode;
};

export function TasksPageContent(props: Props) {
  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <KanbanHeader {...props.header} />
      <TasksListView
        filters={props.filters}
        readError={props.readError}
        unavailableFilter={props.unavailableFilter}
        onRetry={props.onRetry}
        archiveMode={props.archiveMode}
        setArchiveMode={props.onArchiveModeChange}
        tasksListSort={props.sort}
        onTasksListSortChange={props.onSortChange}
        tasksListGroup={props.group}
        onTasksListGroupChange={props.onGroupChange}
        facetOptions={props.facetOptions}
        facetValues={props.facetValues}
        workflowStepPreviews={props.workflowStepPreviews}
        tasks={props.tasks}
        workflows={props.workflows}
        repositories={props.repositories}
        showTaskDetails={props.showTaskDetails}
        total={props.total}
        pageCount={props.pageCount}
        pagination={props.pagination}
        setPagination={props.setPagination}
        isLoading={props.isLoading}
        handleRowClick={props.onRowClick}
        deletingTaskId={props.deletingTaskId}
        handleArchive={props.onArchive}
        handleUnarchive={props.onUnarchive}
        handleDelete={props.onDelete}
        onRefresh={props.isMobile ? props.onRefresh : undefined}
      />
      {props.mobileActions}
    </div>
  );
}
