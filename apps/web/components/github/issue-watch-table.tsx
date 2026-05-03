"use client";

import { IconTrash, IconRefresh, IconPlayerPlay, IconPlayerPause } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import type { IssueWatch } from "@/lib/types/github";

type IssueWatchTableProps = {
  watches: IssueWatch[];
  // showWorkspace renders a Workspace column when the table aggregates rows
  // from every workspace (install-wide settings page).
  showWorkspace?: boolean;
  onEdit: (watch: IssueWatch) => void;
  onDelete: (id: string) => void;
  onTrigger: (id: string) => void;
  onToggleEnabled: (watch: IssueWatch) => void;
};

function formatLastPolled(dateStr: string | null): string {
  if (!dateStr) return "Never";
  const diff = Date.now() - new Date(dateStr).getTime();
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

function formatRepoNames(watch: IssueWatch): string {
  if (watch.repos.length === 0) return "All repositories";
  return watch.repos
    .map((r) => (r.name === "" ? `${r.owner}/*` : `${r.owner}/${r.name}`))
    .join(", ");
}

function formatLabels(watch: IssueWatch): string {
  if (!watch.labels || watch.labels.length === 0) return "";
  return watch.labels.join(", ");
}

type WatchActionsProps = {
  watch: IssueWatch;
  onToggleEnabled: (watch: IssueWatch) => void;
  onTrigger: (id: string) => void;
  onDelete: (id: string) => void;
};

function WatchActions({ watch, onToggleEnabled, onTrigger, onDelete }: WatchActionsProps) {
  const { toast } = useToast();
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
              toast({ description: "Checking for new issues..." });
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

export function IssueWatchTable({
  watches,
  showWorkspace,
  onEdit,
  onDelete,
  onTrigger,
  onToggleEnabled,
}: IssueWatchTableProps) {
  const workspaces = useAppStore((s) => s.workspaces.items);
  const workspaceName = (id: string) => workspaces.find((w) => w.id === id)?.name ?? id;

  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No issue watches configured. Create one to start monitoring GitHub issues.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {showWorkspace && <TableHead>Workspace</TableHead>}
          <TableHead>Repository</TableHead>
          <TableHead>Labels</TableHead>
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
                {workspaceName(watch.workspace_id)}
              </TableCell>
            )}
            <TableCell className="font-medium">{formatRepoNames(watch)}</TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLabels(watch) || "—"}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {Math.round(watch.poll_interval_seconds / 60)}m
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLastPolled(watch.last_polled_at)}
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
