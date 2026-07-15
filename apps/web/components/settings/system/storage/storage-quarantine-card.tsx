"use client";

import { useState } from "react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { IconRestore, IconTrash } from "@tabler/icons-react";
import type { StorageQuarantineEntry } from "@/lib/types/system";
import { formatBytes } from "@/lib/utils/format-bytes";
import { PermanentDeleteDialog } from "./storage-confirmation-dialogs";
import { StorageActionButton } from "./storage-action-button";

type Props = {
  entries: StorageQuarantineEntry[];
  disabledReason?: string;
  onRestore: (id: string) => Promise<void>;
  onDelete: (id: string) => Promise<void>;
};

export function StorageQuarantineCard({ entries, disabledReason, onRestore, onDelete }: Props) {
  const [deleteEntry, setDeleteEntry] = useState<StorageQuarantineEntry | null>(null);
  return (
    <Card className="min-w-0" data-testid="storage-quarantine-card">
      <CardHeader>
        <CardTitle className="text-base">Quarantine</CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {entries.length === 0 && (
          <p className="text-sm text-muted-foreground">No restorable quarantined resources.</p>
        )}
        {entries.map((entry) => (
          <div
            key={entry.id}
            className="min-w-0 rounded-lg border p-3"
            data-testid={`storage-quarantine-${entry.id}`}
          >
            <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline">{entry.resource_type.replace("_", " ")}</Badge>
                  <span className="text-xs text-muted-foreground">
                    {formatBytes(entry.size_bytes)}
                  </span>
                </div>
                <p className="break-all font-mono text-xs">{entry.original_path}</p>
                <p className="break-all text-[11px] text-muted-foreground">
                  Trash: {entry.quarantine_path}
                </p>
                {entry.last_error && (
                  <p className="break-words text-xs text-red-500">{entry.last_error}</p>
                )}
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
                  disabledReason={disabledReason}
                  onClick={() => setDeleteEntry(entry)}
                  data-testid={`storage-quarantine-${entry.id}-delete`}
                >
                  <IconTrash className="size-4" /> Delete
                </StorageActionButton>
              </div>
            </div>
          </div>
        ))}
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
    </Card>
  );
}
