"use client";

import { IconRefresh, IconPlayerPlay, IconPlayerPause, IconRestore } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useAppStore } from "@/components/state-provider";
import type { JiraIssueWatch } from "@/lib/types/jira";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatRelative } from "@/lib/i18n/formats";
import { WatcherDeleteAction } from "@/components/watches/watcher-delete-action";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

type JiraIssueWatchTableProps = {
  watches: JiraIssueWatch[];
  dirtyIds: ReadonlySet<string>;
  // showWorkspace renders a Workspace column when the table aggregates rows
  // from every workspace (install-wide settings page). Off for the legacy
  // single-workspace surfaces.
  showWorkspace?: boolean;
  onEdit: (watch: JiraIssueWatch) => void;
  onDelete: (id: string) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onToggleEnabled: (watch: JiraIssueWatch) => void;
};

// `t` is threaded in rather than read from a hook: this is a plain function, and
// the guard never inspects a return value, so a literal here would survive lint.
//
// The bucket copy now comes from `formatRelative`, which owns the same
// just-now/m/h/d ladder for every surface and reads the active app locale. Two
// consequences, both deliberate: the first bucket reads "just now" rather than
// "Just now" (it is shared with the rest of the app), and an unparseable
// timestamp renders empty instead of "NaNm ago".
function formatLastPolled(t: TFunction, dateStr?: string | null): string {
  if (!dateStr) return t("jira:never");
  return formatRelative(dateStr);
}

function WatchActions({
  watch,
  isDirty,
  onToggleEnabled,
  onTrigger,
  onReset,
  onDelete,
}: {
  watch: JiraIssueWatch;
  isDirty: boolean;
  onToggleEnabled: (watch: JiraIssueWatch) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const { t } = useTranslation();
  const { isFinePointer } = useResponsiveBreakpoint();
  const actionSize = isFinePointer ? "h-7 w-7" : "h-11 w-11";
  return (
    <div
      className="flex items-center justify-end gap-1"
      data-watcher-actions
      onClick={(event) => event.stopPropagation()}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={`${actionSize} p-0 cursor-pointer`}
            data-settings-dirty={isDirty}
            data-testid={`jira-watch-enabled-${watch.id}`}
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
        <TooltipContent>{watch.enabled ? t("jira:pause") : t("jira:enable")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={`${actionSize} p-0 cursor-pointer`}
            onClick={(e) => {
              e.stopPropagation();
              // Toast is fired by the parent's wrappedTrigger on completion —
              // the inline "Checking…" toast it used to fire here was a duplicate.
              onTrigger(watch.id);
            }}
          >
            <IconRefresh className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("jira:checkNow")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={`${actionSize} p-0 cursor-pointer`}
            data-testid="watch-reset-button"
            aria-label={t("jira:resetWatch")}
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
      <WatcherDeleteAction
        title={t("jira:deleteThisJiraWatcher")}
        cancelLabel={t("common:cancel")}
        confirmLabel={t("jira:delete")}
        ariaLabel={t("jira:deleteThisJiraWatcher")}
        tooltipLabel={t("jira:delete")}
        triggerTestId={`jira-watch-delete-${watch.id}`}
        confirmTestId="jira-watch-delete-confirm"
        onConfirm={() => onDelete(watch.id)}
      />
    </div>
  );
}

function MobileWatchCard({
  watch,
  isDirty,
  showWorkspace,
  workspaceName,
  onEdit,
  onToggleEnabled,
  onTrigger,
  onReset,
  onDelete,
}: {
  watch: JiraIssueWatch;
  isDirty: boolean;
  showWorkspace?: boolean;
  workspaceName: (id: string) => string;
  onEdit: (watch: JiraIssueWatch) => void;
  onToggleEnabled: (watch: JiraIssueWatch) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onDelete: (id: string) => void;
}) {
  const { t } = useTranslation();
  const workspaceLabel = showWorkspace ? workspaceName(watch.workspaceId) : null;
  return (
    <div
      className="min-w-0 space-y-3 border-b px-1 py-4 last:border-b-0"
      data-settings-dirty={isDirty}
      data-testid={`jira-watch-mobile-row-${watch.id}`}
    >
      <button
        type="button"
        className="block w-full min-w-0 text-left cursor-pointer"
        onClick={() => onEdit(watch)}
      >
        {workspaceLabel && (
          <span
            className="mb-1 block truncate text-xs text-muted-foreground"
            title={workspaceLabel}
          >
            {workspaceLabel}
          </span>
        )}
        <span className="block truncate font-mono text-sm" title={watch.jql}>
          {watch.jql}
        </span>
        <span className="mt-1 flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Badge variant={watch.enabled ? "default" : "secondary"}>
            {watch.enabled ? t("jira:active") : t("jira:paused")}
          </Badge>
          <span>
            {t("jira:intervalMinutes", {
              count: Math.round(watch.pollIntervalSeconds / 60),
            })}
          </span>
          <span>{formatLastPolled(t, watch.lastPolledAt)}</span>
        </span>
      </button>
      <WatchActions
        watch={watch}
        isDirty={isDirty}
        onToggleEnabled={onToggleEnabled}
        onTrigger={onTrigger}
        onReset={onReset}
        onDelete={onDelete}
      />
    </div>
  );
}

function DesktopJiraIssueWatchTable({
  watches,
  dirtyIds,
  showWorkspace,
  onEdit,
  onDelete,
  onTrigger,
  onReset,
  onToggleEnabled,
  workspaceName,
}: JiraIssueWatchTableProps & { workspaceName: (id: string) => string }) {
  const { t } = useTranslation();
  return (
    <Table className="min-w-[680px]">
      <TableHeader>
        <TableRow>
          {showWorkspace && <TableHead>{t("common:workspace")}</TableHead>}
          <TableHead>JQL</TableHead>
          <TableHead>{t("jira:interval")}</TableHead>
          <TableHead>{t("jira:lastPolled")}</TableHead>
          <TableHead>{t("common:status")}</TableHead>
          <TableHead className="text-right">{t("jira:actions")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {watches.map((watch) => (
          <TableRow
            key={watch.id}
            className="cursor-pointer"
            data-settings-dirty={dirtyIds.has(watch.id)}
            data-settings-dirty-level="container"
            data-testid={`jira-watch-row-${watch.id}`}
            onClick={(event) => {
              if (
                event.target instanceof Element &&
                event.target.closest("[data-watcher-actions]")
              ) {
                return;
              }
              onEdit(watch);
            }}
          >
            {showWorkspace && (
              <TableCell className="text-xs text-muted-foreground">
                {workspaceName(watch.workspaceId)}
              </TableCell>
            )}
            <TableCell className="font-mono text-xs max-w-md truncate" title={watch.jql}>
              {watch.jql}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {t("jira:intervalMinutes", {
                count: Math.round(watch.pollIntervalSeconds / 60),
              })}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLastPolled(t, watch.lastPolledAt)}
            </TableCell>
            <TableCell>
              <Badge variant={watch.enabled ? "default" : "secondary"} className="text-xs">
                {watch.enabled ? t("jira:active") : t("jira:paused")}
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

export function JiraIssueWatchTable({
  watches,
  dirtyIds,
  showWorkspace,
  onEdit,
  onDelete,
  onTrigger,
  onReset,
  onToggleEnabled,
}: JiraIssueWatchTableProps) {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const { usesDesktopWorkbench = true } = useResponsiveBreakpoint();
  const workspaceName = (id: string) => workspaces.find((w) => w.id === id)?.name ?? id;

  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        {t("jira:noJiraWatchersConfiguredCreateOne")}
      </p>
    );
  }

  if (!usesDesktopWorkbench) {
    return (
      <div className="min-w-0" data-testid="jira-watch-mobile-list">
        {watches.map((watch) => (
          <MobileWatchCard
            key={watch.id}
            watch={watch}
            isDirty={dirtyIds.has(watch.id)}
            showWorkspace={showWorkspace}
            workspaceName={workspaceName}
            onEdit={onEdit}
            onToggleEnabled={onToggleEnabled}
            onTrigger={onTrigger}
            onReset={onReset}
            onDelete={onDelete}
          />
        ))}
      </div>
    );
  }

  return (
    <DesktopJiraIssueWatchTable
      watches={watches}
      dirtyIds={dirtyIds}
      showWorkspace={showWorkspace}
      onEdit={onEdit}
      onDelete={onDelete}
      onTrigger={onTrigger}
      onReset={onReset}
      onToggleEnabled={onToggleEnabled}
      workspaceName={workspaceName}
    />
  );
}
