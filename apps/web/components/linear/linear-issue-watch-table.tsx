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
import { useAppStore } from "@/components/state-provider";
import type { LinearIssueWatch, LinearSearchFilter } from "@/lib/types/linear";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatRelative } from "@/lib/i18n/formats";

type LinearIssueWatchTableProps = {
  watches: LinearIssueWatch[];
  dirtyIds: ReadonlySet<string>;
  // showWorkspace renders a Workspace column when the table aggregates rows
  // from every workspace (install-wide settings page).
  showWorkspace?: boolean;
  onEdit: (watch: LinearIssueWatch) => void;
  onDelete: (id: string) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onToggleEnabled: (watch: LinearIssueWatch) => void;
};

// `t` is threaded in rather than read from a hook: this is a plain function, and
// the guard never inspects a return value, so a literal here would survive lint.
//
// The bucket copy now comes from `formatRelative`, which owns the same
// just-now/m/h/d ladder for every surface and reads the active app locale. Two
// consequences, both deliberate and matching the Jira table: the first bucket
// reads "just now" rather than "Just now" (it is shared with the rest of the
// app), and an unparseable timestamp renders empty instead of "NaNm ago".
function formatLastPolled(t: TFunction, dateStr?: string | null): string {
  if (!dateStr) return t("linear:never");
  return formatRelative(dateStr);
}

// summarizeFilter renders the structured filter as a short tag-style label
// the user can scan at a glance. Falls back to "(any)" when somehow empty —
// the backend rejects empty filters at create/update time, so this is just
// defensive for forward-compat.
//
// Deliberately NOT translated. This is a filter *expression*, not prose: the
// `team:` / `state:` / `assigned:` / `q:` prefixes are Linear filter field
// names, the values are user data (a team key, a free-text query), and it is
// rendered in a monospace column. It is the direct analogue of the Jira watch
// table's column, which shows the raw JQL verbatim for the same reason.
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
  isDirty,
  onToggleEnabled,
  onTrigger,
  onReset,
  onDelete,
}: {
  watch: LinearIssueWatch;
  isDirty: boolean;
  onToggleEnabled: (watch: LinearIssueWatch) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-end gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            data-settings-dirty={isDirty}
            data-testid={`linear-watch-enabled-${watch.id}`}
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
        <TooltipContent>{watch.enabled ? t("linear:pause") : t("linear:enable")}</TooltipContent>
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
        <TooltipContent>{t("linear:checkNow")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            data-testid="watch-reset-button"
            aria-label={t("linear:resetWatch")}
            onClick={(e) => {
              e.stopPropagation();
              onReset(watch.id);
            }}
          >
            <IconRestore className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("common:reset")}</TooltipContent>
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
        <TooltipContent>{t("linear:delete")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

export function LinearIssueWatchTable({
  watches,
  dirtyIds,
  showWorkspace,
  onEdit,
  onDelete,
  onTrigger,
  onReset,
  onToggleEnabled,
}: LinearIssueWatchTableProps) {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const workspaceName = (id: string) => workspaces.find((w) => w.id === id)?.name ?? id;

  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        {t("linear:noLinearWatchersConfiguredCreateOne")}
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {showWorkspace && <TableHead>{t("common:workspace")}</TableHead>}
          <TableHead>{t("linear:filter")}</TableHead>
          <TableHead>{t("linear:interval")}</TableHead>
          <TableHead>{t("linear:lastPolled")}</TableHead>
          <TableHead>{t("common:status")}</TableHead>
          <TableHead className="text-right">{t("linear:actions")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {watches.map((watch) => (
          <TableRow
            key={watch.id}
            className="cursor-pointer"
            data-settings-dirty={dirtyIds.has(watch.id)}
            data-settings-dirty-level="container"
            data-testid={`linear-watch-row-${watch.id}`}
            onClick={() => onEdit(watch)}
          >
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
              {t("linear:intervalMinutes", {
                count: Math.round(watch.pollIntervalSeconds / 60),
              })}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLastPolled(t, watch.lastPolledAt)}
            </TableCell>
            <TableCell>
              <Badge variant={watch.enabled ? "default" : "secondary"} className="text-xs">
                {watch.enabled ? t("linear:active") : t("linear:paused")}
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
