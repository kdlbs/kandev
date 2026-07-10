"use client";

import { IconPlus, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import type { Repository, WorktreeFile, WorktreeFileMode } from "@/lib/types/http";

export type RepositoryWorktreeFilesProps = {
  repositoryId: string;
  worktreeFiles: WorktreeFile[];
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

function replaceAt(
  files: WorktreeFile[],
  index: number,
  patch: Partial<WorktreeFile>,
): WorktreeFile[] {
  return files.map((file, i) => (i === index ? { ...file, ...patch } : file));
}

/**
 * RepositoryWorktreeFiles edits the per-file list of paths materialized into
 * each new worktree. Each file chooses its own mode: copied (isolated per
 * worktree) or symlinked (centralized, shared across worktrees).
 */
export function RepositoryWorktreeFiles({
  repositoryId,
  worktreeFiles,
  onUpdate,
}: RepositoryWorktreeFilesProps) {
  const files = worktreeFiles ?? [];

  const addFile = () =>
    onUpdate(repositoryId, {
      worktree_files: [...files, { path: "", mode: "copy" as WorktreeFileMode }],
    });
  const removeFile = (index: number) =>
    onUpdate(repositoryId, { worktree_files: files.filter((_, i) => i !== index) });
  const changePath = (index: number, path: string) =>
    onUpdate(repositoryId, { worktree_files: replaceAt(files, index, { path }) });
  const changeMode = (index: number, mode: WorktreeFileMode) =>
    onUpdate(repositoryId, { worktree_files: replaceAt(files, index, { mode }) });

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <Label>Worktree Files</Label>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="cursor-pointer"
          onClick={addFile}
        >
          <IconPlus className="h-4 w-4 mr-1" />
          Add File
        </Button>
      </div>

      {files.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No files configured. Existing repositories keep the current behavior.
        </p>
      ) : (
        <div className="space-y-2">
          {files.map((file, index) => (
            // Key by path for filled rows so removing a middle row doesn't shift
            // input focus/cursor; empty (new) rows fall back to index.
            <div
              key={file.path ? `p:${file.path}` : `new:${index}`}
              className="flex items-center gap-2"
            >
              <Input
                value={file.path ?? ""}
                onChange={(e) => changePath(index, e.target.value)}
                placeholder=".env.local"
                className="font-mono text-sm"
              />
              <Select
                value={file.mode || "copy"}
                onValueChange={(v) => changeMode(index, v as WorktreeFileMode)}
              >
                <SelectTrigger className="w-36 shrink-0">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="copy">Copy</SelectItem>
                  <SelectItem value="symlink">Symlink</SelectItem>
                </SelectContent>
              </Select>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="cursor-pointer"
                aria-label="Remove file"
                onClick={() => removeFile(index)}
              >
                <IconX className="h-4 w-4" />
              </Button>
            </div>
          ))}
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Repo-relative paths materialized into each new worktree (missing files are skipped).{" "}
        <strong>Copy</strong> = an isolated file per worktree. <strong>Symlink</strong> = a link
        back to the file in the main repository so shared files (e.g.{" "}
        <code className="px-1 py-0.5 bg-muted rounded">.env.local</code>) stay centrally managed and
        updates appear in every worktree.
      </p>
    </div>
  );
}
