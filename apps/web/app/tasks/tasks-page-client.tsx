"use client";

import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useRouter } from "@/lib/routing/client-router";
import type { PaginationState } from "@tanstack/react-table";
import { archiveTask, deleteTask, listTasksByWorkspace } from "@/lib/api";
import { TaskCreateDialog } from "@/components/task-create-dialog";
import { KanbanHeader } from "@/components/kanban/kanban-header";
import type { Task, Workspace, Workflow, WorkflowStep, Repository } from "@/lib/types/http";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings";
import { useDebounce } from "@/hooks/use-debounce";
import { linkToTask } from "@/lib/links";
import { shouldSkipInitialTasksFetch } from "./tasks-page-fetch-policy";
import { TasksListView } from "./tasks-list-view";

interface TasksPageClientProps {
  workspaces: Workspace[];
  initialWorkspaceId?: string;
  initialWorkflows: Workflow[];
  initialSteps: WorkflowStep[];
  initialRepositories: Repository[];
  initialTasks: Task[];
  initialTotal: number;
  initialDataLoaded?: boolean;
}

type UseTaskOperationsParams = {
  activeWorkspaceId: string | null;
  activeWorkflowId: string | null;
  selectedRepositoryId: string | null;
  pagination: PaginationState;
  debouncedQuery: string;
  showArchived: boolean;
  setTasks: (tasks: Task[]) => void;
  setTotal: (total: number) => void;
};

function useTaskOperations({
  activeWorkspaceId,
  activeWorkflowId,
  selectedRepositoryId,
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
        workflowId: activeWorkflowId,
        repositoryId: selectedRepositoryId,
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
    activeWorkflowId,
    selectedRepositoryId,
    pagination.pageIndex,
    pagination.pageSize,
    debouncedQuery,
    showArchived,
    toast,
    setTasks,
    setTotal,
  ]);

  const handleArchive = useCallback(
    async (taskId: string, opts?: { cascade?: boolean }) => {
      try {
        await archiveTask(taskId, opts);
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
    async (taskId: string, opts?: { cascade?: boolean }) => {
      setDeletingTaskId(taskId);
      try {
        await deleteTask(taskId, opts);
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
  activeWorkflowId,
  selectedRepositoryId,
  initialDataLoaded = false,
}: {
  debouncedQuery: string;
  setPagination: (next: PaginationState | ((prev: PaginationState) => PaginationState)) => void;
  activeWorkspaceId: string | null;
  fetchTasks: () => void;
  pagination: PaginationState;
  showArchived: boolean;
  activeWorkflowId: string | null;
  selectedRepositoryId: string | null;
  initialDataLoaded?: boolean;
}) {
  const skippedInitialFetchRef = useRef(false);

  useEffect(() => {
    void Promise.resolve().then(() => setPagination((prev) => ({ ...prev, pageIndex: 0 })));
  }, [debouncedQuery, activeWorkflowId, selectedRepositoryId, setPagination]);

  useEffect(() => {
    if (
      shouldSkipInitialTasksFetch({
        hasInitialData: initialDataLoaded,
        alreadySkipped: skippedInitialFetchRef.current,
        pageIndex: pagination.pageIndex,
        debouncedQuery,
        showArchived,
      })
    ) {
      skippedInitialFetchRef.current = true;
      return;
    }
    if (activeWorkspaceId) fetchTasks();
  }, [
    activeWorkspaceId,
    pagination.pageIndex,
    pagination.pageSize,
    debouncedQuery,
    showArchived,
    fetchTasks,
    initialDataLoaded,
  ]);
}

function useTasksPageComputed({
  total,
  pagination,
  workflows,
  steps,
  router,
  activeWorkflowId,
}: {
  total: number;
  pagination: PaginationState;
  workflows: Workflow[];
  steps: WorkflowStep[];
  router: ReturnType<typeof useRouter>;
  activeWorkflowId: string | null;
}) {
  const pageCount = useMemo(
    () => Math.ceil(total / pagination.pageSize),
    [total, pagination.pageSize],
  );
  const handleRowClick = useCallback(
    (task: Task) => {
      router.push(linkToTask(task.id));
    },
    [router],
  );
  const defaultWorkflow = activeWorkflowId
    ? workflows.find((w) => w.id === activeWorkflowId)
    : workflows[0];
  const defaultStep = steps.find((s) => s.workflow_id === defaultWorkflow?.id);

  return { pageCount, handleRowClick, defaultWorkflow, defaultStep };
}

function useTasksPageSetup(props: TasksPageClientProps) {
  const router = useRouter();
  const {
    activeWorkspaceId,
    activeWorkflowId,
    repositories: storeRepositories,
    selectedRepositoryId,
  } = useKanbanDisplaySettings();
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
    activeWorkspaceId,
    activeWorkflowId,
    selectedRepositoryId,
    pagination: viewState.pagination,
    debouncedQuery,
    showArchived: viewState.showArchived,
    setTasks: viewState.setTasks,
    setTotal: viewState.setTotal,
  });
  useTasksPageEffects({
    debouncedQuery,
    setPagination: viewState.setPagination,
    activeWorkspaceId,
    fetchTasks: ops.fetchTasks,
    pagination: viewState.pagination,
    showArchived: viewState.showArchived,
    activeWorkflowId,
    selectedRepositoryId,
    initialDataLoaded: props.initialDataLoaded,
  });
  const computed = useTasksPageComputed({
    total: viewState.total,
    pagination: viewState.pagination,
    workflows: viewState.workflows,
    steps: viewState.steps,
    router,
    activeWorkflowId,
  });
  return { ...viewState, ...ops, ...computed, activeWorkspaceId, debouncedQuery };
}

export function TasksPageClient(props: TasksPageClientProps) {
  const s = useTasksPageSetup(props);
  const setMobileSearchOpen = useAppStore((state) => state.setMobileKanbanSearchOpen);

  useEffect(() => {
    setMobileSearchOpen(false);
    return () => setMobileSearchOpen(false);
  }, [setMobileSearchOpen]);

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <KanbanHeader workspaceId={s.activeWorkspaceId ?? undefined} currentPage="tasks" />
      <TasksListView
        showArchived={s.showArchived}
        setShowArchived={s.setShowArchived}
        tasks={s.tasks}
        workflows={s.workflows}
        steps={s.steps}
        repositories={s.repositories}
        total={s.total}
        pageCount={s.pageCount}
        pagination={s.pagination}
        setPagination={s.setPagination}
        isLoading={s.isLoading}
        handleRowClick={s.handleRowClick}
        searchQuery={s.searchQuery}
        setSearchQuery={s.setSearchQuery}
        setCreateDialogOpen={s.setCreateDialogOpen}
        deletingTaskId={s.deletingTaskId}
        handleArchive={s.handleArchive}
        handleDelete={s.handleDelete}
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
