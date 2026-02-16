"use client"

import { useState, useCallback, useMemo, useEffect } from "react"
import { useRouter } from "next/navigation"
import type { PaginationState } from "@tanstack/react-table"
import { DataTable } from "@/components/ui/data-table"
import { getColumns } from "./columns"
import { archiveTask, deleteTask, listTasksByWorkspace } from "@/lib/api"
import { TaskCreateDialog } from "@/components/task-create-dialog"
import { KanbanHeader } from "@/components/kanban/kanban-header"
import { Checkbox } from "@kandev/ui/checkbox"
import { Label } from "@kandev/ui/label"
import type { Task, Workspace, Workflow, WorkflowStep, Repository } from "@/lib/types/http"
import { useToast } from "@/components/toast-provider"
import { useKanbanDisplaySettings } from "@/hooks/use-kanban-display-settings"
import { useDebounce } from "@/hooks/use-debounce"

interface TasksPageClientProps {
  workspaces: Workspace[]
  initialWorkspaceId?: string
  initialWorkflows: Workflow[]
  initialSteps: WorkflowStep[]
  initialRepositories: Repository[]
  initialTasks: Task[]
  initialTotal: number
}

export function TasksPageClient({
  initialWorkflows,
  initialSteps,
  initialRepositories,
  initialTasks,
  initialTotal,
}: TasksPageClientProps) {
  const router = useRouter()
  const { toast } = useToast()
  const {
    activeWorkspaceId,
    activeWorkflowId,
    repositories: storeRepositories,
  } = useKanbanDisplaySettings()

  // For columns, we just need id and name from workflows - use initial workflows for full data
  const [workflows] = useState(initialWorkflows)
  const [steps] = useState(initialSteps)
  // Use store repositories if available for filtering, but fall back to initial
  const repositories = storeRepositories.length > 0 ? storeRepositories : initialRepositories
  const [tasks, setTasks] = useState(initialTasks)
  const [total, setTotal] = useState(initialTotal)
  const [isLoading, setIsLoading] = useState(false)
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [deletingTaskId, setDeletingTaskId] = useState<string | null>(null)

  // Search and filter state
  const [searchQuery, setSearchQuery] = useState('')
  const debouncedQuery = useDebounce(searchQuery, 300)
  const [showArchived, setShowArchived] = useState(false)

  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: 25,
  })

  const pageCount = useMemo(
    () => Math.ceil(total / pagination.pageSize),
    [total, pagination.pageSize]
  )

  const fetchTasks = useCallback(async () => {
    if (!activeWorkspaceId) return
    setIsLoading(true)
    try {
      const result = await listTasksByWorkspace(activeWorkspaceId, {
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        query: debouncedQuery,
        includeArchived: showArchived,
      })

      setTasks(result.tasks)
      setTotal(result.total)
    } catch (err) {
      toast({
        title: "Failed to load tasks",
        description: err instanceof Error ? err.message : "Unknown error",
        variant: "error",
      })
    } finally {
      setIsLoading(false)
    }
  }, [activeWorkspaceId, pagination.pageIndex, pagination.pageSize, debouncedQuery, showArchived, toast])

  // Reset to page 1 when search query changes
  useEffect(() => {
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }, [debouncedQuery])

  // Refetch when workspace, pagination, or search query changes
  useEffect(() => {
    if (activeWorkspaceId) {
      fetchTasks()
    }
  }, [activeWorkspaceId, pagination.pageIndex, pagination.pageSize, debouncedQuery, showArchived, fetchTasks])

  const handleCreateTask = useCallback(() => {
    setCreateDialogOpen(true)
  }, [])

  const handleArchive = useCallback(
    async (taskId: string) => {
      try {
        await archiveTask(taskId)
        toast({
          title: "Task archived",
          description: "The task has been archived successfully.",
        })
        fetchTasks()
      } catch (err) {
        toast({
          title: "Failed to archive task",
          description: err instanceof Error ? err.message : "Unknown error",
          variant: "error",
        })
      }
    },
    [fetchTasks, toast]
  )

  const handleDelete = useCallback(
    async (taskId: string) => {
      setDeletingTaskId(taskId)
      try {
        await deleteTask(taskId)
        toast({
          title: "Task deleted",
          description: "The task has been deleted successfully.",
        })
        fetchTasks()
      } catch (err) {
        toast({
          title: "Failed to delete task",
          description: err instanceof Error ? err.message : "Unknown error",
          variant: "error",
        })
      } finally {
        setDeletingTaskId(null)
      }
    },
    [fetchTasks, toast]
  )

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
    [workflows, steps, repositories, handleArchive, handleDelete, deletingTaskId]
  )

  const handleRowClick = useCallback(
    (task: Task) => {
      if (task.primary_session_id) {
        router.push(`/s/${task.primary_session_id}`)
      } else {
        toast({
          title: "No session available",
          description: "This task has no associated session yet.",
        })
      }
    },
    [router, toast]
  )

  const defaultWorkflow = activeWorkflowId
    ? workflows.find((w) => w.id === activeWorkflowId)
    : workflows[0]
  const defaultStep = steps.find((s) => s.workflow_id === defaultWorkflow?.id)

  return (
    <div className="h-screen w-full flex flex-col bg-background">
      <KanbanHeader
        onCreateTask={handleCreateTask}
        workspaceId={activeWorkspaceId ?? undefined}
        currentPage="tasks"
        searchQuery={searchQuery}
        onSearchChange={setSearchQuery}
        isSearchLoading={isLoading && !!debouncedQuery}
      />
      <div className="flex-1 overflow-auto px-6 py-6">
        <div className="mx-auto max-w-5xl">
          <div className="mb-5 flex items-center justify-between">
            <div>
              <h1 className="text-xl font-semibold">All Tasks</h1>
              <p className="text-sm text-muted-foreground">
                {total} task{total !== 1 ? 's' : ''} found
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

      {activeWorkspaceId && defaultWorkflow && defaultStep && (
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
            setCreateDialogOpen(false)
            fetchTasks()
          }}
        />
      )}
    </div>
  )
}
