"use client";

import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  if (isLoading)
    return (
      <div className="flex items-center gap-2 p-3 text-sm text-muted-foreground">
        <IconLoader2 className="h-4 w-4 animate-spin" />
        {t("workspaces:scanningRepositories")}
      </div>
    );
  if (filteredRepositories.length === 0)
    return (
      <div className="p-3 text-sm text-muted-foreground">{t("workspaces:noRepositoriesFound")}</div>
    );
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
  const { t } = useTranslation();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("workspaces:addLocalRepository")}</DialogTitle>
          <DialogDescription>{t("workspaces:discoverRepositoryDescription")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label>{t("workspaces:discoveredRepositories")}</Label>
            <Input
              placeholder={t("workspaces:filterRepositories")}
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
            <Label>{t("workspaces:manualPath")}</Label>
            <div className="flex items-center gap-2">
              {/* An absolute filesystem path is the value shape this field
                  expects, not prose — left in English deliberately. */}
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
                {isValidating ? t("workspaces:checking") : t("workspaces:validate")}
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
            {t("common:cancel")}
          </Button>
          <Button type="button" onClick={onConfirm} disabled={!canSave}>
            {t("workspaces:useRepository")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
