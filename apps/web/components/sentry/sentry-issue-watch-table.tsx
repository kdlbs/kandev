"use client";

import {
  IconTrash,
  IconRefresh,
  IconPlayerPlay,
  IconPlayerPause,
  IconRestore,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { SentryIssueWatch, SentrySearchFilter } from "@/lib/types/sentry";

type SentryIssueWatchTableProps = {
  watches: SentryIssueWatch[];
  dirtyIds: ReadonlySet<string>;
  // instanceName resolves a watch's bound sentryInstanceId to its display name.
  // The table is workspace-scoped, so it shows which instance each watch polls
  // (not which workspace it belongs to).
  instanceName: (sentryInstanceId: string) => string;
  onEdit: (watch: SentryIssueWatch) => void;
  onDelete: (id: string, workspaceId: string) => void;
  onTrigger: (id: string, workspaceId: string) => void;
  onReset: (id: string, workspaceId: string) => void;
  onToggleEnabled: (watch: SentryIssueWatch) => void;
};

function formatLastPolled(dateStr?: string | null): string {
  if (!dateStr) return "Never";
  const ts = new Date(dateStr).getTime();
  if (Number.isNaN(ts)) return "Never";
  const diff = Date.now() - ts;
  const minutes = Math.floor(diff / 60000);
  if (minutes < 1) return "Just now";
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

// summarizeFilter renders the structured filter as a short tag-style label
// the user can scan at a glance.
function summarizeFilter(filter: SentrySearchFilter | undefined): string {
  if (!filter) return "(any)";
  const parts: string[] = [];
  if (filter.orgSlug) parts.push(`org:${filter.orgSlug}`);
  if (filter.projectSlug) parts.push(`project:${filter.projectSlug}`);
  if (filter.environment) parts.push(`env:${filter.environment}`);
  if (filter.levels && filter.levels.length > 0) {
    parts.push(`level:${filter.levels.join(",")}`);
  }
  if (filter.statuses && filter.statuses.length > 0) {
    parts.push(`status:${filter.statuses.join(",")}`);
  }
  if (filter.statsPeriod) parts.push(`period:${filter.statsPeriod}`);
  if (filter.query) parts.push(`q:"${filter.query}"`);
  return parts.length === 0 ? "(any)" : parts.join(" · ");
}

function WatchActions({
  watch,
  isDirty,
  onToggleEnabled,
  onTrigger,
  onReset,
  onDelete,
}: {
  watch: SentryIssueWatch;
  isDirty: boolean;
  onToggleEnabled: (watch: SentryIssueWatch) => void;
  onTrigger: (id: string, workspaceId: string) => void;
  onReset: (id: string, workspaceId: string) => void;
  onDelete: (id: string, workspaceId: string) => void;
}) {
  return (
    <div className="flex items-center justify-end gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            data-settings-dirty={isDirty}
            data-testid={`sentry-watch-enabled-${watch.id}`}
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
              onTrigger(watch.id, watch.workspaceId);
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
            className="h-7 w-7 p-0 cursor-pointer"
            data-testid="watch-reset-button"
            aria-label="Reset watch"
            onClick={(e) => {
              e.stopPropagation();
              onReset(watch.id, watch.workspaceId);
            }}
          >
            <IconRestore className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Reset</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 text-red-500 hover:text-red-600 cursor-pointer"
            onClick={(e) => {
              e.stopPropagation();
              onDelete(watch.id, watch.workspaceId);
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

export function SentryIssueWatchTable({
  watches,
  dirtyIds,
  instanceName,
  onEdit,
  onDelete,
  onTrigger,
  onReset,
  onToggleEnabled,
}: SentryIssueWatchTableProps) {
  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No Sentry watchers configured. Create one to auto-create tasks from filtered issues.
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Instance</TableHead>
          <TableHead>Filter</TableHead>
          <TableHead>Interval</TableHead>
          <TableHead>Last Polled</TableHead>
          <TableHead>Status</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {watches.map((watch) => (
          <TableRow
            key={watch.id}
            className="cursor-pointer"
            data-settings-dirty={dirtyIds.has(watch.id)}
            data-settings-dirty-level="container"
            data-testid={`sentry-watch-row-${watch.id}`}
            onClick={() => onEdit(watch)}
          >
            <TableCell className="text-xs text-muted-foreground" data-testid="watch-instance">
              {instanceName(watch.sentryInstanceId)}
            </TableCell>
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
                isDirty={dirtyIds.has(watch.id)}
                onToggleEnabled={onToggleEnabled}
                onTrigger={onTrigger}
                onReset={onReset}
                onDelete={onDelete}
              />
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}
