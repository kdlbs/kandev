"use client";

import { useEffect, useMemo } from "react";
import type { LocalRepository, Repository } from "@/lib/types/http";
import type {
  DialogFormState,
  TaskRemoteRepoRow,
  TaskRepoRow,
  TaskRepositorySelection,
  TaskRepositorySetsConfig,
} from "@/components/task-create-dialog-types";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";
import {
  collectExcludedRepoIds,
  collectSelectedRepoIdentities,
  RepoChip,
} from "@/components/task-create-dialog-workspace-repo-chips";
import {
  RemoteRepoChip,
  selectedRemoteRepositoryIdentity,
} from "@/components/task-create-dialog-remote-repo-chip";
import {
  inspectedRemoteRepositoryUpdate,
  makeURLChange,
  remoteRepositoryUpdateNeeded,
  retryRemoteResolution,
} from "@/components/task-create-dialog-remote-repo-chips";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { DesktopMixedRepositoryChips } from "@/components/task-create-dialog-mixed-repository-chips-surfaces";
import { MobileMixedRepositorySurface } from "@/components/task-create-dialog-mobile-mixed-repository-surface";
import { useMixedRepositoryActions } from "@/components/task-create-dialog-mixed-repository-actions";
export { buildLocalRepositorySelection } from "@/components/task-create-dialog-mixed-repository-actions";
import { FolderSelectionChip } from "@/components/task-create-dialog-workspace-folder-chip";
import { useTouchDrawer } from "@/hooks/use-compact-task-chrome";
import { computeBranchIntent } from "@/components/task-create-dialog-branch-utils";
import { isPickerRemoteProviderUnavailable } from "@/components/task-create-dialog-remote-provider-readiness";
import type { ExecutorSourcePolicy } from "@/components/task-create-dialog-executor-source-policy";
import type { RepositoryCloneSourceState } from "@/hooks/domains/repositories/use-repository-clone-source";
import {
  remoteOriginBranchesForState,
  remoteOriginSelectionNeedsRecovery,
} from "@/components/task-create-dialog-remote-origin-inspection";
import { useTranslation } from "react-i18next";

export type MixedRepositoryChipsProps = {
  fs: DialogFormState;
  repositories: Repository[];
  workspaceId: string | null;
  isLocalExecutor: boolean;
  executorSourcePolicy?: ExecutorSourcePolicy;
  folderDisabledReason?: string;
  remoteOriginStates?: Record<string, RepositoryCloneSourceState>;
  onRefreshRemoteOrigins?: () => void;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
  freshBranchEnabled?: boolean;
  branchPolicyDisabledReason?: string;
  freshBranchToggle?: React.ReactNode;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onRowPolicyChange?: (key: string, policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
  onWorkspacePathChange?: (value: string) => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  onCreateRepository?: (key: string) => void;
  repositoryCreationOpen?: boolean;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
  repositorySets?: TaskRepositorySetsConfig;
  onFolderSelectionAdded?: (wasEmpty: boolean) => void;
  onRepositorySelectionAdded?: (wasFolderOnly: boolean) => void;
  onAllWorkspaceSourcesRemoved?: () => void;
  onRepositorySelectionRemoved?: (remaining: TaskRepositorySelection[]) => void;
};

/** Renders the ordered local and remote rows with one shared source picker. */
export function MixedRepositoryChips(props: MixedRepositoryChipsProps) {
  const mobile = useTouchDrawer();
  const selections = props.fs.noRepository ? [] : resolveRepositorySelections(props.fs);
  const localRows = selections.filter(isLocalSelection).map(stripLocalKind);
  const remoteRows = selections.filter(isRemoteSelection).map(stripRemoteKind);
  const accessible = useRemoteRepositories(props.workspaceId ?? "");

  useRemoteProviderReadiness(props.fs, accessible);

  useRemoteRowResolution(props.fs, remoteRows);
  const actions = useMixedRepositoryActions({
    fs: props.fs,
    selectionCount: selections.length,
    isLocalExecutor: props.isLocalExecutor,
    onCreateRepository: props.onCreateRepository,
    onFolderSelectionAdded: props.onFolderSelectionAdded,
    onRepositorySelectionAdded: props.onRepositorySelectionAdded,
    onAllWorkspaceSourcesRemoved: props.onAllWorkspaceSourcesRemoved,
    onRepositorySelectionRemoved: props.onRepositorySelectionRemoved,
  });
  const selectionRows = (
    <RepositorySelectionRows
      selections={selections}
      localRows={localRows}
      remoteRows={remoteRows}
      repositories={props.repositories}
      fs={props.fs}
      workspaceId={props.workspaceId}
      isLocalExecutor={props.isLocalExecutor}
      freshBranchEnabled={props.freshBranchEnabled}
      branchPolicyDisabledReason={props.branchPolicyDisabledReason}
      onRowRepositoryChange={props.onRowRepositoryChange}
      onRowBranchChange={props.onRowBranchChange}
      onRowPolicyChange={props.onRowPolicyChange}
      onPolicySelected={props.onPolicySelected}
      lastUsedBranch={props.lastUsedBranch}
      userSettingsLoaded={props.userSettingsLoaded}
      onCreateRepository={props.onCreateRepository}
      onRefreshRepositories={props.onRefreshRepositories}
      repositoriesRefreshing={props.repositoriesRefreshing}
      repositoryLocked={props.repositoryLocked}
      branchLocked={props.branchLocked}
      accessible={accessible}
      remoteOriginMode={props.executorSourcePolicy?.capabilities.requiresCloneableLocalRepository}
      remoteOriginStates={props.remoteOriginStates}
      onRefreshRemoteOrigins={props.onRefreshRemoteOrigins}
      onRemoveLocal={actions.removeLocal}
      onRemoveRemote={actions.removeRemote}
      onRemoveFolder={actions.removeFolder}
    />
  );
  if (mobile) {
    return (
      <MobileMixedRepositorySurface
        props={props}
        localRows={localRows}
        accessible={accessible}
        selectionsCount={selections.length}
        selectionRows={selectionRows}
        actions={actions}
      />
    );
  }

  return (
    <DesktopMixedRepositoryChips
      repositories={props.repositories}
      discoveredRepositories={props.fs.discoveredRepositories}
      accessible={accessible}
      workspaceId={props.workspaceId}
      remoteOriginMode={props.executorSourcePolicy?.capabilities.requiresCloneableLocalRepository}
      selectionRows={selectionRows}
      freshBranchToggle={props.freshBranchToggle}
      branchLocked={props.branchLocked}
      repositoryLocked={props.repositoryLocked}
      repositorySets={props.repositorySets}
      folderAvailable={props.executorSourcePolicy?.folderAvailable ?? props.isLocalExecutor}
      folderDisabledReason={props.folderDisabledReason}
      onSelectLocal={actions.addLocal}
      onSelectRemote={actions.addRemote}
      onPasteRemote={actions.addPastedRemote}
      onSelectFolder={actions.addFolder}
      onCreateRepository={props.onCreateRepository ? actions.openNewLocalRepository : undefined}
      onRefreshRepositories={props.onRefreshRepositories}
      repositoriesRefreshing={props.repositoriesRefreshing}
    />
  );
}

type RepositorySelectionRowsProps = {
  selections: TaskRepositorySelection[];
  localRows: TaskRepoRow[];
  remoteRows: TaskRemoteRepoRow[];
  repositories: Repository[];
  fs: DialogFormState;
  workspaceId: string | null;
  isLocalExecutor: boolean;
  freshBranchEnabled?: boolean;
  branchPolicyDisabledReason?: string;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onRowPolicyChange?: (key: string, policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  onCreateRepository?: (key: string) => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
  accessible: ReturnType<typeof useRemoteRepositories>;
  onRemoveLocal: (key: string) => void;
  onRemoveRemote: (key: string) => void;
  onRemoveFolder: (key: string) => void;
  remoteOriginMode?: boolean;
  remoteOriginStates?: Record<string, RepositoryCloneSourceState>;
  onRefreshRemoteOrigins?: () => void;
};

function RepositorySelectionRows({
  selections,
  localRows,
  remoteRows,
  repositories,
  fs,
  workspaceId,
  isLocalExecutor,
  freshBranchEnabled,
  branchPolicyDisabledReason,
  onRowRepositoryChange,
  onRowBranchChange,
  onRowPolicyChange,
  onPolicySelected,
  lastUsedBranch,
  userSettingsLoaded,
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
  repositoryLocked,
  branchLocked,
  accessible,
  onRemoveLocal,
  onRemoveRemote,
  onRemoveFolder,
  remoteOriginMode,
  remoteOriginStates,
  onRefreshRemoteOrigins,
}: RepositorySelectionRowsProps) {
  return (
    <>
      {selections.map((selection) => {
        if (selection.kind === "local") {
          return (
            <LocalSelectionChip
              key={selection.key}
              row={selection}
              rows={localRows}
              repositories={repositories}
              discoveredRepositories={fs.discoveredRepositories}
              fs={fs}
              workspaceId={workspaceId}
              isLocalExecutor={isLocalExecutor}
              freshBranchEnabled={freshBranchEnabled}
              branchPolicyDisabledReason={branchPolicyDisabledReason}
              onRowRepositoryChange={onRowRepositoryChange}
              onRowBranchChange={onRowBranchChange}
              onRowPolicyChange={onRowPolicyChange}
              onPolicySelected={onPolicySelected}
              lastUsedBranch={lastUsedBranch}
              userSettingsLoaded={userSettingsLoaded}
              onCreateRepository={onCreateRepository}
              onRefreshRepositories={onRefreshRepositories}
              repositoriesRefreshing={repositoriesRefreshing}
              repositoryLocked={repositoryLocked}
              branchLocked={branchLocked}
              remoteOriginMode={remoteOriginMode}
              remoteOriginState={remoteOriginStates?.[selection.key]}
              onRefreshRemoteOrigins={onRefreshRemoteOrigins}
              onRemove={() => onRemoveLocal(selection.key)}
            />
          );
        }
        if (selection.kind === "remote") {
          return (
            <RemoteSelectionChip
              key={selection.key}
              row={selection}
              rows={remoteRows}
              fs={fs}
              accessible={accessible}
              repositoryLocked={repositoryLocked}
              branchLocked={branchLocked}
              onRemove={() => onRemoveRemote(selection.key)}
            />
          );
        }
        return (
          <FolderSelectionChip
            key={selection.key}
            selection={selection}
            repositoryLocked={repositoryLocked}
            onRemove={() => onRemoveFolder(selection.key)}
          />
        );
      })}
    </>
  );
}

type LocalSelectionChipProps = {
  row: TaskRepoRow & { kind: "local" };
  rows: TaskRepoRow[];
  repositories: Repository[];
  discoveredRepositories: LocalRepository[];
  fs: DialogFormState;
  workspaceId: string | null;
  isLocalExecutor: boolean;
  freshBranchEnabled?: boolean;
  branchPolicyDisabledReason?: string;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onRowPolicyChange?: (key: string, policyId: string, baseBranch: string) => void;
  onPolicySelected?: () => void;
  lastUsedBranch?: string | null;
  userSettingsLoaded?: boolean;
  onCreateRepository?: (key: string) => void;
  onRefreshRepositories?: () => void;
  repositoriesRefreshing?: boolean;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
  remoteOriginMode?: boolean;
  remoteOriginState?: RepositoryCloneSourceState;
  onRefreshRemoteOrigins?: () => void;
  onRemove: () => void;
};

function LocalSelectionChip(props: LocalSelectionChipProps) {
  const { row, fs, isLocalExecutor, remoteOriginMode, remoteOriginState } = props;
  const remoteOriginRecoveryNeeded = remoteOriginSelectionNeedsRecovery(
    remoteOriginMode,
    row,
    remoteOriginState,
  );
  const handleBranchChange = (value: string) => {
    if (!isLocalExecutor && row.baseBranch) {
      fs.updateRepository(row.key, { baseBranch: value || undefined });
      return;
    }
    props.onRowBranchChange(row.key, value);
  };
  return (
    <div className="flex max-w-full flex-col items-start gap-1">
      <LocalSelectionChipRepository {...props} onBranchChange={handleBranchChange} />
      <LocalSelectionChipFooter
        checkoutSource={row.checkoutSource}
        remoteOriginMode={remoteOriginMode}
        remoteOriginState={remoteOriginState}
        remoteOriginRecoveryNeeded={remoteOriginRecoveryNeeded}
        onRefreshRemoteOrigins={props.onRefreshRemoteOrigins}
      />
    </div>
  );
}

function LocalSelectionChipRepository({
  row,
  rows,
  repositories,
  discoveredRepositories,
  fs,
  workspaceId,
  isLocalExecutor,
  freshBranchEnabled,
  branchPolicyDisabledReason,
  onRowRepositoryChange,
  onRowPolicyChange,
  onPolicySelected,
  lastUsedBranch,
  userSettingsLoaded,
  onCreateRepository,
  onRefreshRepositories,
  repositoriesRefreshing,
  repositoryLocked,
  branchLocked,
  remoteOriginMode,
  remoteOriginState,
  onRemove,
  onBranchChange,
}: LocalSelectionChipProps & { onBranchChange: (value: string) => void }) {
  return (
    <RepoChip
      row={row}
      workspaceId={workspaceId}
      repositories={repositories}
      discoveredRepositories={discoveredRepositories}
      excludedRepoIds={collectExcludedRepoIds(rows, row, true)}
      selectedElsewhere={collectSelectedRepoIdentities(rows, row)}
      preferredDefaultBranch={isLocalExecutor ? fs.currentLocalBranch : undefined}
      preferredDefaultBranchLoading={isLocalExecutor ? fs.currentLocalBranchLoading : false}
      lastUsedBranch={lastUsedBranch}
      userSettingsLoaded={userSettingsLoaded}
      isLocalExecutor={isLocalExecutor}
      branchValue={isLocalExecutor ? row.branch : row.baseBranch || row.branch}
      savedBaseBranch={row.baseBranch}
      remoteBranches={remoteOriginBranchesForState(
        remoteOriginMode,
        remoteOriginState,
        row.remoteBranches,
      )}
      remoteOriginMode={remoteOriginMode}
      remoteOriginInspectionLoading={
        remoteOriginMode && (!remoteOriginState || remoteOriginState.status === "checking")
      }
      branchPolicyDisabledReason={branchPolicyDisabledReason}
      onRepositoryChange={(value) => onRowRepositoryChange(row.key, value)}
      onBranchChange={onBranchChange}
      onBaseBranchChange={(value) =>
        fs.updateRepository(row.key, { baseBranch: value || undefined })
      }
      onPolicyChange={
        onRowPolicyChange
          ? (policyId, baseBranch) => onRowPolicyChange(row.key, policyId, baseBranch)
          : undefined
      }
      onPolicySelected={onPolicySelected}
      showBranchPolicies
      showDiscoveryControls
      onCreateRepository={onCreateRepository ? () => onCreateRepository(row.key) : undefined}
      onRefreshRepositories={onRefreshRepositories}
      repositoriesRefreshing={repositoriesRefreshing}
      repositoryLocked={repositoryLocked}
      onRemove={onRemove}
      branchIntent={computeBranchIntent({
        isLocalExecutor,
        rowBranch: isLocalExecutor ? row.branch : row.baseBranch || row.branch,
        currentLocalBranch: fs.currentLocalBranch,
        freshBranchEnabled: !!freshBranchEnabled,
      })}
      branchLocked={branchLocked}
    />
  );
}

function LocalSelectionChipFooter({
  checkoutSource,
  remoteOriginMode,
  remoteOriginState,
  remoteOriginRecoveryNeeded,
  onRefreshRemoteOrigins,
}: {
  checkoutSource?: TaskRepoRow["checkoutSource"];
  remoteOriginMode?: boolean;
  remoteOriginState?: RepositoryCloneSourceState;
  remoteOriginRecoveryNeeded?: boolean;
  onRefreshRemoteOrigins?: () => void;
}) {
  const { t } = useTranslation();
  const showRemoteMode = remoteOriginMode ?? checkoutSource === "remote_origin";
  if (!showRemoteMode) return null;
  if (!remoteOriginState || remoteOriginState.status === "checking") {
    return (
      <span className="text-[10px] text-muted-foreground" data-testid="clone-from-remote-checking">
        {t("task:checkingRepositoryOrigin")}
      </span>
    );
  }
  if (
    remoteOriginRecoveryNeeded ||
    remoteOriginState.status === "unavailable" ||
    remoteOriginState.status === "error"
  ) {
    return (
      <span className="flex min-h-11 items-center gap-2 text-[10px] text-destructive sm:min-h-6">
        <span data-testid="clone-from-remote-unavailable">
          {t("task:noUsableRepositoryOrigin")}
        </span>
        {onRefreshRemoteOrigins ? (
          <button
            type="button"
            className="min-h-11 cursor-pointer underline sm:min-h-6"
            onClick={onRefreshRemoteOrigins}
            data-testid="retry-repository-origin"
          >
            {t("task:retry")}
          </button>
        ) : null}
      </span>
    );
  }
  if (checkoutSource !== "remote_origin") return null;
  return (
    <span className="text-[10px] text-muted-foreground" data-testid="clone-from-remote-label">
      {t("task:cloneFromRemote")}
    </span>
  );
}

function RemoteSelectionChip({
  row,
  rows,
  fs,
  accessible,
  repositoryLocked,
  branchLocked,
  onRemove,
}: {
  row: TaskRemoteRepoRow & { kind: "remote" };
  rows: TaskRemoteRepoRow[];
  fs: DialogFormState;
  accessible: ReturnType<typeof useRemoteRepositories>;
  repositoryLocked?: boolean;
  branchLocked?: boolean;
  onRemove: () => void;
}) {
  const selectedRepositoryIdentities = rows
    .filter((otherRow) => otherRow.key !== row.key)
    .map(selectedRemoteRepositoryIdentity)
    .filter((identity): identity is string => Boolean(identity));
  const connectionUnavailable = isRemoteProviderConnectionUnavailable(row, accessible);
  return (
    <RemoteRepoChip
      row={row}
      branches={fs.branchesByUrl.branches(row.url)}
      branchesLoading={fs.branchesByUrl.loading(row.url)}
      prInfo={fs.prInfoByUrl.info(row.url)}
      resolutionError={fs.branchesByUrl.error(row.url) ?? fs.prInfoByUrl.error(row.url)}
      connectionUnavailable={connectionUnavailable}
      accessibleRepos={accessible}
      selectedRepositoryIdentities={selectedRepositoryIdentities}
      onURLChange={makeURLChange(fs.updateRemoteRepo, row.key)}
      onBranchChange={(branch) => fs.updateRemoteRepo(row.key, { branch })}
      onRetry={() => {
        accessible.refresh?.();
        retryRemoteResolution(fs, row.url);
      }}
      onRemove={onRemove}
      repositoryLocked={repositoryLocked}
      branchLocked={branchLocked}
    />
  );
}

function isRemoteProviderConnectionUnavailable(
  row: TaskRemoteRepoRow,
  accessible: ReturnType<typeof useRemoteRepositories>,
): boolean {
  const readiness = accessible.providerCatalog
    ? Object.fromEntries(
        accessible.providerCatalog.map((entry) => [entry.provider, entry.readiness]),
      )
    : undefined;
  return isPickerRemoteProviderUnavailable(row, readiness);
}

function useRemoteProviderReadiness(
  fs: DialogFormState,
  accessible: ReturnType<typeof useRemoteRepositories>,
) {
  const providerCatalog = accessible.providerCatalog;
  const readiness = useMemo(
    () =>
      providerCatalog
        ? Object.fromEntries(providerCatalog.map((entry) => [entry.provider, entry.readiness]))
        : undefined,
    [providerCatalog],
  );
  useEffect(() => {
    if (!readiness || !fs.setRemoteProviderReadiness) return;
    fs.setRemoteProviderReadiness(readiness);
  }, [fs.setRemoteProviderReadiness, readiness]);
}

function useRemoteRowResolution(fs: DialogFormState, rows: TaskRemoteRepoRow[]) {
  const { ensure: ensureBranches } = fs.branchesByUrl;
  const { ensure: ensurePRInfo } = fs.prInfoByUrl;
  useEffect(() => {
    for (const row of rows) {
      if (!row.url) continue;
      ensureBranches(row.url);
      ensurePRInfo(row.url);
    }
  }, [ensureBranches, ensurePRInfo, rows]);
  const inspection = fs.prInfoByUrl.inspection;
  useEffect(() => {
    if (!inspection) return;
    for (const row of rows) {
      const resolved = inspection(row.url);
      const update = resolved ? inspectedRemoteRepositoryUpdate(resolved, row) : undefined;
      if (update && remoteRepositoryUpdateNeeded(row, update)) fs.updateRemoteRepo(row.key, update);
    }
  }, [fs.updateRemoteRepo, inspection, rows]);
}

function isLocalSelection(
  selection: TaskRepositorySelection,
): selection is Extract<TaskRepositorySelection, { kind: "local" }> {
  return selection.kind === "local";
}

function isRemoteSelection(
  selection: TaskRepositorySelection,
): selection is Extract<TaskRepositorySelection, { kind: "remote" }> {
  return selection.kind === "remote";
}

function stripLocalKind(
  selection: Extract<TaskRepositorySelection, { kind: "local" }>,
): TaskRepoRow & { kind: "local" } {
  return selection;
}

function stripRemoteKind(
  selection: Extract<TaskRepositorySelection, { kind: "remote" }>,
): TaskRemoteRepoRow & { kind: "remote" } {
  return selection;
}
