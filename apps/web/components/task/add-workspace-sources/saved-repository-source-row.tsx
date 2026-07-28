"use client";

import { useState } from "react";
import { CreateLocalRepositorySurface } from "@/components/create-local-repository-surface";
import { useAppStore } from "@/components/state-provider";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import { WorkspaceRepoChips } from "@/components/task-create-dialog-workspace-repo-chips";
import type { WorkspaceSourceRow } from "@/components/workspace-source-picker/workspace-source-state";
import type { LocalRepository, Repository } from "@/lib/types/http";

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
  const upsertRepository = useAppStore((state) => state.upsertRepository);
  const chip: TaskRepoRow = {
    key: row.key,
    repositoryId: row.repositoryId,
    localPath: row.localPath,
    branch: row.baseBranch ?? "",
  };

  const selectRepository = (value: string) => {
    const discovered = discoveredRepositories.find((repository) => repository.path === value);
    onUpdate(
      row.key,
      discovered
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
          },
    );
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
    </>
  );
}
