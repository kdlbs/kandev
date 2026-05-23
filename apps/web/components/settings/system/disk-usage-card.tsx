"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Badge } from "@kandev/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconDatabase, IconRefresh, IconAlertTriangle, IconFolderOpen } from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useDiskUsage } from "@/hooks/domains/system/use-disk-usage";
import { openDataFolder } from "@/lib/api/domains/system-api";
import type { DiskBreakdown } from "@/lib/types/system";
import { formatBytes } from "@/lib/utils/format-bytes";
import { JobProgressIndicator } from "./job-progress-indicator";

const REFRESH_HELP =
  "Walks every directory inside the data folder and adds up the file sizes. Results are cached for a couple of hours; click to compute fresh numbers right now. Can take a few seconds on large workspaces.";

type Row = {
  key: keyof Omit<DiskBreakdown, "warnings" | "computed_at" | "total">;
  label: string;
};

const ROWS: Row[] = [
  { key: "data_dir", label: "Data directory" },
  { key: "worktrees", label: "Worktrees" },
  { key: "repos", label: "Repositories" },
  { key: "sessions", label: "Sessions" },
  { key: "tasks", label: "Tasks" },
  { key: "quick_chat", label: "Quick chat" },
  { key: "backups", label: "Backups" },
];

function formatComputedAt(iso: string): string {
  const parsed = new Date(iso);
  if (Number.isNaN(parsed.getTime())) return iso;
  return parsed.toLocaleString();
}

function HomeDirRow({ homeDir }: { homeDir: string }) {
  return (
    <div
      className="mb-3 flex items-center justify-between gap-2 rounded-md border bg-muted/30 px-3 py-2"
      data-testid="system-disk-usage-home-dir"
    >
      <div className="min-w-0 flex-1">
        <p className="text-[11px] uppercase tracking-wide text-muted-foreground">Data directory</p>
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
        Open
      </Button>
    </div>
  );
}

function WarningsBlock({ warnings }: { warnings: string[] }) {
  if (warnings.length === 0) return null;
  return (
    <div
      className="rounded-md border border-amber-500/30 bg-amber-500/5 p-2 text-xs text-amber-700 dark:text-amber-400 flex items-start gap-2"
      data-testid="system-disk-usage-warnings"
    >
      <IconAlertTriangle className="h-3.5 w-3.5 shrink-0 mt-0.5" />
      <div>
        <div className="font-medium">Some directories could not be measured:</div>
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
  return (
    <Table data-testid="system-disk-usage-table">
      <TableHeader>
        <TableRow>
          <TableHead>Path</TableHead>
          <TableHead className="text-right">Size</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {ROWS.map((row) => (
          <TableRow key={row.key} data-testid={`system-disk-usage-row-${row.key}`}>
            <TableCell>{row.label}</TableCell>
            <TableCell className="text-right tabular-nums">{formatBytes(data[row.key])}</TableCell>
          </TableRow>
        ))}
        <TableRow className="font-semibold">
          <TableCell>Total</TableCell>
          <TableCell className="text-right tabular-nums" data-testid="system-disk-usage-total">
            {formatBytes(data.total)}
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  );
}

export function DiskUsageCard() {
  const { diskUsage, isLoading, error, refresh } = useDiskUsage();
  const data = diskUsage?.data ?? null;
  const computing = diskUsage?.computing ?? false;
  const homeDir = diskUsage?.home_dir ?? "";

  return (
    <Card data-testid="system-disk-usage-card">
      <CardHeader className="flex flex-row items-center justify-between space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconDatabase className="h-4 w-4" />
          Disk Usage
          {computing && data && (
            <Badge variant="outline" className="text-[10px]">
              Refreshing...
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
                disabled={isLoading}
                onClick={() => void refresh()}
                className="cursor-pointer"
                data-testid="system-disk-usage-refresh"
              >
                <IconRefresh className="h-3.5 w-3.5 mr-1" />
                Refresh
              </Button>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">{REFRESH_HELP}</TooltipContent>
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
            Calculating...
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
              Computed at {formatComputedAt(data.computed_at)}
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
