"use client";

import { useState } from "react";
import { useRouter } from "@/lib/routing/client-router";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@kandev/ui/alert-dialog";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconChevronDown, IconChevronUp, IconRefresh, IconTrash } from "@tabler/icons-react";
import { useAutomationRuns } from "@/hooks/domains/settings/use-automation-runs";
import type { AutomationRun, ExecutionMode, RunStatus } from "@/lib/types/automation";
import { formatRelativeTime } from "./format-utils";

type RunsSectionProps = {
  automationId: string | null;
  executionMode: ExecutionMode;
  workspaceId: string;
};

const STATUS_BADGE: Record<
  RunStatus,
  { variant: "default" | "destructive" | "secondary" | "outline"; label: string }
> = {
  triggered: { variant: "secondary", label: "Triggered" },
  task_created: { variant: "secondary", label: "Running" },
  succeeded: { variant: "default", label: "Succeeded" },
  failed: { variant: "destructive", label: "Failed" },
  skipped: { variant: "outline", label: "Skipped" },
};

type RunRowProps = {
  run: AutomationRun;
  taskClickable: boolean;
  onDelete: (id: string) => void;
  onNavigate: (taskId: string) => void;
};

function RunRow({ run, taskClickable, onDelete, onNavigate }: RunRowProps) {
  const badge = STATUS_BADGE[run.status] ?? STATUS_BADGE.triggered;
  const rowClickable = taskClickable && !!run.task_id;
  return (
    <TableRow
      className={
        rowClickable
          ? "group cursor-pointer hover:bg-muted/50"
          : "group hover:bg-transparent focus-within:bg-transparent"
      }
      onClick={rowClickable ? () => onNavigate(run.task_id) : undefined}
    >
      <TableCell className="text-sm">{run.trigger_type}</TableCell>
      <TableCell>
        <Badge variant={badge.variant}>{badge.label}</Badge>
      </TableCell>
      <TableCell className="text-sm font-mono">
        {run.task_id ? run.task_id.slice(0, 8) : "-"}
      </TableCell>
      <TableCell className="text-sm text-muted-foreground">
        {formatRelativeTime(run.created_at)}
      </TableCell>
      <TableCell className="text-sm text-destructive max-w-[200px] truncate">
        {run.error_message || "-"}
      </TableCell>
      <TableCell>
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer text-muted-foreground hover:text-destructive opacity-0 pointer-events-none transition-opacity group-hover:opacity-100 group-hover:pointer-events-auto focus-visible:opacity-100 focus-visible:pointer-events-auto [@media(hover:none)]:opacity-100 [@media(hover:none)]:pointer-events-auto"
          onClick={(e) => {
            e.stopPropagation();
            e.preventDefault();
            onDelete(run.id);
          }}
          title="Delete run"
          data-testid="delete-run"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </TableCell>
    </TableRow>
  );
}

type DeleteAllButtonProps = { disabled: boolean; onConfirm: () => void };

function DeleteAllButton({ disabled, onConfirm }: DeleteAllButtonProps) {
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer text-destructive hover:text-destructive"
          disabled={disabled}
          title="Delete all runs"
          data-testid="delete-all-runs"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete all runs?</AlertDialogTitle>
          <AlertDialogDescription>
            This will permanently remove all run records for this automation — including any not
            currently loaded — and their associated tasks. This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={onConfirm}
            data-testid="delete-all-runs-confirm"
          >
            Delete all
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

export function RunsSection({ automationId, executionMode, workspaceId }: RunsSectionProps) {
  const [expanded, setExpanded] = useState(false);
  const { runs, loading, refresh, deleteRun, deleteAllRuns } = useAutomationRuns(
    automationId,
    workspaceId,
  );
  const router = useRouter();

  if (!automationId) return null;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <button
          className="flex items-center gap-2 cursor-pointer"
          onClick={() => setExpanded(!expanded)}
        >
          <Label className="text-xs uppercase tracking-wider text-muted-foreground cursor-pointer">
            Recent Runs ({runs.length})
          </Label>
          {expanded ? (
            <IconChevronUp className="h-3.5 w-3.5 text-muted-foreground" />
          ) : (
            <IconChevronDown className="h-3.5 w-3.5 text-muted-foreground" />
          )}
        </button>
        {expanded && (
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon-sm"
              className="cursor-pointer"
              onClick={refresh}
              disabled={loading}
              title="Refresh"
            >
              <IconRefresh className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            </Button>
            {runs.length > 0 && <DeleteAllButton disabled={loading} onConfirm={deleteAllRuns} />}
          </div>
        )}
      </div>
      {expanded && (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent focus-within:bg-transparent">
                <TableHead>Trigger</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Task</TableHead>
                <TableHead>Time</TableHead>
                <TableHead>Error</TableHead>
                <TableHead className="w-8" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-4">
                    {loading ? "Loading..." : "No runs yet"}
                  </TableCell>
                </TableRow>
              ) : (
                runs.map((run) => (
                  <RunRow
                    key={run.id}
                    run={run}
                    taskClickable={executionMode !== "run"}
                    onDelete={deleteRun}
                    onNavigate={(id) => router.push(`/tasks/${id}`)}
                  />
                ))
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
