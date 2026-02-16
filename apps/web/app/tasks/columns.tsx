"use client"

import type { ColumnDef } from "@tanstack/react-table"
import type { Task, Workflow, WorkflowStep, Repository } from "@/lib/types/http"
import Link from "next/link"
import { IconTrash, IconLoader } from "@tabler/icons-react"
import { Button } from "@kandev/ui/button"
import { Badge } from "@kandev/ui/badge"
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@kandev/ui/tooltip"
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
  onDelete: (taskId: string) => void
  deletingTaskId: string | null
}

export function getColumns({
  workflows,
  steps,
  repositories,
  onDelete,
  deletingTaskId,
}: ColumnsConfig): ColumnDef<TaskWithResolution>[] {
  const workflowMap = new Map(workflows.map((w) => [w.id, w.name]))
  const stepMap = new Map(steps.map((s) => [s.id, s.name]))
  const repoMap = new Map(repositories.map((r) => [r.id, { name: r.name, path: r.local_path }]))

  return [
    {
      accessorKey: "title",
      header: "Title",
      cell: ({ row }) => {
        const task = row.original
        const sessionId = task.primary_session_id
        if (sessionId) {
          return (
            <Link
              href={`/s/${sessionId}`}
              className="text-primary hover:underline font-medium"
            >
              {task.title}
            </Link>
          )
        }
        return <span className="font-medium">{task.title}</span>
      },
    },
    {
      accessorKey: "workflow_id",
      header: "Workflow",
      cell: ({ row }) => {
        const workflowName = workflowMap.get(row.original.workflow_id)
        return workflowName || "-"
      },
    },
    {
      accessorKey: "workflow_step_id",
      header: "Column",
      cell: ({ row }) => {
        const stepName = stepMap.get(row.original.workflow_step_id)
        return stepName || "-"
      },
    },
    {
      accessorKey: "repositories",
      header: "Repository",
      cell: ({ row }) => {
        const taskRepos = row.original.repositories || []
        if (taskRepos.length === 0) return "-"
        const repoInfos = taskRepos
          .map((tr) => repoMap.get(tr.repository_id))
          .filter((info): info is { name: string; path: string } => info !== undefined)
        if (repoInfos.length === 0) return "-"

        return (
          <TooltipProvider>
            <div className="flex flex-wrap gap-1">
              {repoInfos.map((info, idx) => (
                <Tooltip key={idx}>
                  <TooltipTrigger asChild>
                    <span className="cursor-default hover:underline">{info.name}</span>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p className="font-mono text-xs">{info.path || 'No path'}</p>
                  </TooltipContent>
                </Tooltip>
              ))}
            </div>
          </TooltipProvider>
        )
      },
    },
    {
      accessorKey: "state",
      header: "State",
      cell: ({ row }) => {
        const state = row.original.state
        const variant = state === "COMPLETED" ? "default" : state === "IN_PROGRESS" ? "secondary" : "outline"
        return <Badge variant={variant}>{state}</Badge>
      },
    },
    {
      accessorKey: "updated_at",
      header: "Updated",
      cell: ({ row }) => {
        const date = new Date(row.original.updated_at)
        return formatDistanceToNow(date, { addSuffix: true })
      },
    },
    {
      id: "actions",
      header: "Actions",
      cell: ({ row }) => {
        const isDeleting = deletingTaskId === row.original.id
        return (
          <Button
            variant="ghost"
            size="sm"
            className="cursor-pointer"
            disabled={isDeleting}
            onClick={(e) => {
              e.stopPropagation()
              onDelete(row.original.id)
            }}
          >
            {isDeleting ? (
              <IconLoader className="h-4 w-4 animate-spin" />
            ) : (
              <IconTrash className="h-4 w-4 text-destructive" />
            )}
          </Button>
        )
      },
    },
  ]
}
