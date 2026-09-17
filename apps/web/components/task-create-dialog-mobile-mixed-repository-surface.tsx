"use client";

import type { LocalRepositoryChoice } from "@/components/task-create-dialog-repository-picker";
import { MobileMixedRepositoryChips } from "@/components/task-create-dialog-mixed-repository-chips-surfaces";
import { MobileRepositoryBranchHydrators } from "@/components/task-create-dialog-mobile-branch-hydrators";
import type { MixedRepositoryChipsProps } from "@/components/task-create-dialog-mixed-repository-chips";
import type { TaskRepoRow } from "@/components/task-create-dialog-types";
import type {
  RemoteRepository,
  UseRemoteRepositoriesResult,
} from "@/hooks/domains/integrations/use-remote-repositories";

type MobileMixedRepositoryActions = {
  addLocal: (choice: LocalRepositoryChoice) => void;
  addRemote: (repository: RemoteRepository) => void;
  addPastedRemote: (url: string) => void;
  addFolder: (path: string) => void;
  openNewLocalRepository: () => void;
  removeFolder: (key: string) => void;
};

export function MobileMixedRepositorySurface({
  props,
  localRows,
  accessible,
  selectionsCount,
  selectionRows,
  actions,
}: {
  props: MixedRepositoryChipsProps;
  localRows: TaskRepoRow[];
  accessible: UseRemoteRepositoriesResult;
  selectionsCount: number;
  selectionRows: React.ReactNode;
  actions: MobileMixedRepositoryActions;
}) {
  return (
    <>
      <MobileRepositoryBranchHydrators
        rows={localRows}
        fs={props.fs}
        workspaceId={props.workspaceId}
        isLocalExecutor={props.isLocalExecutor}
        onRowBranchChange={props.onRowBranchChange}
        lastUsedBranch={props.lastUsedBranch}
        userSettingsLoaded={props.userSettingsLoaded}
        remoteOriginMode={props.executorSourcePolicy?.capabilities.requiresCloneableLocalRepository}
        remoteOriginStates={props.remoteOriginStates}
      />
      <MobileMixedRepositoryChips
        repositories={props.repositories}
        discoveredRepositories={props.fs.discoveredRepositories}
        accessible={accessible}
        workspaceId={props.workspaceId}
        remoteOriginMode={props.executorSourcePolicy?.capabilities.requiresCloneableLocalRepository}
        selectionsCount={selectionsCount}
        selectionRows={selectionRows}
        freshBranchToggle={props.freshBranchToggle}
        branchLocked={props.branchLocked}
        repositoryLocked={props.repositoryLocked}
        repositorySets={props.repositorySets}
        onSelectLocal={actions.addLocal}
        onSelectRemote={actions.addRemote}
        onPasteRemote={actions.addPastedRemote}
        onSelectFolder={actions.addFolder}
        onCreateRepository={props.onCreateRepository ? actions.openNewLocalRepository : undefined}
        onRefreshRepositories={props.onRefreshRepositories}
        repositoriesRefreshing={props.repositoriesRefreshing}
        repositoryCreationOpen={props.repositoryCreationOpen}
        folderAvailable={props.executorSourcePolicy?.folderAvailable ?? props.isLocalExecutor}
        folderDisabledReason={props.folderDisabledReason}
      />
    </>
  );
}
