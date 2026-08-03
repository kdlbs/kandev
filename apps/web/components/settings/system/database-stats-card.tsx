"use client";

import { useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
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
import type { DatabaseStats } from "@/lib/types/system";
import { formatDateTime } from "@/lib/i18n/formats";
import { formatBytes } from "@/lib/utils/format-bytes";
import { useActionFeedback, type ActionFeedbackState } from "@/hooks/use-action-feedback";
import { ActionButtonContent } from "./action-button-content";
import { JobProgressIndicator } from "./job-progress-indicator";
import { FactoryResetDialog } from "./factory-reset-dialog";

function formatTimestamp(iso: string | null | undefined, t: TFunction): string {
  if (!iso) return t("system:databaseNever");
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return formatDateTime(d);
}

type Row = { label: string; value: string; testid: string; info?: string };

function StatRow({ label, value, testid, info }: Row) {
  const { t } = useTranslation();
  return (
    <div className="flex items-baseline justify-between gap-4 py-1.5 border-b last:border-b-0">
      <span className="text-xs text-muted-foreground inline-flex items-center gap-1">
        {label}
        {info && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                aria-label={t("system:databaseWhatIs", { label })}
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

function databaseDriver(database: DatabaseStats): string {
  return database.driver || "sqlite";
}

/**
 * `driver` is a wire enum. `PostgreSQL` / `SQLite` are the products' own
 * spellings — brand nouns, not copy — so they stay literal, and an unknown
 * driver echoes its raw token.
 */
function formatDriver(driver: string): string {
  if (driver === "postgres") return "PostgreSQL";
  if (driver === "sqlite") return "SQLite";
  return driver;
}

function StatsTable({ database }: { database: DatabaseStats }) {
  const { t } = useTranslation();
  const driver = databaseDriver(database);
  const isSQLite = driver === "sqlite";

  return (
    <div className="rounded-md border px-3 py-2">
      <StatRow
        label={t("system:databaseDriver")}
        value={formatDriver(driver)}
        testid="system-db-driver"
      />
      {/* The database file path is a value. */}
      {isSQLite && (
        <StatRow label={t("system:databasePath")} value={database.path} testid="system-db-path" />
      )}
      <StatRow
        label={isSQLite ? t("system:databaseSize") : t("system:databaseSizeLong")}
        value={formatBytes(database.size_bytes)}
        testid="system-db-size"
      />
      {isSQLite && (
        <StatRow
          label={t("system:databaseWal")}
          value={formatBytes(database.wal_size_bytes)}
          testid="system-db-wal"
          info={t("system:databaseWalHelp")}
        />
      )}
      <StatRow
        label={t("system:databaseSchemaVersion")}
        value={database.schema_version || "-"}
        testid="system-db-schema-version"
      />
      {isSQLite && (
        <StatRow
          label={t("system:databaseLastBackup")}
          value={formatTimestamp(database.last_backup_at, t)}
          testid="system-db-last-backup"
        />
      )}
    </div>
  );
}

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
      className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-start sm:justify-between"
      data-testid={testid}
    >
      <div className="min-w-0 flex-1">
        <p className="text-sm font-medium">{label}</p>
        <p className="text-xs text-muted-foreground mt-1">{description}</p>
      </div>
      <div className="shrink-0 self-start sm:self-auto">{button}</div>
    </div>
  );
}

function UnsupportedMaintenance({ driver }: { driver: string }) {
  const { t } = useTranslation();
  return (
    <p
      className="text-xs text-muted-foreground"
      data-testid="system-database-maintenance-unavailable"
    >
      {t("system:databaseMaintenanceUnavailable", { driver: formatDriver(driver) })}
    </p>
  );
}

type SQLiteMaintenanceButtonsProps = {
  vacuumState: ActionFeedbackState;
  optimizeState: ActionFeedbackState;
  onVacuum: () => void;
  onOptimize: () => void;
  onResetOpen: () => void;
};

function SQLiteMaintenanceButtons({
  vacuumState,
  optimizeState,
  onVacuum,
  onOptimize,
  onResetOpen,
}: SQLiteMaintenanceButtonsProps) {
  const { t } = useTranslation();
  // Each operation's plain-language description is shown both as a paragraph
  // and in the button's tooltip, so it resolves once here.
  const vacuumHelp = t("system:databaseVacuumHelp");
  const optimizeHelp = t("system:databaseOptimizeHelp");
  const factoryResetHelp = t("system:databaseFactoryResetHelp");
  return (
    <div className="space-y-2">
      <OperationRow
        testid="system-vacuum-row"
        label={t("system:databaseVacuum")}
        description={vacuumHelp}
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
                  idleLabel={t("system:databaseRunVacuum")}
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{vacuumHelp}</TooltipContent>
          </Tooltip>
        }
      />
      <OperationRow
        testid="system-optimize-row"
        label={t("system:databaseOptimize")}
        description={optimizeHelp}
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
                  idleLabel={t("system:databaseRunOptimize")}
                />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{optimizeHelp}</TooltipContent>
          </Tooltip>
        }
      />
      <OperationRow
        testid="system-factory-reset-row"
        label={t("system:databaseFactoryReset")}
        description={factoryResetHelp}
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
                <IconTrash className="h-3.5 w-3.5 mr-1" /> {t("system:databaseFactoryReset")}
              </Button>
            </TooltipTrigger>
            <TooltipContent>{factoryResetHelp}</TooltipContent>
          </Tooltip>
        }
      />
    </div>
  );
}

function MaintenanceButtons({
  driver,
  ...props
}: SQLiteMaintenanceButtonsProps & { driver: string | null }) {
  if (driver === null) {
    return null;
  }

  if (driver !== "sqlite") {
    return <UnsupportedMaintenance driver={driver} />;
  }

  return <SQLiteMaintenanceButtons {...props} />;
}

export function DatabaseStatsCard() {
  const { t } = useTranslation();
  const { database, isLoading, error, reload } = useDatabaseStats();
  const vacuum = useActionFeedback();
  const optimize = useActionFeedback();
  const [resetOpen, setResetOpen] = useState(false);
  const driver = database ? databaseDriver(database) : null;

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
          <IconDatabase className="h-4 w-4" /> {t("system:databaseTitle")}
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
            <Spinner className="size-4" /> {t("system:databaseLoading")}
          </div>
        )}
        {database && <StatsTable database={database} />}
        <MaintenanceButtons
          driver={driver}
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
