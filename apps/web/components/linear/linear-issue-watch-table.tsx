"use client";

import { IconTrash, IconRefresh, IconPlayerPlay, IconPlayerPause } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { LinearIssueWatch, LinearSearchFilter } from "@/lib/types/linear";

type LinearIssueWatchTableProps = {
  watches: LinearIssueWatch[];
  // showWorkspace renders a Workspace column when the table aggregates rows
  // from every workspace (install-wide settings page).
  showWorkspace?: boolean;
  onEdit: (watch: LinearIssueWatch) => void;
  onDelete: (id: string) => void;
  onTrigger: (id: string) => void;
  onToggleEnabled: (watch: LinearIssueWatch) => void;
};

function formatLastPolled(dateStr?: string | null): string {
  if (!dateStr) return "Never";
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

// summarizeFilter renders the structured filter as a short tag-style label
// the user can scan at a glance. Falls back to "(any)" when somehow empty —
// the backend rejects empty filters at create/update time, so this is just
// defensive for forward-compat.
function summarizeFilter(filter: LinearSearchFilter | undefined): string {
  if (!filter) return "(any)";
  const parts: string[] = [];
  if (filter.teamKey) parts.push(`team:${filter.teamKey}`);
  if (filter.stateIds && filter.stateIds.length > 0) {
    parts.push(`state:${filter.stateIds.length} selected`);
  }
  if (filter.assigned) parts.push(`assigned:${filter.assigned}`);
  if (filter.query) parts.push(`q:"${filter.query}"`);
  return parts.length === 0 ? "(any)" : parts.join(" · ");
}

function WatchActions({
  watch,
  onToggleEnabled,
  onTrigger,
  onDelete,
}: {
  watch: LinearIssueWatch;
  onToggleEnabled: (watch: LinearIssueWatch) => void;
  onTrigger: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  return (
    <div className="flex items-center justify-end gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onToggleEnabled(watch);
            }}
          >
            {watch.enabled ? (
              <IconPlayerPause className="h-3.5 w-3.5" />
            ) : (
              <IconPlayerPlay className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{watch.enabled ? "Pause" : "Enable"}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onTrigger(watch.id);
            }}
          >
            <IconRefresh className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Check now</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 text-red-500 hover:text-red-600 cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onDelete(watch.id);
            }}
          >
            <IconTrash className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Delete</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function LinearIssueWatchTable({
  watches,
  showWorkspace,
  onEdit,
  onDelete,
  onTrigger,
  onToggleEnabled,
}: LinearIssueWatchTableProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const workspaceName = (id: string) => workspaces.find((w) => w.id === id)?.name ?? id;

  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No Linear watchers configured. Create one to auto-create tasks from filtered issues.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {showWorkspace && <TableHead>Workspace</TableHead>}
          <TableHead>Filter</TableHead>
          <TableHead>Interval</TableHead>
          <TableHead>Last Polled</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {watches.map((watch) => (
          <TableRow key={watch.id} className="cursor-pointer" onClick={() => onEdit(watch)}>
            {showWorkspace && (
              <TableCell className="text-xs text-muted-foreground">
                {workspaceName(watch.workspaceId)}
              </TableCell>
            )}
            <TableCell
              className="font-mono text-xs max-w-md truncate"
              title={summarizeFilter(watch.filter)}
            >
              {summarizeFilter(watch.filter)}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {Math.round(watch.pollIntervalSeconds / 60)}m
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLastPolled(watch.lastPolledAt)}
            </TableCell>
            <TableCell>
              <Badge variant={watch.enabled ? "default" : "secondary"} className="text-xs">
                {watch.enabled ? "Active" : "Paused"}
              </Badge>
            </TableCell>
            <TableCell className="text-right">
              <WatchActions
                watch={watch}
                onToggleEnabled={onToggleEnabled}
                onTrigger={onTrigger}
                onDelete={onDelete}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
