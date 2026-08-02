"use client";

import { useEffect, useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@kandev/ui/card";
import { Spinner } from "@kandev/ui/spinner";
import { IconRestore, IconTrash } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
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
  const eligible = isQuarantineEligible(entry, now);
  return (
    <div className="min-w-0 rounded-lg border p-3" data-testid={`storage-quarantine-${entry.id}`}>
      <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="outline">{entry.resource_type.replace("_", " ")}</Badge>
            <span className="text-xs text-muted-foreground">
              {formatGigabytes(entry.size_bytes)}
            </span>
          </div>
          <p className="break-all font-mono text-xs">{entry.original_path}</p>
          <p className="break-all text-[11px] text-muted-foreground">
            Trash: {entry.quarantine_path}
          </p>
          {entry.last_error && (
            <p className="break-words text-xs text-red-500">{entry.last_error}</p>
          )}
          <p className="text-xs text-muted-foreground">
            {eligible ? (
              <span className="text-emerald-600">Eligible now</span>
            ) : (
              <>
                Protected until{" "}
                <time dateTime={entry.delete_after}>{formatQuarantineDeadline(entry)}</time>
              </>
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
            <IconRestore className="size-4" /> Restore
          </StorageActionButton>
          <StorageActionButton
            variant="destructive"
            disabledReason={
              disabledReason ??
              (eligible ? undefined : `Retention ends ${formatQuarantineDeadline(entry)}.`)
            }
            onClick={() => onDelete(entry)}
            data-testid={`storage-quarantine-${entry.id}-delete`}
          >
            <IconTrash className="size-4" /> Delete
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
          Quarantine
          <StorageSettingHelp label="Quarantine">
            Quarantine is Kandev's recoverable holding area. Orphan task workspaces and rotated Go
            caches are moved here before deletion. You can restore an item during its retention
            period; after the deadline, a later maintenance run may permanently delete it.
          </StorageSettingHelp>
        </CardTitle>
        <JobProgressIndicator
          kind="storage-quarantine-delete"
          jobId={deleteJobId}
          successLabel="Deletion complete"
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
            (counts.eligible === 0 ? "No quarantine entries are eligible yet." : undefined)
          }
          onClick={() => onPurge("eligible")}
          data-testid="storage-quarantine-clear-eligible"
        >
          Clear eligible
        </StorageActionButton>
        <StorageActionButton
          variant="destructive"
          className="w-full sm:w-auto"
          disabled={entries.length === 0}
          disabledReason={bulkDisabledReason}
          onClick={() => onPurge("all")}
          data-testid="storage-quarantine-force-clear"
        >
          Force clear all
        </StorageActionButton>
      </div>
      <CardDescription>
        Cleanup moves recoverable data here first instead of deleting it immediately. Restore an
        item if you still need it, or delete it permanently when you are certain. {counts.eligible}{" "}
        eligible now · {counts.protected} protected
      </CardDescription>
      {showTotal && (
        <p className="text-xs font-medium" data-testid="storage-quarantine-total">
          {t("settings:storageQuarantineTotal", { size: formatGigabytes(totalBytes) })}
        </p>
      )}
      <p className="text-xs text-muted-foreground" data-testid="storage-quarantine-schedule-copy">
        {schedulingEnabled
          ? `Eligible entries are removed by the first successful maintenance run after their deadline (every ${checkIntervalHours} hours when the idle gate allows it).`
          : "Automatic quarantine cleanup is off. Run maintenance manually or use a quarantine action when you are ready."}
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
        {t("settings:storageQuarantineLoading")}
      </div>
    );
  }
  if (error) {
    return (
      <p className="break-words text-sm text-destructive" data-testid="storage-quarantine-error">
        {t("settings:storageSectionUnavailable")}: {error}
      </p>
    );
  }
  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">{t("settings:storageQuarantineEmpty")}</p>;
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
    disabledReason ?? (deleteJobActive ? "A quarantine cleanup is still running." : undefined);
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
