"use client";

import { useState } from "react";
import { IconEdit, IconGitBranch, IconTrash, IconX } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { CardContent } from "@kandev/ui/card";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { Textarea } from "@kandev/ui/textarea";
import { useRequest } from "@/lib/http/use-request";
import { useToast } from "@/components/toast-provider";
import { UnsavedChangesBadge } from "@/components/settings/unsaved-indicator";
import { useSettingsSaveContributor } from "@/components/settings/settings-save-provider";
import { SettingsCard } from "@/components/settings/settings-card";
import { EditableCard } from "@/components/settings/editable-card";
import { RepositoryBranchTemplateHelp } from "@/components/settings/repository-branch-template-help";
import { DeleteRepositoryDialog } from "@/components/settings/repository-delete-dialog";
import { CopyFilesField } from "@/components/settings/repository-copy-files-help";
import { RepositoryCustomScripts } from "@/components/settings/repository-custom-scripts";
import { getRepositoryActiveSessionCountAction } from "@/app/actions/workspaces";
import type { Repository, RepositoryScript } from "@/lib/types/http";
import { defaultWorktreeBranchTemplate } from "@/lib/worktree-branch-template";

type RepositoryWithScripts = Repository & { scripts: RepositoryScript[] };

type RepoFieldsBaseProps = {
  repositoryId: string;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
};

type RepositoryBasicFieldsProps = RepoFieldsBaseProps & {
  savedRepository?: RepositoryWithScripts;
  repositoryName: string;
  repositoryLocalPath: string;
  sourceType: string;
  worktreeBranchTemplate: string;
  pullBeforeWorktree: boolean;
};

function RepositoryBasicFields({
  repositoryId,
  savedRepository,
  onUpdate,
  repositoryName,
  repositoryLocalPath,
  sourceType,
  worktreeBranchTemplate,
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
            data-settings-dirty={repositoryName !== (savedRepository?.name ?? "")}
          />
        </div>
        <div className="space-y-2">
          <Label>Local Path</Label>
          <Input
            value={repositoryLocalPath}
            onChange={(e) => onUpdate(repositoryId, { local_path: e.target.value })}
            placeholder="/path/to/repository"
            disabled={sourceType !== "local"}
            data-settings-dirty={repositoryLocalPath !== (savedRepository?.local_path ?? "")}
          />
        </div>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <div className="flex items-center gap-1.5">
            <Label>Worktree Branch Template</Label>
            <RepositoryBranchTemplateHelp />
          </div>
          <Input
            value={worktreeBranchTemplate}
            onChange={(e) => onUpdate(repositoryId, { worktree_branch_template: e.target.value })}
            placeholder={defaultWorktreeBranchTemplate}
            data-settings-dirty={
              worktreeBranchTemplate !==
              (savedRepository?.worktree_branch_template ?? defaultWorktreeBranchTemplate)
            }
          />
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
              data-settings-dirty={
                pullBeforeWorktree !== (savedRepository?.pull_before_worktree ?? true)
              }
            />
            <div className="space-y-1">
              <Label
                htmlFor={`repo-pull-before-${repositoryId}`}
                className="text-sm text-muted-foreground cursor-pointer"
              >
                Always pull before creating a new worktree
              </Label>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

type RepositoryScriptFieldsProps = RepoFieldsBaseProps & {
  savedRepository?: RepositoryWithScripts;
  setupScript: string;
  cleanupScript: string;
  devScript: string;
  copyFiles: string;
};

function RepositoryScriptFields({
  repositoryId,
  savedRepository,
  onUpdate,
  setupScript,
  cleanupScript,
  devScript,
  copyFiles,
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
            data-settings-dirty={setupScript !== (savedRepository?.setup_script ?? "")}
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
            data-settings-dirty={cleanupScript !== (savedRepository?.cleanup_script ?? "")}
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
          data-settings-dirty={devScript !== (savedRepository?.dev_script ?? "")}
        />
        <p className="text-xs text-muted-foreground">
          Used to start the preview dev server for this repository. Use{" "}
          <code className="px-1 py-0.5 bg-muted rounded">$PORT</code> for automatic port allocation.
        </p>
      </div>

      <CopyFilesField
        repositoryId={repositoryId}
        copyFiles={copyFiles}
        isDirty={copyFiles !== (savedRepository?.copy_files ?? "")}
        onUpdate={onUpdate}
      />
    </>
  );
}

type RepositoryEditViewProps = {
  repository: RepositoryWithScripts;
  savedRepository?: RepositoryWithScripts;
  isDirty: boolean;
  areScriptsDirty: boolean;
  onUpdate: (repoId: string, updates: Partial<Repository>) => void;
  onAddScript: (repoId: string) => void;
  onUpdateScript: (repoId: string, scriptId: string, updates: Partial<RepositoryScript>) => void;
  onDeleteScript: (repoId: string, scriptId: string) => void;
  onOpenDelete: () => void;
  deleteLoading: boolean;
  close: () => void;
};

function RepositoryEditView({
  repository,
  savedRepository,
  isDirty,
  areScriptsDirty,
  onUpdate,
  onAddScript,
  onUpdateScript,
  onDeleteScript,
  onOpenDelete,
  deleteLoading,
  close,
}: RepositoryEditViewProps) {
  return (
    <SettingsCard isDirty={isDirty}>
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
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="cursor-pointer"
              aria-label="Close repository editor"
              onClick={close}
            >
              <IconX className="h-4 w-4" />
            </Button>
          </div>

          <RepositoryBasicFields
            repositoryId={repository.id}
            savedRepository={savedRepository}
            onUpdate={onUpdate}
            repositoryName={repository.name ?? ""}
            repositoryLocalPath={repository.local_path ?? ""}
            sourceType={repository.source_type}
            worktreeBranchTemplate={
              repository.worktree_branch_template ?? defaultWorktreeBranchTemplate
            }
            pullBeforeWorktree={repository.pull_before_worktree ?? true}
          />

          <RepositoryScriptFields
            repositoryId={repository.id}
            savedRepository={savedRepository}
            onUpdate={onUpdate}
            setupScript={repository.setup_script ?? ""}
            cleanupScript={repository.cleanup_script ?? ""}
            devScript={repository.dev_script ?? ""}
            copyFiles={repository.copy_files ?? ""}
          />

          <RepositoryCustomScripts
            repositoryId={repository.id}
            scripts={repository.scripts}
            savedScripts={savedRepository?.scripts}
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
    </SettingsCard>
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
  const sourceLabel = repository.source_type === "local" ? "Local" : "Remote";
  const subtitle =
    repository.source_type === "local"
      ? repository.local_path || "Local path not set"
      : [repository.provider_owner, repository.provider_name].filter(Boolean).join("/") ||
        repository.provider ||
        "Remote repository";
  return {
    repositoryName,
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
    <SettingsCard isDirty={isDirty}>
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
    </SettingsCard>
  );
}

type RepositoryCardProps = {
  repository: RepositoryWithScripts;
  savedRepository?: RepositoryWithScripts;
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

function useRepositoryDelete(
  repositoryId: string,
  onDelete: (repoId: string) => Promise<void> | void,
  onDeleted: () => void,
) {
  const { toast } = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [activeSessionCount, setActiveSessionCount] = useState(0);
  const [checkingCount, setCheckingCount] = useState(false);
  const deleteRequest = useRequest(async () => {
    await onDelete(repositoryId);
  });

  const handleOpenDelete = async () => {
    // Reset count up-front so a stale value from a previous open can't flash
    // the destructive button between dialog mount and the async fetch
    // resolving with the fresh count.
    setActiveSessionCount(0);
    if (repositoryId.startsWith("temp-repo-")) {
      setDeleteOpen(true);
      return;
    }
    setCheckingCount(true);
    try {
      const { active_session_count } = await getRepositoryActiveSessionCountAction(repositoryId);
      setActiveSessionCount(active_session_count);
      setDeleteOpen(true);
    } catch (error) {
      toast({
        title: "Failed to check repository sessions",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    } finally {
      setCheckingCount(false);
    }
  };

  const handleDelete = async () => {
    try {
      await deleteRequest.run();
      setDeleteOpen(false);
      onDeleted();
    } catch (error) {
      toast({
        title: "Failed to delete repository",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
    }
  };

  return {
    deleteOpen,
    setDeleteOpen,
    activeSessionCount,
    handleOpenDelete,
    handleDelete,
    buttonLoading: deleteRequest.isLoading || checkingCount,
    dialogDeleteLoading: deleteRequest.isLoading,
  };
}

export function RepositoryCard({
  repository,
  savedRepository,
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
  const [isEditing, setIsEditing] = useState(() => autoOpen);
  const saveRequest = useRequest(() => onSave(repository.id));
  const isDirty = isRepositoryDirty || areScriptsDirty;
  const deleteState = useRepositoryDelete(repository.id, onDelete, () => setIsEditing(false));

  const handleSave = async () => {
    try {
      await saveRequest.run();
    } catch (error) {
      toast({
        title: "Failed to save repository",
        description: error instanceof Error ? error.message : "Request failed",
        variant: "error",
      });
      throw error;
    }
  };
  useSettingsSaveContributor({
    id: `repository:${repository.id}`,
    revision: JSON.stringify(repository),
    isDirty,
    save: handleSave,
    discard: () => undefined,
  });

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
            savedRepository={savedRepository}
            isDirty={isDirty}
            areScriptsDirty={areScriptsDirty}
            onUpdate={onUpdate}
            onAddScript={onAddScript}
            onUpdateScript={onUpdateScript}
            onDeleteScript={onDeleteScript}
            onOpenDelete={deleteState.handleOpenDelete}
            deleteLoading={deleteState.buttonLoading}
            close={close}
          />
        )}
        renderPreview={({ open }) => (
          <RepositoryPreview
            repository={repository}
            isDirty={isDirty}
            deleteLoading={deleteState.buttonLoading}
            onOpenDelete={deleteState.handleOpenDelete}
            open={open}
          />
        )}
      />
      <DeleteRepositoryDialog
        open={deleteState.deleteOpen}
        onOpenChange={deleteState.setDeleteOpen}
        onDelete={deleteState.handleDelete}
        activeSessionCount={deleteState.activeSessionCount}
        deleteLoading={deleteState.dialogDeleteLoading}
      />
    </>
  );
}
