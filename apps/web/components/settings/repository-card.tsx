"use client";

import { useState } from "react";
import { Trans, useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
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

/**
 * The environment variable the dev script reads to learn its allocated port. It
 * is an identifier the user types into a shell command verbatim, so it travels
 * as an interpolated value rather than sitting inside the message.
 */
const DEV_SCRIPT_PORT_VAR = "$PORT";

/**
 * Shell script samples. Each is code the user edits and the executor runs
 * verbatim, so none of it is copy — the same call as `DEFAULT_DOCKERFILE` in the
 * executor profile editor. Hoisted out of the JSX so the intent is explicit
 * rather than resting on a guard exclusion pattern.
 */
const SETUP_SCRIPT_PLACEHOLDER = "#!/bin/bash\n# any manual setup you need";
const CLEANUP_SCRIPT_PLACEHOLDER = "#!/bin/bash\n# any manual clean up you need";
const DEV_SCRIPT_PLACEHOLDER = "#!/bin/bash\nnpm run dev -- --port $PORT";

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
  const { t } = useTranslation();
  return (
    <>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>{t("workspaces:repositoryName")}</Label>
          {/* Both placeholders are value shapes — a git repository name and an
              absolute filesystem path — so they stay in English. */}
          <Input
            value={repositoryName}
            onChange={(e) => onUpdate(repositoryId, { name: e.target.value })}
            placeholder="my-repo"
            data-settings-dirty={repositoryName !== (savedRepository?.name ?? "")}
          />
        </div>
        <div className="space-y-2">
          <Label>{t("workspaces:localPath")}</Label>
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
            <Label>{t("workspaces:worktreeBranchTemplate")}</Label>
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
          <Label htmlFor={`repo-pull-before-${repositoryId}`}>{t("workspaces:worktreeSync")}</Label>
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
                {t("workspaces:alwaysPullBeforeWorktree")}
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
  const { t } = useTranslation();
  return (
    <>
      {/* Every script placeholder below is a shell sample — a shebang plus a
          command the executor runs verbatim — so it is code, not copy. */}
      <div className="grid gap-4 md:grid-cols-2">
        <div className="space-y-2">
          <Label>{t("workspaces:setupScript")}</Label>
          <Textarea
            value={setupScript}
            onChange={(e) => onUpdate(repositoryId, { setup_script: e.target.value })}
            placeholder={SETUP_SCRIPT_PLACEHOLDER}
            rows={3}
            className="font-mono text-sm"
            data-settings-dirty={setupScript !== (savedRepository?.setup_script ?? "")}
          />
          <p className="text-xs text-muted-foreground">{t("workspaces:setupScriptHelp")}</p>
        </div>
        <div className="space-y-2">
          <Label>{t("workspaces:cleanupScript")}</Label>
          <Textarea
            value={cleanupScript}
            onChange={(e) => onUpdate(repositoryId, { cleanup_script: e.target.value })}
            placeholder={CLEANUP_SCRIPT_PLACEHOLDER}
            rows={3}
            className="font-mono text-sm"
            data-settings-dirty={cleanupScript !== (savedRepository?.cleanup_script ?? "")}
          />
          <p className="text-xs text-muted-foreground">{t("workspaces:cleanupScriptHelp")}</p>
        </div>
      </div>

      <div className="space-y-2">
        <Label>{t("workspaces:devScript")}</Label>
        <Textarea
          value={devScript}
          onChange={(e) => onUpdate(repositoryId, { dev_script: e.target.value })}
          placeholder={DEV_SCRIPT_PLACEHOLDER}
          rows={3}
          className="font-mono text-sm"
          data-settings-dirty={devScript !== (savedRepository?.dev_script ?? "")}
        />
        <p className="text-xs text-muted-foreground">
          <Trans i18nKey="workspaces:devScriptHelp" values={{ port: DEV_SCRIPT_PORT_VAR }}>
            <code className="px-1 py-0.5 bg-muted rounded" />
          </Trans>
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
  const { t } = useTranslation();
  return (
    <SettingsCard isDirty={isDirty}>
      <CardContent className="pt-6">
        <div className="space-y-5">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-center gap-2">
              <IconGitBranch className="h-4 w-4 text-muted-foreground" />
              <Label className="flex items-center gap-2">
                <span>{t("workspaces:repository")}</span>
                {isDirty && <UnsavedChangesBadge />}
              </Label>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="cursor-pointer"
              aria-label={t("workspaces:closeRepositoryEditor")}
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
              {t("workspaces:deleteRepository")}
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

// These two build every string in the collapsed card, and neither holds JSX, so
// `i18next/no-literal-string` could never see any of it — the lint count for this
// file reported zero for the whole block. `t` is a parameter rather than the
// module-level import so each label resolves when the card renders.
function buildRepoScriptsSummary(t: TFunction, repository: RepositoryWithScripts) {
  const setupScript = repository.setup_script ?? "";
  const cleanupScript = repository.cleanup_script ?? "";
  const devScript = repository.dev_script ?? "";
  const scriptsCount = repository.scripts.length;
  const hasSetupScript = Boolean(setupScript.trim());
  const hasCleanupScript = Boolean(cleanupScript.trim());
  const hasDevScript = Boolean(devScript.trim());
  const showScriptsSummary = scriptsCount > 0 || hasSetupScript || hasCleanupScript || hasDevScript;
  // Was `custom script${count === 1 ? "" : "s"}` — an English morpheme built at
  // the call site, which leaves a translator no way to express a third form.
  const scriptsLabel =
    scriptsCount === 0
      ? t("workspaces:noCustomScripts")
      : t("workspaces:customScripts", { count: scriptsCount });
  return {
    scriptsCount,
    hasSetupScript,
    hasCleanupScript,
    hasDevScript,
    showScriptsSummary,
    scriptsLabel,
  };
}

function buildRepoPreviewData(t: TFunction, repository: RepositoryWithScripts) {
  const repositoryName = repository.name ?? "";
  // `source_type` is the wire value; only its badge label is copy.
  const sourceLabel =
    repository.source_type === "local" ? t("workspaces:sourceLocal") : t("workspaces:sourceRemote");
  // The path and the `owner/name` slug are user data / git metadata; only the
  // two "nothing recorded" fallbacks are copy.
  const subtitle =
    repository.source_type === "local"
      ? repository.local_path || t("workspaces:localPathNotSet")
      : [repository.provider_owner, repository.provider_name].filter(Boolean).join("/") ||
        repository.provider ||
        t("workspaces:remoteRepository");
  return {
    repositoryName,
    sourceLabel,
    subtitle,
    ...buildRepoScriptsSummary(t, repository),
  };
}

function RepositoryPreview({
  repository,
  isDirty,
  deleteLoading,
  onOpenDelete,
  open,
}: RepositoryPreviewProps) {
  const { t } = useTranslation();
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
  } = buildRepoPreviewData(t, repository);

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
                <h4 className="font-medium truncate">
                  {repositoryName || t("workspaces:untitledRepository")}
                </h4>
                <Badge variant="secondary" className="text-xs">
                  {sourceLabel}
                </Badge>
                {isDirty && <UnsavedChangesBadge />}
              </div>
              <div className="text-xs text-muted-foreground mt-1 truncate">{subtitle}</div>
              {showScriptsSummary ? (
                <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground mt-1">
                  {scriptsCount > 0 && <span>{scriptsLabel}</span>}
                  {hasSetupScript && <span>{t("workspaces:buildScriptChip")}</span>}
                  {hasCleanupScript && <span>{t("workspaces:cleanupScriptChip")}</span>}
                  {hasDevScript && <span>{t("workspaces:devScriptChip")}</span>}
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
              {t("workspaces:edit")}
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
              {t("workspaces:delete")}
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
  const { t } = useTranslation();
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
        title: t("workspaces:failedToCheckRepositorySessions"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
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
        title: t("workspaces:failedToDeleteRepository"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
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
  const { t } = useTranslation();
  const [isEditing, setIsEditing] = useState(() => autoOpen);
  const saveRequest = useRequest(() => onSave(repository.id));
  const isDirty = isRepositoryDirty || areScriptsDirty;
  const deleteState = useRepositoryDelete(repository.id, onDelete, () => setIsEditing(false));

  const handleSave = async () => {
    try {
      await saveRequest.run();
    } catch (error) {
      toast({
        title: t("workspaces:failedToSaveRepository"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
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
