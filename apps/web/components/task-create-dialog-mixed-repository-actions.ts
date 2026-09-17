"use client";

import { useCallback } from "react";
import type { LocalRepositoryChoice } from "@/components/task-create-dialog-repository-picker";
import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type {
  DialogFormState,
  TaskRepositorySelection,
} from "@/components/task-create-dialog-types";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";

export function buildLocalRepositorySelection(
  choice: LocalRepositoryChoice,
  isLocalExecutor?: boolean,
): Omit<Extract<TaskRepositorySelection, { kind: "local" }>, "key"> {
  return {
    kind: "local",
    ...(choice.repositoryId ? { repositoryId: choice.repositoryId } : {}),
    ...(choice.localPath ? { localPath: choice.localPath } : {}),
    branch: isLocalExecutor === false ? (choice.defaultBranch ?? "") : "",
    ...(choice.checkoutSource ? { checkoutSource: choice.checkoutSource } : {}),
    ...(choice.expectedOrigin ? { expectedOrigin: choice.expectedOrigin } : {}),
    ...(choice.remoteBranches ? { remoteBranches: choice.remoteBranches } : {}),
  };
}

type MixedRepositoryActionsArgs = {
  fs: DialogFormState;
  selectionCount: number;
  isLocalExecutor: boolean;
  onCreateRepository?: (key: string) => void;
  onFolderSelectionAdded?: (wasEmpty: boolean) => void;
  onRepositorySelectionAdded?: (wasFolderOnly: boolean) => void;
  onAllWorkspaceSourcesRemoved?: () => void;
  onRepositorySelectionRemoved?: (remaining: TaskRepositorySelection[]) => void;
};

export function useMixedRepositoryActions({
  fs,
  selectionCount,
  isLocalExecutor,
  onCreateRepository,
  onFolderSelectionAdded,
  onRepositorySelectionAdded,
  onAllWorkspaceSourcesRemoved,
  onRepositorySelectionRemoved,
}: MixedRepositoryActionsArgs) {
  const appendActions = useRepositoryAppendActions({
    fs,
    isLocalExecutor,
    onCreateRepository,
    selectionCount,
    onFolderSelectionAdded,
    onRepositorySelectionAdded,
    onRepositorySelectionRemoved,
  });
  const removeActions = useRepositoryRemoveActions(
    fs,
    selectionCount,
    onAllWorkspaceSourcesRemoved,
    onRepositorySelectionRemoved,
  );
  return { ...appendActions, ...removeActions };
}

type RepositoryAppendActionsArgs = Omit<MixedRepositoryActionsArgs, "onAllWorkspaceSourcesRemoved">;

function useRepositoryAppendActions({
  fs,
  isLocalExecutor,
  onCreateRepository,
  selectionCount = 0,
  onFolderSelectionAdded,
  onRepositorySelectionAdded,
}: RepositoryAppendActionsArgs) {
  const appendSelection = fs.appendRepositorySelection;
  const addLocal = useCallback(
    (choice: LocalRepositoryChoice) => {
      onRepositorySelectionAdded?.(selectionIsFolderOnly(fs));
      fs.setNoRepository(false);
      if (appendSelection) {
        appendSelection(buildLocalRepositorySelection(choice, isLocalExecutor));
        return;
      }
      fs.addRepository();
    },
    [appendSelection, fs, isLocalExecutor, onRepositorySelectionAdded],
  );
  const addRemote = useCallback(
    (repository: RemoteRepository) => {
      onRepositorySelectionAdded?.(selectionIsFolderOnly(fs));
      fs.setNoRepository(false);
      if (!appendSelection) {
        fs.addRemoteRepo();
        return;
      }
      appendSelection({
        kind: "remote",
        url: repository.url,
        branch: repository.defaultBranch,
        source: "picker",
        provider: repository.provider,
        remoteUrl: repository.provider === "github" ? undefined : repository.url,
        providerHost: repository.providerHost,
        providerScope: repository.providerScope,
        providerRepoId: repository.id,
        providerOwner: repository.owner,
        providerName: repository.name,
        fullName: repository.fullName,
      });
    },
    [appendSelection, fs, onRepositorySelectionAdded],
  );
  const addPastedRemote = useCallback(
    (url: string) => {
      onRepositorySelectionAdded?.(selectionIsFolderOnly(fs));
      fs.setNoRepository(false);
      if (!appendSelection) {
        fs.addRemoteRepo();
        return;
      }
      appendSelection({ kind: "remote", url, branch: "", source: "paste" });
    },
    [appendSelection, fs, onRepositorySelectionAdded],
  );
  const addFolder = useCallback(
    (localPath: string) => {
      const path = localPath.trim();
      if (!path || folderAlreadySelected(fs, path)) return;
      onFolderSelectionAdded?.(selectionCount === 0);
      fs.setNoRepository(false);
      if (fs.appendFolderSelection) {
        fs.appendFolderSelection({ kind: "folder", localPath: path });
        return;
      }
      fs.setWorkspacePath(path);
    },
    [fs, onFolderSelectionAdded, selectionCount],
  );
  const openNewLocalRepository = useCallback(() => {
    if (!appendSelection || !onCreateRepository) return;
    onRepositorySelectionAdded?.(selectionIsFolderOnly(fs));
    fs.setNoRepository(false);
    const key = appendSelection({ kind: "local", branch: "" });
    onCreateRepository(key);
  }, [appendSelection, fs, onCreateRepository, onRepositorySelectionAdded]);
  return { addLocal, addRemote, addPastedRemote, addFolder, openNewLocalRepository };
}

function folderAlreadySelected(fs: DialogFormState, path: string): boolean {
  const normalizedPath = normalizeWorkspaceFolderPath(path);
  return resolveRepositorySelections(fs).some(
    (selection) =>
      selection.kind === "folder" &&
      normalizeWorkspaceFolderPath(selection.localPath) === normalizedPath,
  );
}

function useRepositoryRemoveActions(
  fs: DialogFormState,
  selectionCount: number,
  onAllWorkspaceSourcesRemoved?: () => void,
  onRepositorySelectionRemoved?: (remaining: TaskRepositorySelection[]) => void,
) {
  const shouldEnterScratch = selectionCount === 1;
  const removeLocal = useCallback(
    (key: string) => {
      const remaining = remainingSelections(fs, key);
      fs.removeRepository(key);
      if (shouldEnterScratch) {
        fs.setNoRepository(true);
        onAllWorkspaceSourcesRemoved?.();
      } else {
        onRepositorySelectionRemoved?.(remaining);
      }
    },
    [fs, shouldEnterScratch, onAllWorkspaceSourcesRemoved, onRepositorySelectionRemoved],
  );
  const removeRemote = useCallback(
    (key: string) => {
      const remaining = remainingSelections(fs, key);
      fs.removeRemoteRepo(key);
      if (shouldEnterScratch) {
        fs.setNoRepository(true);
        onAllWorkspaceSourcesRemoved?.();
      } else {
        onRepositorySelectionRemoved?.(remaining);
      }
    },
    [fs, shouldEnterScratch, onAllWorkspaceSourcesRemoved, onRepositorySelectionRemoved],
  );
  const removeFolder = useCallback(
    (key: string) => {
      fs.removeRepository(key);
      if (shouldEnterScratch) {
        fs.setNoRepository(true);
        onAllWorkspaceSourcesRemoved?.();
      }
    },
    [fs.removeRepository, fs.setNoRepository, shouldEnterScratch, onAllWorkspaceSourcesRemoved],
  );
  return { removeLocal, removeRemote, removeFolder };
}

function remainingSelections(fs: DialogFormState, removedKey: string): TaskRepositorySelection[] {
  return resolveRepositorySelections(fs).filter((selection) => selection.key !== removedKey);
}

function selectionIsFolderOnly(fs: DialogFormState): boolean {
  const selections = resolveRepositorySelections(fs).filter((selection) => {
    if (selection.kind === "remote") return Boolean(selection.url.trim());
    if (selection.kind === "folder") return Boolean(selection.localPath.trim());
    return Boolean(selection.repositoryId || selection.localPath);
  });
  return selections.length > 0 && selections.every((selection) => selection.kind === "folder");
}

function normalizeWorkspaceFolderPath(path: string): string {
  const normalized = path.replace(/\\/g, "/").replace(/\/+$/g, "");
  return normalized || "/";
}
