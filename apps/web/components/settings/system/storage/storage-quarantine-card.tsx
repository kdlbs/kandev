"use client";

import { useEffect, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconRestore, IconTrash } from "@tabler/icons-react";
import { Trans, useTranslation } from "react-i18next";
import type { StorageQuarantineEntry, StorageQuarantinePurgeScope } from "@/lib/types/system";
import { JobProgressIndicator } from "../job-progress-indicator";
import { PermanentDeleteDialog, QuarantinePurgeDialog } from "./storage-confirmation-dialogs";
import { StorageActionButton } from "./storage-action-button";
import { StorageSettingHelp } from "./storage-setting-help";
import { formatGigabytes } from "./storage-units";
import { quarantineTotalBytes } from "./storage-totals";
import {
  formatQuarantineDeadline,
  isQuarantineEligible,
  quarantineCounts,
  quarantineDeleteAfter,
} from "./storage-quarantine";

const MAX_TIMER_DELAY_MS = 2_147_483_647;

/**
 * `resource_type` is a wire enum. It used to be rendered as
 * `replace("_", " ")`, which is English-shaped by accident; each value now
 * resolves through the catalog and falls back to the raw token so an unknown
 * type from a newer backend still shows something.
 */
const RESOURCE_TYPE_LABEL_KEYS: Record<StorageQuarantineEntry["resource_type"], string> = {
  task_workspace: "system:storageResourceTypeTaskWorkspace",
  go_cache: "system:storageResourceTypeGoCache",
};

function resourceTypeLabel(
  t: (key: string) => string,
  resourceType: StorageQuarantineEntry["resource_type"],
): string {
  const key = RESOURCE_TYPE_LABEL_KEYS[resourceType];
  // `replaceAll`, not `replace`: the original single-pattern call only reached
  // the first underscore, so a future `some_new_resource_type` would have
  // rendered as "some new_resource_type".
  return key ? t(key) : resourceType.replaceAll("_", " ");
}

type Props = {
  entries: StorageQuarantineEntry[];
  loading?: boolean;
  error?: string | null;
  deleteJobId?: string;
  deleteJobActive?: boolean;
  disabledReason?: string;
  schedulingEnabled: boolean;
  checkIntervalHours: number;
  onRestore: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
  onClearEligible: () => Promise<void>;
  onForceClearAll: () => Promise<void>;
};

function QuarantineEntryRow({
  entry,
  now,
  disabledReason,
  onRestore,
  onDelete,
}: {
  entry: StorageQuarantineEntry;
  now: Date;
  disabledReason?: string;
  onRestore: (id: string) => Promise<void>;
  onDelete: (entry: StorageQuarantineEntry) => void;
}) {
  const { t } = useTranslation();
  const eligible = isQuarantineEligible(entry, now);
  const deadline = formatQuarantineDeadline(entry);
  return (
    <div className="min-w-0 rounded-lg border p-3" data-testid={`storage-quarantine-${entry.id}`}>
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{resourceTypeLabel(t, entry.resource_type)}</Badge>
            <span className="text-xs text-muted-foreground">
              {formatGigabytes(entry.size_bytes)}
            </span>
          </div>
          {/* Both paths are API data and are never routed through the catalog. */}
          <p className="break-all font-mono text-xs">{entry.original_path}</p>
          <p className="break-all text-[11px] text-muted-foreground">
            {t("system:storageTrashPath", { path: entry.quarantine_path })}
          </p>
          {entry.last_error && (
            <p className="break-words text-xs text-red-500">{entry.last_error}</p>
          )}
          <p className="text-xs text-muted-foreground">
            {eligible ? (
              <span className="text-emerald-600">{t("system:storageEligibleNow")}</span>
            ) : (
              <Trans i18nKey="system:storageProtectedUntil" values={{ deadline }}>
                Protected until <time dateTime={entry.delete_after}>{deadline}</time>
              </Trans>
            )}
          </p>
        </div>
        <div className="flex shrink-0 flex-col gap-2 sm:flex-row">
          <StorageActionButton
            variant="outline"
            disabledReason={disabledReason}
            onClick={() => void onRestore(entry.id)}
            data-testid={`storage-quarantine-${entry.id}-restore`}
          >
            <IconRestore className="size-4" /> {t("system:storageRestore")}
          </StorageActionButton>
          <StorageActionButton
            variant="destructive"
            disabledReason={
              disabledReason ??
              (eligible ? undefined : t("system:storageRetentionEnds", { deadline }))
            }
            onClick={() => onDelete(entry)}
            data-testid={`storage-quarantine-${entry.id}-delete`}
          >
            <IconTrash className="size-4" /> {t("system:storageDelete")}
          </StorageActionButton>
        </div>
      </div>
    </div>
  );
}

function QuarantineHeader({
  entries,
  counts,
  deleteJobId,
  bulkDisabledReason,
  schedulingEnabled,
  checkIntervalHours,
  showTotal,
  onPurge,
  totalBytes,
}: {
  entries: StorageQuarantineEntry[];
  counts: ReturnType<typeof quarantineCounts>;
  deleteJobId?: string;
  bulkDisabledReason?: string;
  schedulingEnabled: boolean;
  checkIntervalHours: number;
  showTotal: boolean;
  onPurge: (scope: StorageQuarantinePurgeScope) => void;
  totalBytes: number;
}) {
  const { t } = useTranslation();
  return (
    <CardHeader>
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle className="flex items-center gap-1 text-base">
          {t("system:storageQuarantineHeading")}
          <StorageSettingHelp label={t("system:storageQuarantineHeading")}>
            {t("system:storageQuarantineHelp")}
          </StorageSettingHelp>
        </CardTitle>
        <JobProgressIndicator
          kind="storage-quarantine-delete"
          jobId={deleteJobId}
          successLabel={t("system:storageDeletionComplete")}
          testId="storage-delete-job"
        />
      </div>
      <div className="flex flex-col gap-2 sm:flex-row">
        <StorageActionButton
          variant="outline"
          className="w-full sm:w-auto"
          disabled={counts.eligible === 0 || entries.length === 0}
          disabledReason={
            bulkDisabledReason ??
            (counts.eligible === 0 ? t("system:storageNoEligibleEntries") : undefined)
          }
          onClick={() => onPurge("eligible")}
          data-testid="storage-quarantine-clear-eligible"
        >
          {t("system:storageClearEligible")}
        </StorageActionButton>
        <StorageActionButton
          variant="destructive"
          className="w-full sm:w-auto"
          disabled={entries.length === 0}
          disabledReason={bulkDisabledReason}
          onClick={() => onPurge("all")}
          data-testid="storage-quarantine-force-clear"
        >
          {t("system:storageForceClearAll")}
        </StorageActionButton>
      </div>
      <CardDescription>
        {t("system:storageQuarantineCardDescription")}{" "}
        {t("system:storageQuarantineEligibleCount", { count: counts.eligible })} ·{" "}
        {t("system:storageQuarantineProtectedCount", { count: counts.protected })}
      </CardDescription>
      {showTotal && (
        <p className="text-xs font-medium" data-testid="storage-quarantine-total">
          {t("system:storageQuarantineTotal", { size: formatGigabytes(totalBytes) })}
        </p>
      )}
      <p className="text-xs text-muted-foreground" data-testid="storage-quarantine-schedule-copy">
        {schedulingEnabled
          ? t("system:storageQuarantineScheduleOn", { count: checkIntervalHours })
          : t("system:storageQuarantineScheduleOff")}
      </p>
    </CardHeader>
  );
}

function QuarantineContent({
  entries,
  loading,
  error,
  now,
  disabledReason,
  onRestore,
  onDelete,
}: {
  entries: StorageQuarantineEntry[];
  loading: boolean;
  error?: string | null;
  now: Date;
  disabledReason?: string;
  onRestore: (id: string) => Promise<void>;
  onDelete: (entry: StorageQuarantineEntry) => void;
}) {
  const { t } = useTranslation();
  if (loading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Spinner className="size-4" data-testid="storage-quarantine-spinner" />
        {t("system:storageQuarantineLoading")}
      </div>
    );
  }
  if (error) {
    return (
      <p className="break-words text-sm text-destructive" data-testid="storage-quarantine-error">
        {t("system:storageSectionUnavailable")}: {error}
      </p>
    );
  }
  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("system:storageQuarantineEmpty")}</p>;
  }
  return entries.map((entry) => (
    <QuarantineEntryRow
      key={entry.id}
      entry={entry}
      now={now}
      disabledReason={disabledReason}
      onRestore={onRestore}
      onDelete={onDelete}
    />
  ));
}

export function StorageQuarantineCard({
  entries,
  loading = false,
  error,
  deleteJobId,
  deleteJobActive,
  disabledReason,
  schedulingEnabled,
  checkIntervalHours,
  onRestore,
  onDelete,
  onClearEligible,
  onForceClearAll,
}: Props) {
  const { t } = useTranslation();
  const [deleteEntry, setDeleteEntry] = useState<StorageQuarantineEntry | null>(null);
  const [purgeScope, setPurgeScope] = useState<StorageQuarantinePurgeScope | null>(null);
  const [now, setNow] = useState(() => new Date());
  useEffect(() => {
    const nextDeadline = entries
      .map(quarantineDeleteAfter)
      .map((deadline) => deadline.getTime())
      .filter((deadline) => deadline > now.getTime())
      .sort((left, right) => left - right)[0];
    if (nextDeadline === undefined) return;
    const delay = Math.min(MAX_TIMER_DELAY_MS, Math.max(0, nextDeadline - now.getTime() + 1));
    const timer = setTimeout(() => setNow(new Date()), delay);
    return () => clearTimeout(timer);
  }, [entries, now]);
  const counts = quarantineCounts(entries, now);
  const totalBytes = quarantineTotalBytes(entries);
  const bulkDisabledReason =
    disabledReason ?? (deleteJobActive ? t("system:storageQuarantineCleanupRunning") : undefined);
  return (
    <Card className="min-w-0" data-testid="storage-quarantine-card">
      <QuarantineHeader
        entries={entries}
        counts={counts}
        deleteJobId={deleteJobId}
        bulkDisabledReason={bulkDisabledReason}
        schedulingEnabled={schedulingEnabled}
        checkIntervalHours={checkIntervalHours}
        showTotal={!loading && !error}
        onPurge={setPurgeScope}
        totalBytes={totalBytes}
      />
      <CardContent className="space-y-3">
        <QuarantineContent
          entries={entries}
          loading={loading}
          error={error}
          now={now}
          disabledReason={bulkDisabledReason}
          onRestore={onRestore}
          onDelete={setDeleteEntry}
        />
      </CardContent>
      <PermanentDeleteDialog
        entry={deleteEntry}
        open={deleteEntry !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteEntry(null);
        }}
        onConfirm={() => {
          if (deleteEntry) void onDelete(deleteEntry.id);
          setDeleteEntry(null);
        }}
      />
      {purgeScope && (
        <QuarantinePurgeDialog
          scope={purgeScope}
          eligibleCount={counts.eligible}
          protectedCount={counts.protected}
          open
          onOpenChange={(open) => {
            if (!open) setPurgeScope(null);
          }}
          onConfirm={() => {
            if (purgeScope === "eligible") void onClearEligible();
            else void onForceClearAll();
            setPurgeScope(null);
          }}
        />
      )}
    </Card>
  );
}
