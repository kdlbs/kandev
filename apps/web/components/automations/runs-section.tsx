"use client";

import { useTranslation } from "react-i18next";
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
import type { AutomationRun, RunStatus } from "@/lib/types/automation";
import { formatRelativeTime } from "@/lib/utils";

type RunsSectionProps = {
  automationId: string | null;
  workspaceId: string;
};

const STATUS_BADGE: Record<
  RunStatus,
  { variant: "default" | "destructive" | "secondary" | "outline"; labelKey: string }
> = {
  triggered: { variant: "secondary", labelKey: "automations:runStatusTriggered" },
  task_created: { variant: "secondary", labelKey: "automations:runStatusRunning" },
  succeeded: { variant: "default", labelKey: "automations:runStatusSucceeded" },
  failed: { variant: "destructive", labelKey: "automations:runStatusFailed" },
  skipped: { variant: "outline", labelKey: "automations:runStatusSkipped" },
  // The generating task was archived — via the UI or by the agent itself
  // (e.g. an "archive this task" instruction). Distinct from a genuine
  // user cancellation: archiving just closes the task out, it doesn't
  // mean the run's work was rejected. See internal/automation.RunStatusArchived.
  archived: { variant: "outline", labelKey: "automations:runStatusArchived" },
  // The generating task no longer exists, or its current primary session
  // is CANCELLED — a real cancellation, distinct from archived.
  // See internal/automation.RunStatusCancelled.
  cancelled: { variant: "outline", labelKey: "automations:runStatusCancelled" },
};

type RunRowProps = {
  run: AutomationRun;
  deleting: boolean;
  onDelete: (id: string) => void;
  onNavigate: (taskId: string) => void;
};

function RunRow({ run, deleting, onDelete, onNavigate }: RunRowProps) {
  const { t } = useTranslation();
  const badge = STATUS_BADGE[run.status] ?? STATUS_BADGE.triggered;
  // Any run that produced a task links to it, run-mode included. Run mode
  // keeps the task off the board, which is not a reason to withhold the only
  // route to what the run actually said.
  const rowClickable = !!run.task_id;
  return (
    <TableRow
      className={
        rowClickable
          ? "group cursor-pointer hover:bg-muted/50"
          : "group hover:bg-transparent focus-within:bg-transparent"
      }
      onClick={rowClickable ? () => onNavigate(run.task_id) : undefined}
      data-testid={`run-row-${run.id}`}
      // The truncated task id used to be a column, and tooling identified a row
      // by reading it. That column collapsed into Outcome, so the association
      // lives here instead of being inferred from rendered copy.
      data-task-id={run.task_id || undefined}
    >
      <TableCell className="text-sm">{run.trigger_type}</TableCell>
      <TableCell>
        <Badge variant={badge.variant}>{t(badge.labelKey)}</Badge>
      </TableCell>
      <TableCell
        className={`text-sm max-w-[420px] truncate ${
          run.error_message ? "text-destructive" : "text-muted-foreground"
        }`}
        title={run.error_message || run.summary || undefined}
        data-testid="run-outcome"
      >
        {run.error_message || run.summary || "-"}
      </TableCell>
      <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
        {formatRelativeTime(run.created_at)}
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
          disabled={deleting}
          title={t("automations:deleteRun")}
          data-testid="delete-run"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </TableCell>
    </TableRow>
  );
}

/**
 * Status filters for the run log. "Skipped" is deliberately one of them: a
 * scheduled firing turned away by the concurrency cap writes a row and
 * nothing else, so without a way to see those a paused automation looks
 * identical to one that was never due.
 */
const STATUS_FILTERS: { value: RunStatus | "all"; labelKey: string }[] = [
  { value: "all", labelKey: "automations:runAll" },
  { value: "task_created", labelKey: "automations:runStatusRunning" },
  { value: "succeeded", labelKey: "automations:runStatusSucceeded" },
  { value: "failed", labelKey: "automations:runStatusFailed" },
  { value: "skipped", labelKey: "automations:runStatusSkipped" },
  // Both are read-time-derived terminal statuses the table already renders, so
  // leaving them out here made those runs visible but unfilterable.
  { value: "archived", labelKey: "automations:runStatusArchived" },
  { value: "cancelled", labelKey: "automations:runStatusCancelled" },
];

type DeleteAllButtonProps = {
  disabled: boolean;
  statusFilter: RunStatus | "all";
  onConfirm: () => void;
};

function DeleteAllButton({ disabled, statusFilter, onConfirm }: DeleteAllButtonProps) {
  const { t } = useTranslation();
  // A status-scoped delete-all only removes the runs shown in the active
  // view, so the confirmation names that scope instead of promising every
  // run record for the automation.
  const scoped = statusFilter !== "all";
  const statusLabel = scoped
    ? t(STATUS_FILTERS.find((filter) => filter.value === statusFilter)?.labelKey ?? "")
    : "";
  // The accessible name must match the action's actual scope, not just the
  // dialog: in a filtered view "Delete all runs" would promise more than the
  // button deletes.
  const label = scoped
    ? t("automations:deleteAllRunsScoped", { status: statusLabel })
    : t("automations:deleteAllRuns");
  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          variant="ghost"
          size="icon-sm"
          className="cursor-pointer text-destructive hover:text-destructive"
          disabled={disabled}
          title={label}
          aria-label={label}
          data-testid="delete-all-runs"
        >
          <IconTrash className="h-3.5 w-3.5" />
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            {scoped
              ? t("automations:deleteAllRunsScopedTitle", { status: statusLabel })
              : t("automations:deleteAllRunsTitle")}
          </AlertDialogTitle>
          <AlertDialogDescription>
            {scoped
              ? t("automations:deleteAllRunsScopedDescription", { status: statusLabel })
              : t("automations:deleteAllRunsDescription")}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel className="cursor-pointer">{t("common:cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className="cursor-pointer bg-destructive text-destructive-foreground hover:bg-destructive/90"
            onClick={onConfirm}
            data-testid="delete-all-runs-confirm"
          >
            {t("automations:deleteAll")}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function StatusFilter({
  runs,
  value,
  onChange,
}: {
  runs: AutomationRun[];
  value: RunStatus | "all";
  onChange: (value: RunStatus | "all") => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-1 flex-wrap" data-testid="run-status-filter">
      {STATUS_FILTERS.map((filter) => {
        const count =
          filter.value === "all"
            ? runs.length
            : runs.filter((run) => run.status === filter.value).length;
        // Only offer a filter that would show something, so the row of chips
        // reflects what this automation has actually done.
        if (count === 0 && filter.value !== "all" && value !== filter.value) return null;
        return (
          <Button
            key={filter.value}
            variant={value === filter.value ? "secondary" : "ghost"}
            size="sm"
            className="cursor-pointer h-7 text-xs"
            onClick={() => onChange(filter.value)}
            data-testid={`run-filter-${filter.value}`}
          >
            {t(filter.labelKey)}
            <span className="ml-1 text-muted-foreground">{count}</span>
          </Button>
        );
      })}
    </div>
  );
}

export function RunsSection({ automationId, workspaceId }: RunsSectionProps) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const [statusFilter, setStatusFilter] = useState<RunStatus | "all">("all");
  const { runs, loading, refresh, deleteRun, deleteAllRuns, deleting } = useAutomationRuns(
    automationId,
    workspaceId,
  );
  const router = useRouter();

  if (!automationId) return null;

  const visibleRuns =
    statusFilter === "all" ? runs : runs.filter((run) => run.status === statusFilter);
  const emptyMessage = runs.length === 0 ? "No runs yet" : "No runs match this filter";

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <button
          className="flex items-center gap-2 cursor-pointer"
          onClick={() => setExpanded(!expanded)}
        >
          <Label className="text-xs uppercase tracking-wider text-muted-foreground cursor-pointer">
            {t("automations:recentRuns", { count: runs.length })}
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
              disabled={loading || deleting}
              title={t("automations:refresh")}
            >
              <IconRefresh className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
            </Button>
          </div>
        )}
      </div>
      {expanded && runs.length > 0 && (
        <StatusFilter runs={runs} value={statusFilter} onChange={setStatusFilter} />
      )}
      {expanded && (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow className="hover:bg-transparent focus-within:bg-transparent">
                <TableHead>{t("automations:runsColumnTrigger")}</TableHead>
                <TableHead>{t("automations:runsColumnStatus")}</TableHead>
                <TableHead>{t("automations:outcome")}</TableHead>
                <TableHead>{t("automations:runsColumnTime")}</TableHead>
                <TableHead className="w-8">
                  {visibleRuns.length > 0 && (
                    <DeleteAllButton
                      disabled={loading || deleting}
                      statusFilter={statusFilter}
                      onConfirm={() => {
                        if (statusFilter === "all") {
                          deleteAllRuns();
                        } else {
                          deleteAllRuns(visibleRuns.map((run) => run.id));
                        }
                      }}
                    />
                  )}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visibleRuns.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground py-4">
                    {loading ? t("automations:loading") : emptyMessage}
                  </TableCell>
                </TableRow>
              ) : (
                visibleRuns.map((run) => (
                  <RunRow
                    key={run.id}
                    run={run}
                    deleting={deleting}
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
