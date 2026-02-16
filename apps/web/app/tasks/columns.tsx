"use client"

import type { ColumnDef } from "@tanstack/react-table"
import type { Task, Workflow, WorkflowStep, Repository } from "@/lib/types/http"
import Link from "next/link"
import { IconTrash, IconLoader, IconArchive } from "@tabler/icons-react"
import { Button } from "@kandev/ui/button"
import { Badge } from "@kandev/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip"
import { formatDistanceToNow } from "date-fns"

type TaskWithResolution = Task & {
  workflowName?: string
  stepName?: string
  repositoryNames?: string[]
}

interface ColumnsConfig {
  workflows: Workflow[]
  steps: WorkflowStep[]
  repositories: Repository[]
  onArchive: (taskId: string) => void
  onDelete: (taskId: string) => void
  deletingTaskId: string | null
}

export function getColumns({
  workflows,
  steps,
  repositories,
  onArchive,
  onDelete,
  deletingTaskId,
}: ColumnsConfig): ColumnDef<TaskWithResolution>[] {
  const workflowMap = new Map(workflows.map((w) => [w.id, w.name]))
  const stepMap = new Map(steps.map((s) => [s.id, s.name]))
  const repoMap = new Map(repositories.map((r) => [r.id, r.name]))

  return [
    {
      accessorKey: "title",
      header: "Task",
      cell: ({ row }) => {
        const task = row.original
        const sessionId = task.primary_session_id
        const isArchived = !!task.archived_at
        const repoName = task.repositories?.[0]
          ? repoMap.get(task.repositories[0].repository_id)
          : undefined

        return (
          <div className="flex flex-col gap-0.5 py-0.5">
            <div className="flex items-center gap-2">
              {sessionId ? (
                <Link
                  href={`/s/${sessionId}`}
                  className="text-primary font-medium text-sm"
                >
                  {task.title}
                </Link>
              ) : (
                <span className="font-medium text-sm">{task.title}</span>
              )}
              {isArchived && (
                <Badge variant="outline" className="text-[10px] px-1.5 py-0 text-amber-500 border-amber-500/30">
                  Archived
                </Badge>
              )}
            </div>
            {repoName && (
              <span className="text-xs text-muted-foreground/60">{repoName}</span>
            )}
          </div>
        )
      },
    },
    {
      accessorKey: "workflow_id",
      header: "Workflow",
      cell: ({ row }) => {
        const name = workflowMap.get(row.original.workflow_id)
        return (
          <span className="text-xs text-muted-foreground">
            {name || "-"}
          </span>
        )
      },
    },
    {
      accessorKey: "workflow_step_id",
      header: "Step",
      cell: ({ row }) => {
        const stepName = stepMap.get(row.original.workflow_step_id)
        return (
          <span className="text-xs text-muted-foreground bg-foreground/[0.06] px-2 py-0.5 rounded-md">
            {stepName || "-"}
          </span>
        )
      },
    },
    {
      accessorKey: "updated_at",
      header: "Updated",
      cell: ({ row }) => {
        const date = new Date(row.original.updated_at)
        return (
          <span className="text-xs text-muted-foreground">
            {formatDistanceToNow(date, { addSuffix: true })}
          </span>
        )
      },
    },
    {
      id: "actions",
      header: "",
      cell: ({ row }) => {
        const task = row.original
        const isDeleting = deletingTaskId === task.id
        const isArchived = !!task.archived_at
        return (
          <div className="flex items-center justify-end gap-0.5">
            {!isArchived && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="cursor-pointer h-7 w-7 p-0"
                    onClick={(e) => {
                      e.stopPropagation()
                      onArchive(task.id)
                    }}
                  >
                    <IconArchive className="h-3.5 w-3.5 text-muted-foreground" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>Archive</TooltipContent>
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
                    e.stopPropagation()
                    onDelete(task.id)
                  }}
                >
                  {isDeleting ? (
                    <IconLoader className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <IconTrash className="h-3.5 w-3.5 text-destructive" />
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>Delete</TooltipContent>
            </Tooltip>
          </div>
        )
      },
    },
  ]
}
