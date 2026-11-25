"use client";

import { useTranslation } from "react-i18next";
import { IconGitBranch } from "@tabler/icons-react";
import { SettingsSection } from "@/components/settings/settings-section";
import { useSearchParams } from "@/lib/routing/client-router";
import { readMCPContextHint } from "@/lib/mcp/management-links";
import { RepositoryCard } from "@/components/settings/repository-card";
import type { Workspace } from "@/lib/types/http";
import { AddRepositoryMenu } from "./workspace-add-repository-menu";
import { areRepositoryScriptsDirty, isRepositoryDirty } from "./workspace-repositories-dirty";
import type { useWorkspaceRepositoriesPage } from "./workspace-repositories-client";

type WorkspaceRepositoriesSectionProps = {
  workspaceId: Workspace["id"];
  readOnly: boolean;
  state: Pick<
    ReturnType<typeof useWorkspaceRepositoriesPage>,
    | "repositoryItems"
    | "savedRepositoriesById"
    | "handleUpdateRepository"
    | "handleAddRepositoryScript"
    | "handleUpdateRepositoryScript"
    | "handleDeleteRepositoryScript"
    | "handleSaveRepository"
    | "handleDiscardRepository"
    | "handleDeleteRepository"
    | "openDialog"
    | "setRemoteRepoDialogOpen"
  >;
};

export function WorkspaceRepositoriesSection({
  workspaceId,
  readOnly,
  state,
}: WorkspaceRepositoriesSectionProps) {
  const { t } = useTranslation();
  const params = useSearchParams();
  const hint = readMCPContextHint(params, "repositoryId");
  const target =
    hint.value &&
    state.repositoryItems.find(
      (repo) => repo.id === hint.value && repo.workspace_id === workspaceId,
    );
  const {
    repositoryItems,
    savedRepositoriesById,
    handleUpdateRepository,
    handleAddRepositoryScript,
    handleUpdateRepositoryScript,
    handleDeleteRepositoryScript,
    handleSaveRepository,
    handleDiscardRepository,
    handleDeleteRepository,
    openDialog,
    setRemoteRepoDialogOpen,
  } = state;

  return (
    <SettingsSection
      divided
      framed={false}
      icon={<IconGitBranch className="h-5 w-5" />}
      title={t("workspaces:repositories")}
      description={
        readOnly
          ? t("workspaces:repositoriesReadOnlyImprove")
          : t("workspaces:repositoriesInThisWorkspace")
      }
      action={
        readOnly ? undefined : (
          <AddRepositoryMenu
            onAdd={(kind) => (kind === "local" ? openDialog() : setRemoteRepoDialogOpen(true))}
          />
        )
      }
    >
      {hint.present && !target && (
        <p role="alert" className="text-sm text-muted-foreground">
          {t("settings:mcpOwnerUnavailable")}
        </p>
      )}
      <div className="grid gap-3">
        {repositoryItems.map((repo) => (
          <RepositoryCard
            key={repo.id}
            repository={repo}
            workspaceId={workspaceId}
            savedRepository={savedRepositoriesById.get(repo.id)}
            isRepositoryDirty={isRepositoryDirty(repo, savedRepositoriesById.get(repo.id))}
            areScriptsDirty={areRepositoryScriptsDirty(repo, savedRepositoriesById.get(repo.id))}
            autoOpen={Boolean(repo.__autoOpen) || Boolean(target && target.id === repo.id)}
            readOnly={readOnly}
            onUpdate={handleUpdateRepository}
            onAddScript={handleAddRepositoryScript}
            onUpdateScript={handleUpdateRepositoryScript}
            onDeleteScript={handleDeleteRepositoryScript}
            onSave={handleSaveRepository}
            onDiscard={handleDiscardRepository}
            onDelete={handleDeleteRepository}
          />
        ))}
      </div>
    </SettingsSection>
  );
}
