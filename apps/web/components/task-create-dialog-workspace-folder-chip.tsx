"use client";

import { IconFolder, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import type { TaskRepositorySelection } from "@/components/task-create-dialog-types";
import { useTranslation } from "react-i18next";

export function FolderSelectionChip({
  selection,
  repositoryLocked,
  onRemove,
}: {
  selection: Extract<TaskRepositorySelection, { kind: "folder" }>;
  repositoryLocked?: boolean;
  onRemove: () => void;
}) {
  const { t } = useTranslation();
  const label = selection.displayName || folderLeafName(selection.localPath);
  return (
    <div
      className="inline-flex min-h-7 min-w-0 max-w-full items-center gap-1.5 rounded-md border border-border/60 bg-primary/10 px-2 text-xs text-foreground"
      data-testid="workspace-folder-selection"
      title={selection.localPath}
    >
      <IconFolder className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
      <span className="max-w-[220px] truncate">{label}</span>
      {repositoryLocked ? null : (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="size-6 shrink-0 rounded-sm p-0 [@media(pointer:coarse)]:size-9"
          onClick={onRemove}
          aria-label={t("task:removeWorkspaceFolder")}
          data-testid="remove-workspace-folder"
        >
          <IconX className="size-3.5" aria-hidden="true" />
        </Button>
      )}
    </div>
  );
}

function folderLeafName(path: string): string {
  const trimmed = path.replace(/[\\/]+$/, "");
  if (!trimmed) return path;
  const index = Math.max(trimmed.lastIndexOf("/"), trimmed.lastIndexOf("\\"));
  return index >= 0 ? trimmed.slice(index + 1) || trimmed : trimmed;
}
