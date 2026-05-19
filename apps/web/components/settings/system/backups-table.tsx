"use client";

import { useEffect, useRef, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconDownload, IconTrash, IconArchive, IconRotateClockwise } from "@tabler/icons-react";
import { useBackups } from "@/hooks/domains/system/use-backups";
import { useSystemJob } from "@/hooks/domains/system/use-system-jobs";
import { buildBackupDownloadUrl, createBackup, deleteBackup } from "@/lib/api/domains/system-api";
import { formatBytes } from "@/lib/utils/format-bytes";
import { JobProgressIndicator } from "./job-progress-indicator";
import { RestoreDialog } from "./restore-dialog";
import type { SnapshotInfo } from "@/lib/types/system";

function formatTimestamp(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

function BackupRow({
  row,
  onRestore,
  onDelete,
}: {
  row: SnapshotInfo;
  onRestore: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  return (
    <TableRow data-testid="system-backups-row" data-name={row.name}>
      <TableCell className="font-mono text-xs break-all" data-testid="system-backups-name">
        {row.name}
      </TableCell>
      <TableCell>
        <Badge variant={row.kind === "manual" ? "default" : "secondary"} className="text-[10px]">
          {row.kind}
        </Badge>
      </TableCell>
      <TableCell className="text-xs text-right">{formatBytes(row.size_bytes)}</TableCell>
      <TableCell className="text-xs">{formatTimestamp(row.mtime)}</TableCell>
      <TableCell>
        <div className="flex items-center justify-end gap-1">
          <Button
            asChild
            size="sm"
            variant="ghost"
            className="cursor-pointer"
            data-testid="system-backups-download"
          >
            <a href={buildBackupDownloadUrl(row.name)} download>
              <IconDownload className="h-3.5 w-3.5" />
            </a>
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer"
            onClick={() => onRestore(row.name)}
            data-testid="system-backups-restore"
          >
            <IconRotateClockwise className="h-3.5 w-3.5" />
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="cursor-pointer text-destructive"
            onClick={() => onDelete(row.name)}
            data-testid="system-backups-delete"
          >
            <IconTrash className="h-3.5 w-3.5" />
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

function BackupsList({
  items,
  onRestore,
  onDelete,
}: {
  items: SnapshotInfo[];
  onRestore: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  return (
    <Table data-testid="system-backups-table">
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Kind</TableHead>
          <TableHead className="text-right">Size</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((row) => (
          <BackupRow key={row.name} row={row} onRestore={onRestore} onDelete={onDelete} />
        ))}
      </TableBody>
    </Table>
  );
}

// useTerminalJobReload observes a single system job by id and fires onSuccess
// when the job transitions into "succeeded", or surfaces job.message via
// onError on "failed". Exits early until a terminal state lands so reloads
// don't fire on intermediate progress events.
function useTerminalJobReload(
  jobId: string | null,
  onSuccess: () => void,
  onError: (message: string) => void,
) {
  const job = useSystemJob(jobId);
  const lastObservedState = useRef<string | null>(null);
  useEffect(() => {
    if (!job) return;
    if (job.state !== "succeeded" && job.state !== "failed") return;
    if (lastObservedState.current === job.state) return;
    lastObservedState.current = job.state;
    if (job.state === "succeeded") {
      onSuccess();
    } else if (job.message) {
      onError(job.message);
    }
  }, [job, onSuccess, onError]);
  return { reset: () => (lastObservedState.current = null) };
}

export function BackupsTable() {
  const { backups, loaded, isLoading, reload } = useBackups();
  const [creating, setCreating] = useState(false);
  const [restoreName, setRestoreName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  // POST /backups returns 202 immediately; track the returned job_id and
  // reload only when that job lands in a terminal state, so we don't flash
  // an empty table while VACUUM INTO is still running.
  const [createJobId, setCreateJobId] = useState<string | null>(null);
  const { reset: resetJobTracking } = useTerminalJobReload(
    createJobId,
    () => void reload(),
    setError,
  );

  const onCreate = async () => {
    setCreating(true);
    setError(null);
    resetJobTracking();
    try {
      const { job_id } = await createBackup();
      setCreateJobId(job_id);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Create snapshot failed");
    } finally {
      setCreating(false);
    }
  };

  const onDelete = async (name: string) => {
    setError(null);
    try {
      await deleteBackup(name);
      void reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Delete failed");
    }
  };

  const items = backups;

  return (
    <Card data-testid="system-backups-card">
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-base flex items-center gap-2">
          <IconArchive className="h-4 w-4" /> Backups
        </CardTitle>
        <Button
          size="sm"
          disabled={creating}
          onClick={() => void onCreate()}
          className="cursor-pointer"
          data-testid="system-backups-create"
        >
          {creating ? "Creating..." : "Create snapshot"}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        {error && (
          <p className="text-xs text-destructive" data-testid="system-backups-error">
            {error}
          </p>
        )}
        {!loaded && isLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> Loading backups...
          </div>
        )}
        {loaded && items.length === 0 && (
          <p className="text-sm text-muted-foreground" data-testid="system-backups-empty">
            No backups yet. Snapshots created automatically on version upgrade or manually via the
            button above will appear here.
          </p>
        )}
        {items.length > 0 && (
          <BackupsList
            items={items}
            onRestore={(n) => setRestoreName(n)}
            onDelete={(n) => void onDelete(n)}
          />
        )}

        <div className="flex flex-col gap-1">
          <JobProgressIndicator kind="backup-create" jobId={createJobId ?? undefined} />
          <JobProgressIndicator kind="restore" />
        </div>

        <RestoreDialog
          open={restoreName !== null}
          onOpenChange={(open) => {
            if (!open) setRestoreName(null);
          }}
          name={restoreName ?? ""}
        />
      </CardContent>
    </Card>
  );
}
