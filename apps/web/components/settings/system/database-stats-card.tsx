"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { IconDatabase, IconBolt, IconRefresh, IconTrash } from "@tabler/icons-react";
import { useDatabaseStats } from "@/hooks/domains/system/use-database-stats";
import { optimizeDatabase, vacuumDatabase } from "@/lib/api/domains/system-api";
import { formatBytes } from "@/lib/utils/format-bytes";
import { JobProgressIndicator } from "./job-progress-indicator";
import { FactoryResetDialog } from "./factory-reset-dialog";

function formatTimestamp(iso: string | null | undefined): string {
  if (!iso) return "Never";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

type Row = { label: string; value: string; testid: string };

function StatRow({ label, value, testid }: Row) {
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5 border-b last:border-b-0">
      <span className="text-xs text-muted-foreground">{label}</span>
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
      <StatRow label="WAL" value={formatBytes(database.wal_size_bytes)} testid="system-db-wal" />
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

function MaintenanceButtons({
  vacuumPending,
  optimizePending,
  onVacuum,
  onOptimize,
  onResetOpen,
}: {
  vacuumPending: boolean;
  optimizePending: boolean;
  onVacuum: () => void;
  onOptimize: () => void;
  onResetOpen: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        variant="outline"
        size="sm"
        disabled={vacuumPending}
        onClick={onVacuum}
        className="cursor-pointer"
        data-testid="system-vacuum-button"
      >
        <IconBolt className="h-3.5 w-3.5 mr-1" /> VACUUM
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={optimizePending}
        onClick={onOptimize}
        className="cursor-pointer"
        data-testid="system-optimize-button"
      >
        <IconRefresh className="h-3.5 w-3.5 mr-1" /> Optimize
      </Button>
      <Button
        variant="destructive"
        size="sm"
        onClick={onResetOpen}
        className="cursor-pointer"
        data-testid="system-factory-reset-button"
      >
        <IconTrash className="h-3.5 w-3.5 mr-1" /> Factory Reset
      </Button>
    </div>
  );
}

export function DatabaseStatsCard() {
  const { database, isLoading, error, reload } = useDatabaseStats();
  const [vacuumPending, setVacuumPending] = useState(false);
  const [optimizePending, setOptimizePending] = useState(false);
  const [resetOpen, setResetOpen] = useState(false);

  const onVacuum = async () => {
    setVacuumPending(true);
    try {
      await vacuumDatabase();
      void reload();
    } finally {
      setVacuumPending(false);
    }
  };

  const onOptimize = async () => {
    setOptimizePending(true);
    try {
      await optimizeDatabase();
    } finally {
      setOptimizePending(false);
    }
  };

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
          vacuumPending={vacuumPending}
          optimizePending={optimizePending}
          onVacuum={() => void onVacuum()}
          onOptimize={() => void onOptimize()}
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
