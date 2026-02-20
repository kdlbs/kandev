"use client";

import { useState, useCallback, useMemo, useEffect } from "react";
import { useRouter } from "next/navigation";
import type { PaginationState } from "@tanstack/react-table";
import { DataTable } from "@/components/ui/data-table";
import { getColumns } from "./columns";
import { archiveTask, deleteTask, listTasksByWorkspace } from "@/lib/api";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import { Checkbox } from "@kandev/ui/checkbox";
import { Label } from "@kandev/ui/label";
import type { Task, Workspace, Workflow, WorkflowStep, Repository } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useDebounce } from "@/hooks/use-debounce";

interface TasksPageClientProps {
  workspaces: Workspace[];
  initialWorkspaceId?: string;
  initialWorkflows: Workflow[];
  initialSteps: WorkflowStep[];
  initialRepositories: Repository[];
  initialTasks: Task[];
  initialTotal: number;
}

type UseTaskOperationsParams = {
  activeWorkspaceId: string | null;
  pagination: PaginationState;
  debouncedQuery: string;
  showArchived: boolean;
  setTasks: (tasks: Task[]) => void;
  setTotal: (total: number) => void;
};

function useTaskOperations({
  activeWorkspaceId,
  pagination,
  debouncedQuery,
  showArchived,
  setTasks,
  setTotal,
}: UseTaskOperationsParams) {
  const { toast } = useToast();
  const [isLoading, setIsLoading] = useState(false);
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null);

  const fetchTasks = useCallback(async () => {
    if (!activeWorkspaceId) return;
    setIsLoading(true);
    try {
      const result = await listTasksByWorkspace(activeWorkspaceId, {
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        query: debouncedQuery,
        includeArchived: showArchived,
      });
      setTasks(result.tasks);
      setTotal(result.total);
    } catch (err) {
      toast({
        title: "Failed to load tasks",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      });
    } finally {
      setIsLoading(false);
    }
  }, [
    activeWorkspaceId,
    pagination.pageIndex,
    pagination.pageSize,
    debouncedQuery,
    showArchived,
    toast,
    setTasks,
    setTotal,
  ]);

  const handleArchive = useCallback(
    async (taskId: string) => {
      try {
        await archiveTask(taskId);
        toast({ title: "Task archived", description: "The task has been archived successfully." });
        fetchTasks();
      } catch (err) {
        toast({
          title: "Failed to archive task",
          description: err instanceof Error ? err.message : "Unknown error",
          variant: "error",
        });
      }
    },
    [fetchTasks, toast],
  );

  const handleDelete = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId);
      try {
        await deleteTask(taskId);
        toast({ title: "Task deleted", description: "The task has been deleted successfully." });
        fetchTasks();
      } catch (err) {
        toast({
          title: "Failed to delete task",
          description: err instanceof Error ? err.message : "Unknown error",
          variant: "error",
        });
      } finally {
        setDeletingTaskId(null);
      }
    },
    [fetchTasks, toast],
  );

  return { isLoading, deletingTaskId, fetchTasks, handleArchive, handleDelete };
}

type TasksPageBodyProps = {
  total: number;
  showArchived: boolean;
  setShowArchived: (show: boolean) => void;
  columns: ReturnType<typeof getColumns>;
  tasks: Task[];
  pageCount: number;
  pagination: PaginationState;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  isLoading: boolean;
  handleRowClick: (task: Task) => void;
};

function TasksPageBody({
  total,
  showArchived,
  setShowArchived,
  columns,
  tasks,
  pageCount,
  pagination,
  setPagination,
  isLoading,
  handleRowClick,
}: TasksPageBodyProps) {
  return (
    <div className="flex-1 overflow-auto px-6 py-6">
      <div className="mx-auto max-w-5xl">
        <div className="mb-5 flex items-center justify-between">
          <div>
            <h1 className="text-xl font-semibold">All Tasks</h1>
            <p className="text-sm text-muted-foreground">
              {total} task{total !== 1 ? "s" : ""} found
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Checkbox
              id="show-archived"
              checked={showArchived}
              onCheckedChange={(checked) => setShowArchived(checked === true)}
            />
            <Label htmlFor="show-archived" className="text-sm cursor-pointer">
              Show archived
            </Label>
          </div>
        </div>
        <DataTable
          columns={columns}
          data={tasks}
          pageCount={pageCount}
          pagination={pagination}
          onPaginationChange={setPagination}
          isLoading={isLoading}
          onRowClick={handleRowClick}
        />
      </div>
    </div>
  );
}

type TaskCreateDialogMountProps = {
  activeWorkspaceId: string | null;
  defaultWorkflow: Workflow | undefined;
  defaultStep: WorkflowStep | undefined;
  createDialogOpen: boolean;
  setCreateDialogOpen: (open: boolean) => void;
  steps: WorkflowStep[];
  fetchTasks: () => void;
};

function TaskCreateDialogMount({
  activeWorkspaceId,
  defaultWorkflow,
  defaultStep,
  createDialogOpen,
  setCreateDialogOpen,
  steps,
  fetchTasks,
}: TaskCreateDialogMountProps) {
  if (!activeWorkspaceId || !defaultWorkflow || !defaultStep) return null;
  return (
    <TaskCreateDialog
      open={createDialogOpen}
      onOpenChange={setCreateDialogOpen}
      workspaceId={activeWorkspaceId}
      workflowId={defaultWorkflow.id}
      defaultStepId={defaultStep.id}
      steps={steps
        .filter((s) => s.workflow_id === defaultWorkflow.id)
        .map((s) => ({ id: s.id, title: s.name, events: s.events }))}
      onSuccess={() => {
        setCreateDialogOpen(false);
        fetchTasks();
      }}
    />
  );
}

function useTasksPageViewState({
  initialWorkflows,
  initialSteps,
  initialRepositories,
  initialTasks,
  initialTotal,
  storeRepositories,
}: {
  initialWorkflows: Workflow[];
  initialSteps: WorkflowStep[];
  initialRepositories: Repository[];
  initialTasks: Task[];
  initialTotal: number;
  storeRepositories: Repository[];
}) {
  const [workflows] = useState(initialWorkflows);
  const [steps] = useState(initialSteps);
  const repositories = storeRepositories.length > 0 ? storeRepositories : initialRepositories;
  const [tasks, setTasks] = useState(initialTasks);
  const [total, setTotal] = useState(initialTotal);
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [showArchived, setShowArchived] = useState(false);
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize: 25 });

  return {
    workflows,
    steps,
    repositories,
    tasks,
    setTasks,
    total,
    setTotal,
    createDialogOpen,
    setCreateDialogOpen,
    searchQuery,
    setSearchQuery,
    showArchived,
    setShowArchived,
    pagination,
    setPagination,
  };
}

function useTasksPageEffects({
  debouncedQuery,
  setPagination,
  activeWorkspaceId,
  fetchTasks,
  pagination,
  showArchived,
}: {
  debouncedQuery: string;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  activeWorkspaceId: string | null;
  fetchTasks: () => void;
  pagination: PaginationState;
  showArchived: boolean;
}) {
  useEffect(() => {
    void Promise.resolve().then(() => setPagination((prev) => ({ ...prev, pageIndex: 0 })));
  }, [debouncedQuery, setPagination]);

  useEffect(() => {
    if (activeWorkspaceId) fetchTasks();
  }, [
    activeWorkspaceId,
    pagination.pageIndex,
    pagination.pageSize,
    debouncedQuery,
    showArchived,
    fetchTasks,
  ]);
}

function useTasksPageComputed({
  total,
  pagination,
  workflows,
  steps,
  repositories,
  handleArchive,
  handleDelete,
  deletingTaskId,
  router,
  toast,
  activeWorkflowId,
}: {
  total: number;
  pagination: PaginationState;
  workflows: Workflow[];
  steps: WorkflowStep[];
  repositories: Repository[];
  handleArchive: (taskId: string) => Promise<void>;
  handleDelete: (taskId: string) => Promise<void>;
  deletingTaskId: string | null;
  router: ReturnType<typeof useRouter>;
  toast: ReturnType<typeof useToast>["toast"];
  activeWorkflowId: string | null;
}) {
  const pageCount = useMemo(
    () => Math.ceil(total / pagination.pageSize),
    [total, pagination.pageSize],
  );
  const columns = useMemo(
    () =>
      getColumns({
        workflows,
        steps,
        repositories,
        onArchive: handleArchive,
        onDelete: handleDelete,
        deletingTaskId,
      }),
    [workflows, steps, repositories, handleArchive, handleDelete, deletingTaskId],
  );
  const handleRowClick = useCallback(
    (task: Task) => {
      if (task.primary_session_id) {
        router.push(`/s/${task.primary_session_id}`);
      } else {
        toast({
          title: "No session available",
          description: "This task has no associated session yet.",
        });
      }
    },
    [router, toast],
  );
  const defaultWorkflow = activeWorkflowId
    ? workflows.find((w) => w.id === activeWorkflowId)
    : workflows[0];
  const defaultStep = steps.find((s) => s.workflow_id === defaultWorkflow?.id);

  return { pageCount, columns, handleRowClick, defaultWorkflow, defaultStep };
}

function useTasksPageSetup(props: TasksPageClientProps) {
  const router = useRouter();
  const { activeWorkspaceId, activeWorkflowId, repositories: storeRepositories } = useKanbanDisplaySettings();
  const { toast } = useToast();
  const viewState = useTasksPageViewState({
    initialWorkflows: props.initialWorkflows,
    initialSteps: props.initialSteps,
    initialRepositories: props.initialRepositories,
    initialTasks: props.initialTasks,
    initialTotal: props.initialTotal,
    storeRepositories,
  });
  const debouncedQuery = useDebounce(viewState.searchQuery, 300);
  const ops = useTaskOperations({
    activeWorkspaceId, pagination: viewState.pagination, debouncedQuery,
    showArchived: viewState.showArchived, setTasks: viewState.setTasks, setTotal: viewState.setTotal,
  });
  useTasksPageEffects({
    debouncedQuery, setPagination: viewState.setPagination,
    activeWorkspaceId, fetchTasks: ops.fetchTasks,
    pagination: viewState.pagination, showArchived: viewState.showArchived,
  });
  const computed = useTasksPageComputed({
    total: viewState.total, pagination: viewState.pagination,
    workflows: viewState.workflows, steps: viewState.steps,
    repositories: viewState.repositories,
    handleArchive: ops.handleArchive, handleDelete: ops.handleDelete,
    deletingTaskId: ops.deletingTaskId, router, toast, activeWorkflowId,
  });
  return { ...viewState, ...ops, ...computed, activeWorkspaceId, debouncedQuery };
}

export function TasksPageClient(props: TasksPageClientProps) {
  const s = useTasksPageSetup(props);
  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <KanbanHeader
        onCreateTask={() => s.setCreateDialogOpen(true)}
        workspaceId={s.activeWorkspaceId ?? undefined}
        currentPage="tasks"
        searchQuery={s.searchQuery}
        onSearchChange={s.setSearchQuery}
        isSearchLoading={s.isLoading && !!s.debouncedQuery}
      />
      <TasksPageBody
        total={s.total}
        showArchived={s.showArchived}
        setShowArchived={s.setShowArchived}
        columns={s.columns}
        tasks={s.tasks}
        pageCount={s.pageCount}
        pagination={s.pagination}
        setPagination={s.setPagination}
        isLoading={s.isLoading}
        handleRowClick={s.handleRowClick}
      />
      <TaskCreateDialogMount
        activeWorkspaceId={s.activeWorkspaceId}
        defaultWorkflow={s.defaultWorkflow}
        defaultStep={s.defaultStep}
        createDialogOpen={s.createDialogOpen}
        setCreateDialogOpen={s.setCreateDialogOpen}
        steps={s.steps}
        fetchTasks={s.fetchTasks}
      />
    </div>
  );
}
