"use client";

import { useTranslation } from "react-i18next";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconDatabase, IconRefresh, IconAlertTriangle, IconFolderOpen } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useDiskUsage } from "@/hooks/domains/system/use-disk-usage";
import { openDataFolder } from "@/lib/api/domains/system-api";
import { useActionFeedback } from "@/hooks/use-action-feedback";
import { formatDateTime } from "@/lib/i18n/formats";
import type { DiskBreakdown } from "@/lib/types/system";
import { formatBytes } from "@/lib/utils/format-bytes";
import { ActionButtonContent } from "./action-button-content";
import { JobProgressIndicator } from "./job-progress-indicator";

type Row = {
  key: keyof Omit<DiskBreakdown, "warnings" | "computed_at" | "total">;
  labelKey: string;
};

/**
 * The breakdown `key` is the wire field name and stays a value; only
 * `labelKey` is copy, and it resolves at render so the table follows a locale
 * switch. Assigning the resolved labels here instead would freeze them at the
 * boot locale — and `mode: "jsx-only"` skips SCREAMING_CASE identifiers, so
 * lint would never have reported it.
 */
const ROWS: Row[] = [
  { key: "data_dir", labelKey: "system:diskRowDataDir" },
  { key: "worktrees", labelKey: "system:diskRowWorktrees" },
  { key: "repos", labelKey: "system:diskRowRepos" },
  { key: "sessions", labelKey: "system:diskRowSessions" },
  { key: "tasks", labelKey: "system:diskRowTasks" },
  { key: "quick_chat", labelKey: "system:diskRowQuickChat" },
  { key: "backups", labelKey: "system:diskRowBackups" },
];

function formatComputedAt(iso: string): string {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return iso;
  return formatDateTime(parsed);
}

function HomeDirRow({ homeDir }: { homeDir: string }) {
  const { t } = useTranslation();
  return (
    <div
      className="mb-3 flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2"
      data-testid="system-disk-usage-home-dir"
    >
      <div className="min-w-0 flex-1">
        <p className="text-[11px] uppercase tracking-wide text-muted-foreground">
          {t("system:diskRowDataDir")}
        </p>
        {/* The resolved data-folder path is a value. */}
        <p className="text-xs font-mono break-all">{homeDir}</p>
      </div>
      <Button
        variant="outline"
        size="sm"
        onClick={() => {
          void openDataFolder().catch(() => {});
        }}
        className="cursor-pointer shrink-0"
        data-testid="system-disk-usage-open"
      >
        <IconFolderOpen className="h-3.5 w-3.5 mr-1" />
        {t("system:diskOpen")}
      </Button>
    </div>
  );
}

function WarningsBlock({ warnings }: { warnings: string[] }) {
  const { t } = useTranslation();
  if (warnings.length === 0) return null;
  return (
    <div
      className="rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-400 flex items-start gap-2"
      data-testid="system-disk-usage-warnings"
    >
      <IconAlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
      <div>
        <div className="font-medium">{t("system:diskUnmeasuredDirs")}</div>
        {/* Each warning is a backend-rendered diagnostic and stays as sent. */}
        <ul className="list-disc pl-4 mt-1">
          {warnings.map((w, i) => (
            <li key={i}>{w}</li>
          ))}
        </ul>
      </div>
    </div>
  );
}

function BreakdownTable({ data }: { data: DiskBreakdown }) {
  const { t } = useTranslation();
  return (
    <Table data-testid="system-disk-usage-table">
      <TableHeader>
        <TableRow>
          <TableHead>{t("system:diskColumnPath")}</TableHead>
          <TableHead className="text-right">{t("system:diskColumnSize")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {ROWS.map((row) => (
          <TableRow key={row.key} data-testid={`system-disk-usage-row-${row.key}`}>
            <TableCell>{t(row.labelKey)}</TableCell>
            <TableCell className="text-right tabular-nums">{formatBytes(data[row.key])}</TableCell>
          </TableRow>
        ))}
        <TableRow className="font-semibold">
          <TableCell>{t("system:diskTotal")}</TableCell>
          <TableCell className="text-right tabular-nums" data-testid="system-disk-usage-total">
            {formatBytes(data.total)}
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  );
}

export function DiskUsageCard() {
  const { t } = useTranslation();
  const { diskUsage, isLoading, error, refresh } = useDiskUsage();
  const refreshFeedback = useActionFeedback();
  const data = diskUsage?.data ?? null;
  const computing = diskUsage?.computing ?? false;
  const homeDir = diskUsage?.home_dir ?? "";

  const onRefresh = () =>
    void refreshFeedback.run(async () => {
      await refresh();
    });

  return (
    <Card data-testid="system-disk-usage-card">
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconDatabase className="h-4 w-4" />
          {t("system:diskUsageTitle")}
          {computing && data && (
            <Badge variant="outline" className="text-[10px]">
              {t("system:diskRefreshing")}
            </Badge>
          )}
        </CardTitle>
        <div className="flex items-center gap-2">
          <JobProgressIndicator kind="disk-walk" />
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={isLoading || refreshFeedback.state === "pending"}
                onClick={onRefresh}
                className="cursor-pointer min-w-[6rem] justify-center"
                data-testid="system-disk-usage-refresh"
                data-state={refreshFeedback.state}
              >
                <ActionButtonContent
                  state={refreshFeedback.state}
                  idleIcon={<IconRefresh className="h-3.5 w-3.5 mr-1" />}
                  idleLabel={t("system:diskRefresh")}
                  pendingLabel={t("system:diskRefreshing")}
                  successLabel={t("system:diskRefreshed")}
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">{t("system:diskRefreshHelp")}</TooltipContent>
          </Tooltip>
        </div>
      </CardHeader>
      <CardContent>
        {homeDir && <HomeDirRow homeDir={homeDir} />}
        {error && (
          <p className="text-xs text-red-500" data-testid="system-disk-usage-error">
            {error}
          </p>
        )}
        {!data && (
          <div
            className="flex items-center gap-2 text-sm text-muted-foreground py-4"
            data-testid="system-disk-usage-spinner"
          >
            <Spinner className="size-4" />
            {t("system:diskCalculating")}
          </div>
        )}
        {data && (
          <div className="space-y-3">
            <BreakdownTable data={data} />
            <WarningsBlock warnings={data.warnings ?? []} />
            <p
              className="text-xs text-muted-foreground"
              data-testid="system-disk-usage-computed-at"
            >
              {t("system:diskComputedAt", { at: formatComputedAt(data.computed_at) })}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
