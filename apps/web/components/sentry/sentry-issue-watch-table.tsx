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
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatRelative } from "@/lib/i18n/formats";

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

// `t` is threaded in rather than read from a hook: this is a plain function, and
// the guard never inspects a return value, so a literal here would survive lint.
//
// The bucket copy now comes from `formatRelative`, which owns the same
// just-now/m/h/d ladder for every surface and reads the active app locale. Two
// consequences, both deliberate and matching the Linear and Jira tables: the
// first bucket reads "just now" rather than "Just now" (it is shared with the
// rest of the app), and an unparseable timestamp renders empty instead of
// "NaNm ago".
function formatLastPolled(t: TFunction, dateStr?: string | null): string {
  if (!dateStr) return t("sentry:never");
  return formatRelative(dateStr);
}

// summarizeFilter renders the structured filter as a short tag-style label
// the user can scan at a glance.
//
// Deliberately NOT translated. This is a filter *expression*, not prose: the
// `org:` / `project:` / `env:` / `level:` / `status:` / `period:` / `q:` prefixes
// mirror Sentry's own search syntax, every value is either a slug the user typed
// or a `SentryLevel` / `SentryStatus` union member that must stay byte-identical
// to what is on the wire, and the column is rendered in monospace. Same call as
// the Linear watch table's summary and the Jira table's raw JQL. "(any)" is the
// empty state of that expression, so it stays with it.
function summarizeFilter(filter: SentrySearchFilter | undefined): string {
  if (!filter) return "(any)";
  const parts: string[] = [];
  if (filter.orgSlug) parts.push(`org:${filter.orgSlug}`);
  if (filter.projectSlugs && filter.projectSlugs.length > 0) {
    parts.push(`project:${filter.projectSlugs.join(",")}`);
  }
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
        <TooltipContent>{watch.enabled ? t("sentry:pause") : t("sentry:enable")}</TooltipContent>
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
        <TooltipContent>{t("sentry:checkNow")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            data-testid="watch-reset-button"
            aria-label={t("sentry:resetWatch")}
            onClick={(e) => {
              e.stopPropagation();
              onReset(watch.id, watch.workspaceId);
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
              onDelete(watch.id, watch.workspaceId);
            }}
          >
            <IconTrash className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("sentry:delete")}</TooltipContent>
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
  const { t } = useTranslation();
  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        {t("sentry:noWatchersConfigured")}
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t("sentry:instance")}</TableHead>
          <TableHead>{t("sentry:filter")}</TableHead>
          <TableHead>{t("sentry:interval")}</TableHead>
          <TableHead>{t("sentry:lastPolled")}</TableHead>
          <TableHead>{t("common:status")}</TableHead>
          <TableHead className="text-right">{t("sentry:actions")}</TableHead>
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
              {t("sentry:intervalMinutes", {
                count: Math.round(watch.pollIntervalSeconds / 60),
              })}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLastPolled(t, watch.lastPolledAt)}
            </TableCell>
            <TableCell>
              <Badge variant={watch.enabled ? "default" : "secondary"} className="text-xs">
                {watch.enabled ? t("sentry:active") : t("sentry:paused")}
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
