"use client";

import { useCallback, useState } from "react";
import { CreateLocalRepositorySurface } from "@/components/create-local-repository-surface";
import { RepositoryDiscoveryDialog } from "@/components/repository-discovery-dialog";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useRepositoryDiscovery } from "@/hooks/domains/workspace/use-repository-discovery";
import { addDesktopDiscoveryRootAction } from "@/app/actions/workspaces";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import type { WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";
import type { LocalRepository, Repository } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

type Props = {
  row: WorkspaceSourceRow;
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  workspaceId: string | null;
  canCreateRepository: boolean;
  repositoriesRefreshing: boolean;
  onRefreshRepositories: () => void;
  onUpdate: (key: string, patch: Partial<WorkspaceSourceRow>) => void;
};

function useSavedRepositoryDiscovery(workspaceId: string | null) {
  const [discoverySettingsOpen, setDiscoverySettingsOpen] = useState(false);
  const [addingHome, setAddingHome] = useState(false);
  const discovery = useRepositoryDiscovery(workspaceId);
  const { toast } = useToast();
  const { t } = useTranslation();

  const handleAddHomeAndOpenDiscovery = useCallback(async () => {
    setDiscoverySettingsOpen(true);
    setAddingHome(true);
    try {
      await addDesktopDiscoveryRootAction("~");
      await discovery.load();
    } catch (error) {
      toast({
        title: t("workspaces:failedToDiscoverRepositories"),
        description: error instanceof Error ? error.message : t("common:requestFailed"),
        variant: "error",
      });
    } finally {
      setAddingHome(false);
    }
  }, [discovery, toast, t]);

  return {
    discovery,
    discoverySettingsOpen,
    setDiscoverySettingsOpen,
    addingHome,
    handleAddHomeAndOpenDiscovery,
  };
}

function resolveRepositorySelection(
  value: string,
  discoveredRepositories: LocalRepository[],
): Partial<WorkspaceSourceRow> {
  const discovered = discoveredRepositories.find((repository) => repository.path === value);
  return discovered
    ? {
        repositoryId: undefined,
        localPath: discovered.path,
        remoteUrl: undefined,
        baseBranch: discovered.default_branch ?? "",
      }
    : {
        repositoryId: value,
        localPath: undefined,
        remoteUrl: undefined,
        baseBranch: "",
      };
}

export function SavedRepositorySourceRow({
  row,
  repositories,
  discoveredRepositories,
  workspaceId,
  canCreateRepository,
  repositoriesRefreshing,
  onRefreshRepositories,
  onUpdate,
}: Props) {
  const [creatingRepository, setCreatingRepository] = useState(false);
  const {
    discovery,
    discoverySettingsOpen,
    setDiscoverySettingsOpen,
    addingHome,
    handleAddHomeAndOpenDiscovery,
  } = useSavedRepositoryDiscovery(workspaceId);
  const upsertRepository = useAppStore((state) => state.upsertRepository);
  const chip: TaskRepoRow = {
    key: row.key,
    repositoryId: row.repositoryId,
    localPath: row.localPath,
    branch: row.baseBranch ?? "",
  };

  const selectRepository = (value: string) => {
    onUpdate(row.key, resolveRepositorySelection(value, discoveredRepositories));
  };

  const selectCreatedRepository = (repository: Repository) => {
    if (workspaceId) upsertRepository(workspaceId, repository);
    onUpdate(row.key, {
      repositoryId: repository.id,
      localPath: undefined,
      remoteUrl: undefined,
      baseBranch: repository.default_branch || "main",
    });
  };

  return (
    <>
      <WorkspaceRepoChips
        rows={[chip]}
        repositories={repositories}
        discoveredRepositories={discoveredRepositories}
        workspaceId={workspaceId}
        showDiscoveryControls={discovery.desktopRuntime}
        onOpenDiscoverySettings={() => setDiscoverySettingsOpen(true)}
        onAddHomeAndOpenDiscovery={handleAddHomeAndOpenDiscovery}
        canAddMore={false}
        onAdd={() => {}}
        onRemove={() => {}}
        onRowRepositoryChange={(_, value) => selectRepository(value)}
        onRowBranchChange={(_, baseBranch) => onUpdate(row.key, { baseBranch })}
        onCreateRepository={canCreateRepository ? () => setCreatingRepository(true) : undefined}
        onRefreshRepositories={onRefreshRepositories}
        repositoriesRefreshing={repositoriesRefreshing}
      />
      <CreateLocalRepositorySurface
        open={creatingRepository}
        onOpenChange={setCreatingRepository}
        workspaceId={workspaceId}
        executorSelection={null}
        context="workspace"
        onCreated={selectCreatedRepository}
      />
      <RepositoryDiscoveryDialog
        open={discoverySettingsOpen && discovery.desktopRuntime}
        onOpenChange={setDiscoverySettingsOpen}
        workspaceId={workspaceId}
        isInitialLoading={addingHome}
      />
    </>
  );
}
