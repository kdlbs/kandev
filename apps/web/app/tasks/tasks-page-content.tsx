import type { PaginationState } from "@tanstack/react-table";
import type { ReactNode } from "react";
import type { Repository, Task, Workflow } from "@/lib/types/http";
import type { TaskListFacetValue } from "@/lib/plugins/types";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import { MobileSearchBar } from "@/components/kanban/mobile-search-bar";
import { TasksListView } from "./tasks-list-view";

type Props = {
  header: React.ComponentProps<typeof KanbanHeader>;
  isMobile: boolean;
  isMobileSearchOpen: boolean;
  tasks: Task[];
  workflows: Workflow[];
  repositories: Repository[];
  facetOptions: ReadonlyArray<{ value: string; label: string }>;
  facetValues: Record<string, readonly TaskListFacetValue[]>;
  total: number;
  pageCount: number;
  pagination: PaginationState;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  isLoading: boolean;
  showArchived: boolean;
  showTaskDetails: boolean;
  sort: string;
  group: string;
  onSortChange: (value: string) => void;
  onGroupChange: (value: string) => void;
  onShowArchivedChange: (value: boolean) => void;
  onRowClick: (task: Task) => void;
  deletingTaskId: string | null;
  onArchive: (id: string, opts?: { cascade?: boolean }) => Promise<void>;
  onUnarchive: (id: string) => Promise<void>;
  onDelete: (id: string, opts?: { cascade?: boolean }) => Promise<void>;
  onRefresh: () => Promise<void>;
  mobileActions: ReactNode;
};

export function TasksPageContent(props: Props) {
  return (
    <div className="flex h-full min-h-0 w-full flex-col bg-background">
      <KanbanHeader {...props.header} />
      {props.isMobile && props.isMobileSearchOpen && (
        <MobileSearchBar
          searchQuery={props.header.searchQuery ?? ""}
          onSearchChange={props.header.onSearchChange!}
        />
      )}
      <TasksListView
        showArchived={props.showArchived}
        setShowArchived={props.onShowArchivedChange}
        tasksListSort={props.sort}
        onTasksListSortChange={props.onSortChange}
        tasksListGroup={props.group}
        onTasksListGroupChange={props.onGroupChange}
        facetOptions={props.facetOptions}
        facetValues={props.facetValues}
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
