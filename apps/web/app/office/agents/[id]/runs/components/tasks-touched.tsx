"use client";

import Link from "@/components/routing/app-link";
import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { useAppStore } from "@/components/state-provider";
import { officeTaskQueryOptions } from "@/lib/query/query-options";
import type { OfficeTask } from "@/lib/state/slices/office/types";
import { StatusIcon } from "@/app/office/tasks/status-icon";

type TasksTouchedProps = {
  runId: string;
  /**
   * Pre-resolved task ids from the run detail response. The component
   * reads the full task rows (identifier, title, status, priority)
   * via the task query cache. Empty list renders the empty state.
   */
  taskIds: string[];
};

/**
 * Tasks Touched table on the run detail page. Each row links to
 * `/office/tasks/:id`. Renders a compact empty state when the run
 * produced no activity rows under any task — common for e.g. heartbeat
 * runs that finished without claiming any task work.
 *
 * Prop contract is stable (Wave 0): `runId` + `taskIds`. The real task
 * rows are fetched lazily through TanStack Query so the run detail page
 * can stream in even before the tasks API responds.
 */
export function TasksTouched({ runId, taskIds }: TasksTouchedProps) {
  if (taskIds.length === 0) {
    return <EmptyState />;
  }
  return (
    <Card data-testid="tasks-touched" data-run-id={runId}>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Tasks touched</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <TasksTable taskIds={taskIds} />
      </CardContent>
    </Card>
  );
}

function EmptyState() {
  return (
    <Card data-testid="tasks-touched-empty">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">Tasks touched</CardTitle>
      </CardHeader>
      <CardContent className="pt-0">
        <p className="text-xs text-muted-foreground">No tasks were modified during this run.</p>
      </CardContent>
    </Card>
  );
}

type RowState =
  | { kind: "loading"; id: string }
  | { kind: "loaded"; task: OfficeTask }
  | { kind: "error"; id: string };

function TasksTable({ taskIds }: { taskIds: string[] }) {
  const rows = useTaskRows(taskIds);
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="w-[110px]">Identifier</TableHead>
          <TableHead>Title</TableHead>
          <TableHead className="w-[120px]">Status</TableHead>
          <TableHead className="w-[100px]">Priority</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TaskTableRow key={rowKey(row)} row={row} />
        ))}
      </TableBody>
    </Table>
  );
}

function rowKey(row: RowState): string {
  if (row.kind === "loaded") return row.task.id;
  return row.id;
}

function TaskTableRow({ row }: { row: RowState }) {
  if (row.kind === "loading") {
    return (
      <TableRow data-testid="tasks-touched-row-loading">
        <TableCell colSpan={4} className="text-xs text-muted-foreground">
          Loading {shortId(row.id)}…
        </TableCell>
      </TableRow>
    );
  }
  if (row.kind === "error") {
    return (
      <TableRow data-testid="tasks-touched-row-error">
        <TableCell colSpan={4} className="text-xs text-muted-foreground">
          Failed to load {shortId(row.id)}
        </TableCell>
      </TableRow>
    );
  }
  const { task } = row;
  return (
    <TableRow
      data-testid="tasks-touched-row"
      data-task-id={task.id}
      className="cursor-pointer hover:bg-muted/60"
    >
      <TableCell className="font-mono text-xs">
        <Link href={`/office/tasks/${task.id}`} className="hover:underline">
          {task.identifier || shortId(task.id)}
        </Link>
      </TableCell>
      <TableCell>
        <Link
          href={`/office/tasks/${task.id}`}
          className="hover:underline"
          data-testid="tasks-touched-row-title"
        >
          {task.title || "(untitled)"}
        </Link>
      </TableCell>
      <TableCell>
        <span className="inline-flex items-center gap-1.5 text-xs">
          <StatusIcon status={task.status} className="h-3.5 w-3.5" />
          {formatStatus(task.status)}
        </span>
      </TableCell>
      <TableCell className="text-xs capitalize">{task.priority || "—"}</TableCell>
    </TableRow>
  );
}

function shortId(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

function formatStatus(status: string): string {
  return status.replace(/_/g, " ");
}

/**
 * Loads each task row in parallel through the per-task query key. Kept
 * inline because this component is the only arbitrary id-list consumer.
 */
function useTaskRows(taskIds: string[]): RowState[] {
  const workspaceId = useAppStore((s) => s.workspaces.activeId) ?? "";
  const queries = useQueries({
    queries: taskIds.map((taskId) => officeTaskQueryOptions(workspaceId, taskId)),
  });

  return useMemo(
    () =>
      taskIds.map((id, index) => {
        if (!workspaceId) return { kind: "error" as const, id };
        const query = queries[index];
        if (query?.data?.task) return { kind: "loaded" as const, task: query.data.task };
        if (query?.isError || query?.isSuccess) return { kind: "error" as const, id };
        return { kind: "loading" as const, id };
      }),
    [taskIds, queries, workspaceId],
  );
}
