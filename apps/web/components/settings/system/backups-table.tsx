"use client";

import { useState } from "react";
import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Badge } from "@kandev/ui/badge";
import { Spinner } from "@kandev/ui/spinner";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@kandev/ui/table";
import { IconDownload, IconTrash, IconArchive, IconRotateClockwise } from "@tabler/icons-react";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";
import { useBackups } from "@/hooks/domains/system/use-backups";
import { buildBackupDownloadUrl, createBackup, deleteBackup } from "@/lib/api/domains/system-api";
import { formatDateTime } from "@/lib/i18n/formats";
import { formatBytes } from "@/lib/utils/format-bytes";
import { JobProgressIndicator } from "./job-progress-indicator";
import { RestoreDialog } from "./restore-dialog";
import type { SnapshotInfo, SnapshotKind } from "@/lib/types/system";
import { SettingsCardHeader } from "@/components/settings/settings-card-header";
import {
  SettingsErrorText,
  SettingsFieldDescription,
} from "@/components/settings/settings-typography";
import { settingsActionClassName } from "@/components/settings/settings-control";

const BACKUP_CREATE_TIMEOUT_MS = 15_000;
const BACKUP_CREATE_POLL_MS = 250;
/**
 * How many snapshots the backend keeps. Only referenced by the help text, but
 * it travels as a `count` so the sentence inflects in every locale rather than
 * hardcoding the English plural.
 */
const BACKUP_RETENTION_COUNT = 2;

function formatTimestamp(iso: string): string {
  if (!iso) return "-";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return formatDateTime(d);
}

/** `kind` is the wire enum; only its badge label is copy. */
const KIND_LABEL_KEYS: Record<SnapshotKind, string> = {
  auto: "system:backupsKindAuto",
  manual: "system:backupsKindManual",
};

function BackupRowActions({
  row,
  onRestore,
  onDelete,
}: {
  row: SnapshotInfo;
  onRestore: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center justify-end gap-1">
      <Button
        asChild
        size="sm"
        variant="ghost"
        className="cursor-pointer"
        data-testid="system-backups-download"
      >
        {/* These three are icon-only, so the aria-label is their only
            accessible name. It names the snapshot it acts on. */}
        <a
          href={buildBackupDownloadUrl(row.name)}
          download
          aria-label={t("system:backupsDownloadLabel", { name: row.name })}
        >
          <IconDownload className="h-3.5 w-3.5" />
        </a>
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="cursor-pointer"
        onClick={() => onRestore(row.name)}
        aria-label={t("system:backupsRestoreLabel", { name: row.name })}
        data-testid="system-backups-restore"
      >
        <IconRotateClockwise className="h-3.5 w-3.5" />
      </Button>
      <Button
        size="sm"
        variant="ghost"
        className="cursor-pointer text-destructive"
        onClick={() => onDelete(row.name)}
        aria-label={t("system:backupsDeleteLabel", { name: row.name })}
        data-testid="system-backups-delete"
      >
        <IconTrash className="h-3.5 w-3.5" />
      </Button>
    </div>
  );
}

function BackupRow({
  row,
  isAdmin,
  onRestore,
  onDelete,
}: {
  row: SnapshotInfo;
  isAdmin: boolean;
  onRestore: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <TableRow data-testid="system-backups-row" data-name={row.name}>
      {/* The snapshot filename is a value. */}
      <TableCell className="font-mono text-xs break-all" data-testid="system-backups-name">
        {row.name}
      </TableCell>
      <TableCell>
        <Badge variant={row.kind === "manual" ? "default" : "secondary"} className="text-[10px]">
          {KIND_LABEL_KEYS[row.kind] ? t(KIND_LABEL_KEYS[row.kind]) : row.kind}
        </Badge>
      </TableCell>
      <TableCell className="text-xs text-right">{formatBytes(row.size_bytes)}</TableCell>
      <TableCell className="text-xs">{formatTimestamp(row.mtime)}</TableCell>
      {/* The whole cell goes, not just its contents: the Actions header is
          conditional too, and an empty fifth cell would leave the table with
          four headers and five cells: an unlabeled column for assistive
          technology and dead width on a phone. */}
      {isAdmin && (
        <TableCell>
          <BackupRowActions row={row} onRestore={onRestore} onDelete={onDelete} />
        </TableCell>
      )}
    </TableRow>
  );
}

function BackupsList({
  items,
  isAdmin,
  onRestore,
  onDelete,
}: {
  items: SnapshotInfo[];
  isAdmin: boolean;
  onRestore: (name: string) => void;
  onDelete: (name: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <Table data-testid="system-backups-table">
      <TableHeader>
        <TableRow>
          <TableHead>{t("system:backupsColumnName")}</TableHead>
          <TableHead>{t("system:backupsColumnKind")}</TableHead>
          <TableHead className="text-right">{t("system:backupsColumnSize")}</TableHead>
          <TableHead>{t("system:backupsColumnCreated")}</TableHead>
          {isAdmin && (
            <TableHead className="text-right">{t("system:backupsColumnActions")}</TableHead>
          )}
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((row) => (
          <BackupRow
            key={row.name}
            row={row}
            isAdmin={isAdmin}
            onRestore={onRestore}
            onDelete={onDelete}
          />
        ))}
      </TableBody>
    </Table>
  );
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForCreatedBackup(
  reload: () => Promise<SnapshotInfo[]>,
  previousNames: Set<string>,
  t: TFunction,
): Promise<void> {
  const deadline = Date.now() + BACKUP_CREATE_TIMEOUT_MS;
  while (Date.now() < deadline) {
    const items = await reload();
    if (items.some((item) => item.kind === "manual" && !previousNames.has(item.name))) {
      return;
    }
    await sleep(BACKUP_CREATE_POLL_MS);
  }
  // This message is caught and rendered into the card's error line, so it is
  // display copy rather than a control-flow-only throw.
  throw new Error(t("system:backupsCreateTimeout", { seconds: BACKUP_CREATE_TIMEOUT_MS / 1_000 }));
}

export function BackupsTable() {
  const { t } = useTranslation();
  const { backups, loaded, isLoading, reload } = useBackups();
  // Creating, downloading, restoring, and deleting a snapshot are admin-only
  // on the backend: a snapshot is a copy of the whole multi-user database.
  // Members keep the read-only listing.
  const isAdmin = useIsAdmin();
  const [creating, setCreating] = useState(false);
  const [restoreName, setRestoreName] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const onCreate = async () => {
    setCreating(true);
    setError(null);
    const previousNames = new Set(backups.map((backup) => backup.name));
    try {
      await createBackup();
      await waitForCreatedBackup(reload, previousNames, t);
    } catch (err) {
      setError(err instanceof Error ? err.message : t("system:backupsCreateFailed"));
    } finally {
      setCreating(false);
    }
  };

  const onDelete = async (name: string) => {
    setError(null);
    try {
      await deleteBackup(name);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : t("system:backupsDeleteFailed"));
    }
  };

  const items = backups;

  return (
    <Card data-testid="system-backups-card">
      <SettingsCardHeader
        title={
          <span className="flex items-center gap-2">
            <IconArchive className="h-4 w-4" /> {t("system:backupsTitle")}
          </span>
        }
        actions={
          isAdmin && (
            <Button
              size="sm"
              disabled={creating}
              onClick={() => void onCreate()}
              className={settingsActionClassName("cursor-pointer")}
              data-testid="system-backups-create"
            >
              {creating ? t("system:backupsCreating") : t("system:backupsCreate")}
            </Button>
          )
        }
      />
      <CardContent className="space-y-4">
        <SettingsFieldDescription data-testid="system-backups-help">
          {t("system:backupsHelp")}{" "}
          {t("system:backupsRetention", { count: BACKUP_RETENTION_COUNT })}
        </SettingsFieldDescription>
        {!isAdmin && (
          <SettingsFieldDescription data-testid="system-backups-admin-only">
            {t("system:backupsAdminOnly")}
          </SettingsFieldDescription>
        )}
        {error && <SettingsErrorText data-testid="system-backups-error">{error}</SettingsErrorText>}
        {!loaded && isLoading && (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Spinner className="size-4" /> {t("system:backupsLoading")}
          </div>
        )}
        {loaded && items.length === 0 && (
          <p className="text-sm text-muted-foreground" data-testid="system-backups-empty">
            {t("system:backupsEmpty")}
          </p>
        )}
        {items.length > 0 && (
          <BackupsList
            items={items}
            isAdmin={isAdmin}
            onRestore={(n) => setRestoreName(n)}
            onDelete={(n) => void onDelete(n)}
          />
        )}

        <div className="flex flex-col gap-1">
          <JobProgressIndicator kind="backup-create" />
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
