"use client";

import { IconLoader2 } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import type { LocalRepository } from "@/lib/types/http";

export type ManualValidation = {
  status: "idle" | "loading" | "success" | "error";
  message?: string;
  isValid?: boolean;
  path?: string;
};

type DiscoverRepoDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isLoading: boolean;
  filteredRepositories: LocalRepository[];
  repoSearch: string;
  onRepoSearchChange: (value: string) => void;
  selectedRepoPath: string | null;
  onSelectRepoPath: (path: string) => void;
  manualRepoPath: string;
  onManualRepoPathChange: (value: string) => void;
  manualValidation: ManualValidation;
  onValidateManualPath: () => void;
  isValidating: boolean;
  canSave: boolean;
  onConfirm: () => void;
};

function RepoListContent({
  isLoading,
  filteredRepositories,
  selectedRepoPath,
  onSelectRepoPath,
}: {
  isLoading: boolean;
  filteredRepositories: LocalRepository[];
  selectedRepoPath: string | null;
  onSelectRepoPath: (path: string) => void;
}) {
  if (isLoading)
    return (
      <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        Scanning repositories...
      </div>
    );
  if (filteredRepositories.length === 0)
    return <div className="p-3 text-sm text-muted-foreground">No repositories found.</div>;
  return (
    <>
      {filteredRepositories.map((repo) => (
        <button
          key={repo.path}
          type="button"
          className={`flex w-full flex-col px-3 py-2 text-left text-sm hover:bg-muted ${selectedRepoPath === repo.path ? "bg-muted" : ""}`}
          onClick={() => onSelectRepoPath(repo.path)}
        >
          <span className="font-medium">{repo.name}</span>
          <span className="text-xs text-muted-foreground">{repo.path}</span>
        </button>
      ))}
    </>
  );
}

export function DiscoverRepoDialog({
  open,
  onOpenChange,
  isLoading,
  filteredRepositories,
  repoSearch,
  onRepoSearchChange,
  selectedRepoPath,
  onSelectRepoPath,
  manualRepoPath,
  onManualRepoPathChange,
  manualValidation,
  onValidateManualPath,
  isValidating,
  canSave,
  onConfirm,
}: DiscoverRepoDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>Add Local Repository</DialogTitle>
          <DialogDescription>
            Select a discovered repository or provide an absolute path to validate.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>Discovered repositories</Label>
            <Input
              placeholder="Filter repositories..."
              value={repoSearch}
              onChange={(e) => onRepoSearchChange(e.target.value)}
            />
            <div className="max-h-56 overflow-auto rounded-md border border-border">
              <RepoListContent
                isLoading={isLoading}
                filteredRepositories={filteredRepositories}
                selectedRepoPath={selectedRepoPath}
                onSelectRepoPath={onSelectRepoPath}
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label>Manual path</Label>
            <div className="flex items-center gap-2">
              <Input
                placeholder="/absolute/path/to/repository"
                value={manualRepoPath}
                onChange={(e) => onManualRepoPathChange(e.target.value)}
              />
              <Button
                type="button"
                variant="outline"
                onClick={onValidateManualPath}
                disabled={!manualRepoPath.trim() || isValidating}
              >
                {isValidating ? "Checking..." : "Validate"}
              </Button>
            </div>
            {manualValidation.status === "error" && (
              <p className="text-xs text-destructive">{manualValidation.message}</p>
            )}
            {manualValidation.status === "success" && (
              <p className="text-xs text-emerald-500">{manualValidation.message}</p>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" onClick={onConfirm} disabled={!canSave}>
            Use Repository
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
