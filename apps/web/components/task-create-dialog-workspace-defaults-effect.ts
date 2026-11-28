"use client";

import { useEffect } from "react";
import type { DialogFormState, TaskCreateEffectsArgs } from "@/components/task-create-dialog-types";
import {
  hasLastUsedWorkspaceSnapshot,
  selectionsFromLastUsedSources,
} from "@/components/task-create-dialog-workspace-defaults";

function hasExplicitSourcePreset(initialValues: TaskCreateEffectsArgs["initialValues"]): boolean {
  return Boolean(
    initialValues?.repositorySelections !== undefined ||
    initialValues?.repositories?.length ||
    initialValues?.repositoryId ||
    initialValues?.remoteUrl ||
    initialValues?.githubUrl ||
    initialValues?.noRepository !== undefined,
  );
}

export function useLastUsedWorkspaceSourcesEffect(
  fs: DialogFormState,
  args: TaskCreateEffectsArgs,
) {
  const {
    open,
    workspaceId,
    userSettingsLoaded,
    restoreWorkspaceContents,
    initialValues,
    workspaceSourcesByWorkspace,
    hasWorkspaceSourcesSnapshot,
  } = args;
  const snapshots = workspaceSourcesByWorkspace ?? {};
  const hasSnapshot =
    hasWorkspaceSourcesSnapshot ?? hasLastUsedWorkspaceSnapshot(snapshots, workspaceId);
  const sources = workspaceId ? snapshots[workspaceId] : undefined;
  const {
    hydrateRepositorySelections,
    repositorySelectionsTouched,
    setNoRepository,
    setUseRemote,
    setWorkspacePath,
  } = fs;
  useEffect(() => {
    if (
      !restoreWorkspaceContents ||
      !open ||
      !workspaceId ||
      userSettingsLoaded === false ||
      !hasSnapshot ||
      hasExplicitSourcePreset(initialValues) ||
      repositorySelectionsTouched
    ) {
      return;
    }
    const selections = selectionsFromLastUsedSources(sources);
    hydrateRepositorySelections?.(selections);
    setNoRepository(selections.length === 0);
    setUseRemote(false);
    setWorkspacePath("");
  }, [
    hydrateRepositorySelections,
    hasSnapshot,
    initialValues,
    open,
    repositorySelectionsTouched,
    restoreWorkspaceContents,
    sources,
    setNoRepository,
    setUseRemote,
    setWorkspacePath,
    userSettingsLoaded,
    workspaceId,
  ]);
}
