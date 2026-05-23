"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import {
  IconDatabase,
  IconBolt,
  IconInfoCircle,
  IconRefresh,
  IconTrash,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useDatabaseStats } from "@/hooks/domains/system/use-database-stats";
import { optimizeDatabase, vacuumDatabase } from "@/lib/api/domains/system-api";
import { formatBytes } from "@/lib/utils/format-bytes";
import { useActionFeedback, type ActionFeedbackState } from "@/hooks/use-action-feedback";
import { ActionButtonContent } from "./action-button-content";
import { JobProgressIndicator } from "./job-progress-indicator";
import { FactoryResetDialog } from "./factory-reset-dialog";

function formatTimestamp(iso: string | null | undefined): string {
  if (!iso) return "Never";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

const WAL_HELP =
  "Write-Ahead Log. SQLite writes new changes to this companion file first and merges them into the main database over time. A non-zero WAL size is normal - it temporarily grows under load and shrinks back during checkpoints. Running vacuum will reset it.";

type Row = { label: string; value: string; testid: string; info?: string };

function StatRow({ label, value, testid, info }: Row) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5 border-b last:border-b-0">
      <span className="text-xs text-muted-foreground inline-flex items-center gap-1">
        {label}
        {info && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={`What is ${label}?`}
                className="cursor-pointer text-muted-foreground/70 hover:text-foreground transition-colors"
                data-testid={`${testid}-info`}
              >
                <IconInfoCircle className="h-3 w-3" />
              </button>
            </TooltipTrigger>
            <TooltipContent className="max-w-xs">{info}</TooltipContent>
          </Tooltip>
        )}
      </span>
      <span className="text-sm font-mono break-all text-right" data-testid={testid}>
        {value}
      </span>
    </div>
  );
}

type DBStats = {
  path: string;
  size_bytes: number;
  wal_size_bytes: number;
  schema_version: string;
  last_backup_at: string | null;
};

function StatsTable({ database }: { database: DBStats }) {
  return (
    <div className="rounded-md border px-3 py-2">
      <StatRow label="Path" value={database.path} testid="system-db-path" />
      <StatRow label="Size" value={formatBytes(database.size_bytes)} testid="system-db-size" />
      <StatRow
        label="WAL"
        value={formatBytes(database.wal_size_bytes)}
        testid="system-db-wal"
        info={WAL_HELP}
      />
      <StatRow
        label="Schema version"
        value={database.schema_version || "-"}
        testid="system-db-schema-version"
      />
      <StatRow
        label="Last backup"
        value={formatTimestamp(database.last_backup_at)}
        testid="system-db-last-backup"
      />
    </div>
  );
}

// Plain-language descriptions of each maintenance operation. Surfaced as a
// short paragraph next to the button + an on-hover tooltip so users without
// database background know what each action does and when to use it.
const VACUUM_HELP =
  "Reclaims unused space inside the database file. The file can grow over time as you delete tasks or sessions; vacuum compacts it back down. Safe to run anytime - it does not change any data. Use it once in a while if the database size looks large.";
const OPTIMIZE_HELP =
  "Asks the database to refresh its internal indexes so common queries stay fast. Run it after deleting a lot of data or if the app feels sluggish. Quick and safe - no data is changed.";
const FACTORY_RESET_HELP =
  "Wipes ALL Kandev data: tasks, sessions, settings, repositories, and worktrees. There is no undo. A backup is taken first, but restoring it requires using the Backups page after the reset. Only use this if you want to start over from scratch.";

function OperationRow({
  testid,
  label,
  description,
  button,
}: {
  testid: string;
  label: string;
  description: string;
  button: React.ReactNode;
}) {
  return (
    <div
      className="flex items-start justify-between gap-3 rounded-md border p-3"
      data-testid={testid}
    >
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-muted-foreground mt-1">{description}</p>
      </div>
      <div className="shrink-0">{button}</div>
    </div>
  );
}

function MaintenanceButtons({
  vacuumState,
  optimizeState,
  onVacuum,
  onOptimize,
  onResetOpen,
}: {
  vacuumState: ActionFeedbackState;
  optimizeState: ActionFeedbackState;
  onVacuum: () => void;
  onOptimize: () => void;
  onResetOpen: () => void;
}) {
  return (
    <div className="space-y-2">
      <OperationRow
        testid="system-vacuum-row"
        label="Vacuum"
        description={VACUUM_HELP}
        button={
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={vacuumState === "pending"}
                onClick={onVacuum}
                className="cursor-pointer min-w-[7.5rem] justify-center"
                data-testid="system-vacuum-button"
                data-state={vacuumState}
              >
                <ActionButtonContent
                  state={vacuumState}
                  idleIcon={<IconBolt className="h-3.5 w-3.5 mr-1" />}
                  idleLabel="Run vacuum"
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{VACUUM_HELP}</TooltipContent>
          </Tooltip>
        }
      />
      <OperationRow
        testid="system-optimize-row"
        label="Optimize"
        description={OPTIMIZE_HELP}
        button={
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="outline"
                size="sm"
                disabled={optimizeState === "pending"}
                onClick={onOptimize}
                className="cursor-pointer min-w-[7.5rem] justify-center"
                data-testid="system-optimize-button"
                data-state={optimizeState}
              >
                <ActionButtonContent
                  state={optimizeState}
                  idleIcon={<IconRefresh className="h-3.5 w-3.5 mr-1" />}
                  idleLabel="Run optimize"
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{OPTIMIZE_HELP}</TooltipContent>
          </Tooltip>
        }
      />
      <OperationRow
        testid="system-factory-reset-row"
        label="Factory reset"
        description={FACTORY_RESET_HELP}
        button={
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                variant="destructive"
                size="sm"
                onClick={onResetOpen}
                className="cursor-pointer"
                data-testid="system-factory-reset-button"
              >
                <IconTrash className="h-3.5 w-3.5 mr-1" /> Factory reset
              </Button>
            </TooltipTrigger>
            <TooltipContent>{FACTORY_RESET_HELP}</TooltipContent>
          </Tooltip>
        }
      />
    </div>
  );
}

export function DatabaseStatsCard() {
  const { database, isLoading, error, reload } = useDatabaseStats();
  const vacuum = useActionFeedback();
  const optimize = useActionFeedback();
  const [resetOpen, setResetOpen] = useState(false);

  const onVacuum = () =>
    void vacuum.run(async () => {
      await vacuumDatabase();
      await reload();
    });

  const onOptimize = () =>
    void optimize.run(async () => {
      await optimizeDatabase();
    });

  return (
    <Card data-testid="system-database-card">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2">
          <IconDatabase className="h-4 w-4" /> Database
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p className="text-xs text-red-500" data-testid="system-database-error">
            {error}
          </p>
        )}
        {!database && isLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> Loading database stats...
          </div>
        )}
        {database && <StatsTable database={database} />}
        <MaintenanceButtons
          vacuumState={vacuum.state}
          optimizeState={optimize.state}
          onVacuum={onVacuum}
          onOptimize={onOptimize}
          onResetOpen={() => setResetOpen(true)}
        />
        <div className="flex flex-col gap-1">
          <JobProgressIndicator kind="vacuum" />
          <JobProgressIndicator kind="optimize" />
          <JobProgressIndicator kind="factory-reset" />
        </div>
        <FactoryResetDialog open={resetOpen} onOpenChange={setResetOpen} />
      </CardContent>
    </Card>
  );
}
