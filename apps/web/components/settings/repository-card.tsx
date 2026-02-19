"use client";

import { useState } from "react";
import { IconEdit, IconGitBranch, IconPlus, IconTrash, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Card, CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@kandev/ui/dialog";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { UnsavedChangesBadge, UnsavedSaveButton } from "@/components/settings/unsaved-indicator";
import { EditableCard } from "@/components/settings/editable-card";
import type { Repository, RepositoryScript } from "@/lib/types/http";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type RepoFieldsBaseProps = {
  repositoryId: string;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

type RepositoryBasicFieldsProps = RepoFieldsBaseProps & {
  repositoryName: string;
  repositoryLocalPath: string;
  sourceType: string;
  worktreeBranchPrefix: string;
  pullBeforeWorktree: boolean;
};

function RepositoryBasicFields({
  repositoryId,
  onUpdate,
  repositoryName,
  repositoryLocalPath,
  sourceType,
  worktreeBranchPrefix,
  pullBeforeWorktree,
}: RepositoryBasicFieldsProps) {
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>Repository Name</Label>
          <Input
            value={repositoryName}
            onChange={(e) => onUpdate(repositoryId, { name: e.target.value })}
            placeholder="my-repo"
          />
        </div>
        <div className="space-y-2">
          <Label>Local Path</Label>
          <Input
            value={repositoryLocalPath}
            onChange={(e) => onUpdate(repositoryId, { local_path: e.target.value })}
            placeholder="/path/to/repository"
            disabled={sourceType !== "local"}
          />
        </div>
      </div>

      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>Worktree Branch Prefix</Label>
          <Input
            value={worktreeBranchPrefix}
            onChange={(e) => onUpdate(repositoryId, { worktree_branch_prefix: e.target.value })}
            placeholder="feature/"
          />
          <p className="text-xs text-muted-foreground">
            Used for new worktree branches. Leave empty to use the default. Branches are generated
            as {"{prefix}{sanitized-title}-{rand}"}.
          </p>
        </div>
        <div className="space-y-2">
          <Label htmlFor={`repo-pull-before-${repositoryId}`}>Worktree Sync</Label>
          <div className="flex items-start gap-2 pt-2">
            <Checkbox
              id={`repo-pull-before-${repositoryId}`}
              checked={pullBeforeWorktree}
              onCheckedChange={(checked) =>
                onUpdate(repositoryId, { pull_before_worktree: checked === true })
              }
            />
            <div className="space-y-1">
              <Label
                htmlFor={`repo-pull-before-${repositoryId}`}
                className="text-sm text-muted-foreground cursor-pointer"
              >
                Always pull before creating a new worktree
              </Label>
              <p className="text-xs text-muted-foreground">
                Keeps the base branch up to date with the remote before spawning new sessions.
              </p>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

type RepositoryScriptFieldsProps = RepoFieldsBaseProps & {
  setupScript: string;
  cleanupScript: string;
  devScript: string;
};

function RepositoryScriptFields({
  repositoryId,
  onUpdate,
  setupScript,
  cleanupScript,
  devScript,
}: RepositoryScriptFieldsProps) {
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>Setup Script</Label>
          <Textarea
            value={setupScript}
            onChange={(e) => onUpdate(repositoryId, { setup_script: e.target.value })}
            placeholder="#!/bin/bash&#10;# any manual setup you need"
            rows={3}
            className="font-mono text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Runs when the repo is cloned or a git worktree is created.
          </p>
        </div>
        <div className="space-y-2">
          <Label>Cleanup Script</Label>
          <Textarea
            value={cleanupScript}
            onChange={(e) => onUpdate(repositoryId, { cleanup_script: e.target.value })}
            placeholder="#!/bin/bash&#10;# any manual clean up you need"
            rows={3}
            className="font-mono text-sm"
          />
          <p className="text-xs text-muted-foreground">
            Runs when the task is completed to clean up the workspace.
          </p>
        </div>
      </div>

      <div className="space-y-2">
        <Label>Dev Script</Label>
        <Textarea
          value={devScript}
          onChange={(e) => onUpdate(repositoryId, { dev_script: e.target.value })}
          placeholder="#!/bin/bash&#10;npm run dev -- --port $PORT"
          rows={3}
          className="font-mono text-sm"
        />
        <p className="text-xs text-muted-foreground">
          Used to start the preview dev server for this repository. Use{" "}
          <code className="px-1 py-0.5 bg-muted rounded">$PORT</code> for automatic port allocation.
        </p>
      </div>
    </>
  );
}

type RepositoryCustomScriptsProps = {
  repositoryId: string;
  scripts: RepositoryScript[];
  areScriptsDirty: boolean;
  onAddScript: (repoId: string) => void;
  onUpdateScript: (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => void;
  onDeleteScript: (repoId: string, scriptId: string) => void;
};

function RepositoryCustomScripts({
  repositoryId,
  scripts,
  areScriptsDirty,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
}: RepositoryCustomScriptsProps) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <Label className="flex items-center gap-2">
          <span>Custom Scripts</span>
          {areScriptsDirty && <UnsavedChangesBadge />}
        </Label>
        <Button type="button" variant="outline" size="sm" onClick={() => onAddScript(repositoryId)}>
          <IconPlus className="h-4 w-4 mr-1" />
          Add Script
        </Button>
      </div>
      <div className="space-y-3">
        {scripts.length === 0 ? (
          <p className="text-sm text-muted-foreground">No scripts yet.</p>
        ) : (
          scripts.map((script) => (
            <div key={script.id} className="grid gap-2">
              <div className="flex items-center gap-2">
                <Input
                  value={script.name ?? ""}
                  onChange={(e) =>
                    onUpdateScript(repositoryId, script.id, { name: e.target.value })
                  }
                  placeholder="Script name"
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  onClick={() => onDeleteScript(repositoryId, script.id)}
                >
                  <IconX className="h-4 w-4" />
                </Button>
              </div>
              <Textarea
                value={script.command ?? ""}
                onChange={(e) =>
                  onUpdateScript(repositoryId, script.id, { command: e.target.value })
                }
                placeholder="#!/bin/bash&#10;npm run dev"
                rows={3}
                className="font-mono text-sm"
              />
            </div>
          ))
        )}
      </div>
    </div>
  );
}

type RepositoryEditViewProps = {
  repository: RepositoryWithScripts;
  isDirty: boolean;
  areScriptsDirty: boolean;
  saveRequest: { isLoading: boolean; status: "idle" | "loading" | "success" | "error" };
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
  onAddScript: (repoId: string) => void;
  onUpdateScript: (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => void;
  onDeleteScript: (repoId: string, scriptId: string) => void;
  onSave: (close: () => void) => void;
  onOpenDelete: () => void;
  deleteLoading: boolean;
  close: () => void;
};

function RepositoryEditView({
  repository,
  isDirty,
  areScriptsDirty,
  saveRequest,
  onUpdate,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
  onSave,
  onOpenDelete,
  deleteLoading,
  close,
}: RepositoryEditViewProps) {
  return (
    <Card>
      <CardContent className="pt-6">
        <div className="space-y-5">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-2">
              <IconGitBranch className="h-4 w-4 text-muted-foreground" />
              <Label className="flex items-center gap-2">
                <span>Repository</span>
                {isDirty && <UnsavedChangesBadge />}
              </Label>
            </div>
            <div className="flex items-center gap-2">
              <UnsavedSaveButton
                isDirty={isDirty}
                isLoading={saveRequest.isLoading}
                status={saveRequest.status}
                cleanLabel="Close"
                onClick={isDirty ? () => onSave(close) : close}
              />
            </div>
          </div>

          <RepositoryBasicFields
            repositoryId={repository.id}
            onUpdate={onUpdate}
            repositoryName={repository.name ?? ""}
            repositoryLocalPath={repository.local_path ?? ""}
            sourceType={repository.source_type}
            worktreeBranchPrefix={repository.worktree_branch_prefix ?? ""}
            pullBeforeWorktree={repository.pull_before_worktree ?? true}
          />

          <RepositoryScriptFields
            repositoryId={repository.id}
            onUpdate={onUpdate}
            setupScript={repository.setup_script ?? ""}
            cleanupScript={repository.cleanup_script ?? ""}
            devScript={repository.dev_script ?? ""}
          />

          <RepositoryCustomScripts
            repositoryId={repository.id}
            scripts={repository.scripts}
            areScriptsDirty={areScriptsDirty}
            onAddScript={onAddScript}
            onUpdateScript={onUpdateScript}
            onDeleteScript={onDeleteScript}
          />

          <div className="flex justify-end">
            <Button
              type="button"
              variant="destructive"
              onClick={onOpenDelete}
              disabled={deleteLoading}
            >
              <IconTrash className="h-4 w-4 mr-2" />
              Delete Repository
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

type RepositoryPreviewProps = {
  repository: RepositoryWithScripts;
  isDirty: boolean;
  deleteLoading: boolean;
  onOpenDelete: () => void;
  open: () => void;
};

function buildRepoScriptsSummary(repository: RepositoryWithScripts) {
  const setupScript = repository.setup_script ?? "";
  const cleanupScript = repository.cleanup_script ?? "";
  const devScript = repository.dev_script ?? "";
  const scriptsCount = repository.scripts.length;
  const hasSetupScript = Boolean(setupScript.trim());
  const hasCleanupScript = Boolean(cleanupScript.trim());
  const hasDevScript = Boolean(devScript.trim());
  const showScriptsSummary = scriptsCount > 0 || hasSetupScript || hasCleanupScript || hasDevScript;
  const scriptsLabel =
    scriptsCount === 0
      ? "No custom scripts"
      : `${scriptsCount} custom script${scriptsCount === 1 ? "" : "s"}`;
  return {
    scriptsCount,
    hasSetupScript,
    hasCleanupScript,
    hasDevScript,
    showScriptsSummary,
    scriptsLabel,
  };
}

function buildRepoPreviewData(repository: RepositoryWithScripts) {
  const repositoryName = repository.name ?? "";
  const worktreeBranchPrefix = repository.worktree_branch_prefix ?? "";
  const sourceLabel = repository.source_type === "local" ? "Local" : "Remote";
  const subtitle =
    repository.source_type === "local"
      ? repository.local_path || "Local path not set"
      : [repository.provider_owner, repository.provider_name].filter(Boolean).join("/") ||
        repository.provider ||
        "Remote repository";
  return {
    repositoryName,
    worktreeBranchPrefix,
    sourceLabel,
    subtitle,
    ...buildRepoScriptsSummary(repository),
  };
}

function RepositoryPreview({
  repository,
  isDirty,
  deleteLoading,
  onOpenDelete,
  open,
}: RepositoryPreviewProps) {
  const {
    repositoryName,
    worktreeBranchPrefix,
    scriptsCount,
    hasSetupScript,
    hasCleanupScript,
    hasDevScript,
    showScriptsSummary,
    scriptsLabel,
    sourceLabel,
    subtitle,
  } = buildRepoPreviewData(repository);

  return (
    <Card>
      <CardContent className="py-4 cursor-pointer" onClick={open}>
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3 min-w-0">
            <div className="p-2 bg-muted rounded-md">
              <IconGitBranch className="h-4 w-4 text-muted-foreground" />
            </div>
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <h4 className="font-medium truncate">{repositoryName || "Untitled repository"}</h4>
                <Badge variant="secondary" className="text-xs">
                  {sourceLabel}
                </Badge>
                {worktreeBranchPrefix.trim() && worktreeBranchPrefix.trim() !== "feature/" ? (
                  <Badge variant="outline" className="text-xs">
                    {worktreeBranchPrefix.trim()}
                  </Badge>
                ) : null}
                {isDirty && <UnsavedChangesBadge />}
              </div>
              <div className="text-xs text-muted-foreground mt-1 truncate">{subtitle}</div>
              {showScriptsSummary ? (
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground mt-1">
                  {scriptsCount > 0 && <span>{scriptsLabel}</span>}
                  {hasSetupScript && <span>build script</span>}
                  {hasCleanupScript && <span>cleanup script</span>}
                  {hasDevScript && <span>dev script</span>}
                </div>
              ) : null}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                open();
              }}
            >
              <IconEdit className="h-4 w-4" />
              Edit
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="cursor-pointer"
              onClick={(event) => {
                event.stopPropagation();
                onOpenDelete();
              }}
              disabled={deleteLoading}
            >
              <IconTrash className="h-4 w-4" />
              Delete
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

type DeleteRepositoryDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDelete: () => void;
};

function DeleteRepositoryDialog({ open, onOpenChange, onDelete }: DeleteRepositoryDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete repository</DialogTitle>
          <DialogDescription>
            This will remove the repository and its scripts. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="button" variant="destructive" onClick={onDelete}>
            Delete Repository
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type RepositoryCardProps = {
  repository: RepositoryWithScripts;
  isRepositoryDirty: boolean;
  areScriptsDirty: boolean;
  autoOpen?: boolean;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
  onAddScript: (repoId: string) => void;
  onUpdateScript: (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => void;
  onDeleteScript: (repoId: string, scriptId: string) => void;
  onSave: (repoId: string) => Promise<void>;
  onDelete: (repoId: string) => Promise<void> | void;
};

export function RepositoryCard({
  repository,
  isRepositoryDirty,
  areScriptsDirty,
  autoOpen = false,
  onUpdate,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
  onSave,
  onDelete,
}: RepositoryCardProps) {
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [isEditing, setIsEditing] = useState(() => autoOpen);
  const saveRequest = useRequest(() => onSave(repository.id));
  const deleteRequest = useRequest(async () => {
    await onDelete(repository.id);
  });
  const isDirty = isRepositoryDirty || areScriptsDirty;

  const handleSave = async (close: () => void) => {
    try {
      await saveRequest.run();
      close();
    } catch (error) {
      toast({
        title: "Failed to save repository",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    }
  };

  const handleDelete = async () => {
    try {
      await deleteRequest.run();
      setDeleteOpen(false);
      setIsEditing(false);
    } catch (error) {
      toast({
        title: "Failed to delete repository",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    }
  };

  return (
    <>
      <EditableCard
        isEditing={isEditing}
        historyId={`repo-${repository.id}`}
        onOpen={() => setIsEditing(true)}
        onClose={() => setIsEditing(false)}
        renderEdit={({ close }) => (
          <RepositoryEditView
            repository={repository}
            isDirty={isDirty}
            areScriptsDirty={areScriptsDirty}
            saveRequest={saveRequest}
            onUpdate={onUpdate}
            onAddScript={onAddScript}
            onUpdateScript={onUpdateScript}
            onDeleteScript={onDeleteScript}
            onSave={handleSave}
            onOpenDelete={() => setDeleteOpen(true)}
            deleteLoading={deleteRequest.isLoading}
            close={close}
          />
        )}
        renderPreview={({ open }) => (
          <RepositoryPreview
            repository={repository}
            isDirty={isDirty}
            deleteLoading={deleteRequest.isLoading}
            onOpenDelete={() => setDeleteOpen(true)}
            open={open}
          />
        )}
      />
      <DeleteRepositoryDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onDelete={handleDelete}
      />
    </>
  );
}
