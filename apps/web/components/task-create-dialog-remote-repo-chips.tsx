"use client";

import { useEffect } from "react";
import { IconPlus } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import type { DialogFormState, TaskRemoteRepoRow } from "@/components/task-create-dialog-types";
import {
  RemoteRepoChip,
  type RemoteRepoChipProps,
} from "@/components/task-create-dialog-remote-repo-chip";

/**
 * Chip row for the Remote tab. Renders one `RemoteRepoChip` per row in
 * `fs.remoteRepos`, plus a trailing "+ Add" button. Mirrors the layout of
 * `RepoChipsRow` (workspace/local mode) — same flex-wrap container, same
 * pill-row spacing.
 *
 * Branch loading is driven from `fs.branchesByUrl`: every non-empty URL
 * gets an `ensure(url)` call so the per-URL cache populates ahead of the
 * branch pill being opened. Cleared rows (empty url) are skipped because
 * `ensure("")` is a no-op anyway.
 */
export type RemoteRepoChipsRowProps = {
  fs: DialogFormState;
  onUpdateRow: (key: string, update: Partial<TaskRemoteRepoRow>) => void;
  onAddRow: () => void;
  onRemoveRow: (key: string) => void;
};

export function RemoteRepoChipsRow({
  fs,
  onUpdateRow,
  onAddRow,
  onRemoveRow,
}: RemoteRepoChipsRowProps) {
  // Keep the per-URL branches cache warm. Re-runs whenever the set of URLs
  // changes; `ensure` is internally idempotent so re-firing on unrelated
  // re-renders is cheap.
  useEffect(() => {
    for (const row of fs.remoteRepos) {
      if (row.url) fs.branchesByUrl.ensure(row.url);
    }
  }, [fs.remoteRepos, fs.branchesByUrl]);

  const rows = fs.remoteRepos;
  return (
    <div className="flex min-h-9 flex-wrap items-center gap-2" data-testid="remote-repo-chips-row">
      {rows.map((row) => (
        <RemoteRepoChip
          key={row.key}
          row={row}
          branches={fs.branchesByUrl.branches(row.url)}
          branchesLoading={fs.branchesByUrl.loading(row.url)}
          onURLChange={makeURLChange(onUpdateRow, row.key)}
          onBranchChange={(branch) => onUpdateRow(row.key, { branch })}
          onRemove={() => onRemoveRow(row.key)}
        />
      ))}
      <AddRowButton onAddRow={onAddRow} />
    </div>
  );
}

/**
 * Wires a per-row `onURLChange` so paste-mode strips picker metadata while
 * picker-mode propagates it. Picker-mode also pre-fills `branch` with the
 * repo's `default_branch` so the user can launch without waiting for the
 * branch list to load (they can still pick another branch from the pill
 * dropdown once it populates). Extracted so the JSX above stays compact and
 * the metadata-clearing rule lives in one obvious spot.
 */
function makeURLChange(
  onUpdateRow: (key: string, update: Partial<TaskRemoteRepoRow>) => void,
  key: string,
): RemoteRepoChipProps["onURLChange"] {
  return (url, source, metadata) => {
    if (source === "picker" && metadata) {
      onUpdateRow(key, {
        url,
        source,
        provider: metadata.provider,
        fullName: metadata.fullName,
        branch: metadata.defaultBranch,
      });
      return;
    }
    onUpdateRow(key, {
      url,
      source,
      provider: undefined,
      fullName: undefined,
      branch: "",
    });
  };
}

function AddRowButton({ onAddRow }: { onAddRow: () => void }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          onClick={onAddRow}
          aria-label="Add remote repository"
          data-testid="remote-add-row"
          className={cn(
            "h-7 w-7 inline-flex items-center justify-center rounded-md text-muted-foreground",
            "hover:bg-muted hover:text-foreground cursor-pointer",
          )}
        >
          <IconPlus className="h-3.5 w-3.5" />
        </button>
      </TooltipTrigger>
      <TooltipContent>Add another remote repository</TooltipContent>
    </Tooltip>
  );
}
