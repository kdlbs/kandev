"use client";

import { useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconChevronDown, IconChevronUp, IconRefresh } from "@tabler/icons-react";
import { useAutomationRuns } from "@/hooks/domains/settings/use-automation-runs";
import type { RunStatus } from "@/lib/types/automation";
import { formatRelativeTime } from "./format-utils";

type RunsSectionProps = {
  automationId: string | null;
};

const STATUS_BADGE: Record<
  RunStatus,
  { variant: "default" | "destructive" | "secondary" | "outline"; label: string }
> = {
  triggered: { variant: "secondary", label: "Triggered" },
  task_created: { variant: "default", label: "Task Created" },
  failed: { variant: "destructive", label: "Failed" },
  skipped: { variant: "outline", label: "Skipped" },
};

export function RunsSection({ automationId }: RunsSectionProps) {
  const [expanded, setExpanded] = useState(false);
  const { runs, loading, refresh } = useAutomationRuns(automationId);

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
          <Button
            variant="ghost"
            size="icon-sm"
            className="cursor-pointer"
            onClick={refresh}
            disabled={loading}
          >
            <IconRefresh className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </Button>
        )}
      </div>
      {expanded && (
        <div className="rounded-md border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Trigger</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Task</TableHead>
                <TableHead>Time</TableHead>
                <TableHead>Error</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={5} className="text-center text-muted-foreground py-4">
                    {loading ? "Loading..." : "No runs yet"}
                  </TableCell>
                </TableRow>
              ) : (
                runs.map((run) => {
                  const badge = STATUS_BADGE[run.status] ?? STATUS_BADGE.triggered;
                  return (
                    <TableRow key={run.id}>
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
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
