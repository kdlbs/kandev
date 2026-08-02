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
import { useToast } from "@/components/toast-provider";
import { useAppStore } from "@/components/state-provider";
import type { IssueWatch } from "@/lib/types/github";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { formatRelative } from "@/lib/i18n/formats";

type IssueWatchTableProps = {
  watches: IssueWatch[];
  // showWorkspace renders a Workspace column when the table aggregates rows
  // from every workspace (install-wide settings page).
  showWorkspace?: boolean;
  onEdit: (watch: IssueWatch) => void;
  onDelete: (id: string) => void;
  onTrigger: (id: string) => void;
  onReset: (id: string) => void;
  onToggleEnabled: (watch: IssueWatch) => void;
};

// `formatRelative` already implements these exact buckets (just now / Nm / Nh /
// Nd ago) against the catalog, so the local copy is dropped rather than
// translated — it was English-only and duplicated the shared helper.
function formatLastPolled(t: TFunction, dateStr: string | null): string {
  if (!dateStr) return t("github:never");
  return formatRelative(dateStr);
}

function formatRepoNames(t: TFunction, watch: IssueWatch): string {
  if (watch.repos.length === 0) return t("github:allRepositories");
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
  onReset: (id: string) => void;
  onDelete: (id: string) => void;
};

function WatchActions({ watch, onToggleEnabled, onTrigger, onReset, onDelete }: WatchActionsProps) {
  const { t } = useTranslation();
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
        <TooltipContent>{watch.enabled ? t("github:pause") : t("github:enable")}</TooltipContent>
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
              toast({ description: t("github:checkingForNewIssues") });
            }}
          >
            <IconRefresh className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("github:checkNow")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 cursor-pointer"
            data-testid="watch-reset-button"
            aria-label={t("github:resetWatch")}
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
        <TooltipContent>{t("github:delete")}</TooltipContent>
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
  onReset,
  onToggleEnabled,
}: IssueWatchTableProps) {
  const { t } = useTranslation();
  const workspaces = useAppStore((s) => s.workspaces.items);
  const workspaceName = (id: string) => workspaces.find((w) => w.id === id)?.name ?? id;

  if (watches.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        {t("github:noIssueWatchesConfiguredCreateOne")}
      </p>
    );
  }

  return (
    <Table>
      <TableHeader>
        <TableRow>
          {showWorkspace && <TableHead>{t("common:workspace")}</TableHead>}
          <TableHead>{t("github:repository")}</TableHead>
          <TableHead>{t("github:labels")}</TableHead>
          <TableHead>{t("github:interval")}</TableHead>
          <TableHead>{t("github:lastPolled")}</TableHead>
          <TableHead>{t("common:status")}</TableHead>
          <TableHead className="text-right">{t("github:actions")}</TableHead>
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
            <TableCell className="font-medium">{formatRepoNames(t, watch)}</TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLabels(watch) || "—"}
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {Math.round(watch.poll_interval_seconds / 60)}m
            </TableCell>
            <TableCell className="text-xs text-muted-foreground">
              {formatLastPolled(t, watch.last_polled_at)}
            </TableCell>
            <TableCell>
              <Badge variant={watch.enabled ? "default" : "secondary"} className="text-xs">
                {watch.enabled ? t("github:active") : t("github:paused")}
              </Badge>
            </TableCell>
            <TableCell className="text-right">
              <WatchActions
                watch={watch}
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
