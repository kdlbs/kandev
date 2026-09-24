"use client";

import { useEffect } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const selectionDebug = createDebugLogger("task-create:selection");

type RepositoryAutoPickDecision = {
  pickId: string | null;
  source: string;
  defer: boolean;
  settingsRepoId: string | null;
  settingsValid: boolean;
};

type RepositoryAutoSelectSettings = {
  lastUsedRepositoryId?: string | null;
  userSettingsLoaded?: boolean;
  repositoriesLoaded?: boolean;
  hasWorkspaceSourcesSnapshot?: boolean;
};

export function useRepositoryAutoSelectEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositories: Repository[],
  settings: RepositoryAutoSelectSettings = {},
) {
  // On open, seed a row only when a valid last-used repository or the
  // workspace's sole repository is available. An empty catalog enters scratch
  // mode after loading so the picker stays visible without an identity-free chip.
  const { repositories: rows, useRemote } = fs;
  const hydrateRepositories = fs.hydrateRepositories ?? fs.setRepositories;
  const hasRemoteSelection = fs.repositorySelections
    ? fs.repositorySelections.some((selection) => selection.kind === "remote")
    : useRemote;
  const {
    lastUsedRepositoryId,
    userSettingsLoaded = true,
    repositoriesLoaded = true,
    hasWorkspaceSourcesSnapshot = false,
  } = settings;
  useEffect(() => {
    if (
      shouldSkipRepositoryAutoSelect({
        open,
        workspaceId,
        fs,
        hasRemoteSelection,
        hasWorkspaceSourcesSnapshot,
        repositoriesLoaded,
      })
    )
      return;
    if (!workspaceId) return;
    const decision = decideRepositoryAutoPick(
      repositories,
      lastUsedRepositoryId,
      userSettingsLoaded,
    );
    logRepositoryAutoPick(workspaceId, repositories.length, decision);
    if (decision.defer) return;
    if (!decision.pickId) {
      if (
        fs.repositorySelections &&
        fs.repositorySelections.length === 0 &&
        rows.length === 0 &&
        repositories.length === 0
      ) {
        fs.setNoRepository?.(true);
      }
      return;
    }
    const { pickId } = decision;
    if (rows.length > 0 && !canReplaceEmptyRepositoryPlaceholder(rows, pickId)) return;
    void Promise.resolve().then(() => {
      hydrateRepositories((prev) => {
        if (prev.length > 0) return replaceSeededRepositoryRows(prev, pickId);
        return [buildRepositoryAutoPickRow("row-0", pickId)];
      });
    });
  }, [
    open,
    repositories,
    rows,
    hasRemoteSelection,
    fs.noRepository,
    fs.repositorySelectionsTouched,
    workspaceId,
    hydrateRepositories,
    lastUsedRepositoryId,
    userSettingsLoaded,
    repositoriesLoaded,
    hasWorkspaceSourcesSnapshot,
  ]);
}

function shouldSkipRepositoryAutoSelect({
  open,
  workspaceId,
  fs,
  hasRemoteSelection,
  hasWorkspaceSourcesSnapshot,
  repositoriesLoaded,
}: {
  open: boolean;
  workspaceId: string | null;
  fs: DialogFormState;
  hasRemoteSelection: boolean;
  hasWorkspaceSourcesSnapshot: boolean;
  repositoriesLoaded: boolean;
}): boolean {
  return (
    !open ||
    !workspaceId ||
    fs.noRepository ||
    hasRemoteSelection ||
    fs.repositorySelectionsTouched ||
    hasWorkspaceSourcesSnapshot ||
    !repositoriesLoaded
  );
}

function replaceSeededRepositoryRows(rows: TaskRepoRow[], pickId: string | null): TaskRepoRow[] {
  if (canReplaceEmptyRepositoryPlaceholder(rows, pickId)) {
    return [buildRepositoryAutoPickRow(rows[0]?.key ?? "row-0", pickId!)];
  }
  if (isDebug()) {
    selectionDebug("repository-autopick-skip", {
      reason: "rows-seeded-before-microtask",
      row_count: rows.length,
    });
  }
  return rows;
}

function decideRepositoryAutoPick(
  repositories: Repository[],
  lastUsedRepositoryId?: string | null,
  userSettingsLoaded = true,
): RepositoryAutoPickDecision {
  const settingsRepoId = lastUsedRepositoryId ?? null;
  const settingsValid = isRepositoryIdValid(settingsRepoId, repositories);
  if (settingsRepoId && settingsValid) {
    return buildRepositoryAutoPickDecision("settings:taskCreateLastUsed", settingsRepoId, {
      settingsRepoId,
      settingsValid,
    });
  }
  if (!userSettingsLoaded) {
    return buildRepositoryAutoPickDecision("user-settings-loading", null, {
      defer: true,
      settingsRepoId,
      settingsValid,
    });
  }
  return buildRepositoryAutoPickDecision(
    repositories.length === 1 ? "single-workspace-repo" : "no-repository-candidate",
    repositories.length === 1 ? repositories[0].id : null,
    { settingsRepoId, settingsValid },
  );
}

function buildRepositoryAutoPickDecision(
  source: string,
  pickId: string | null,
  fields: Omit<RepositoryAutoPickDecision, "source" | "pickId" | "defer"> & {
    defer?: boolean;
  },
): RepositoryAutoPickDecision {
  return {
    pickId,
    source,
    defer: fields.defer ?? false,
    settingsRepoId: fields.settingsRepoId,
    settingsValid: fields.settingsValid,
  };
}

function isRepositoryIdValid(repositoryId: string | null, repositories: Repository[]): boolean {
  return Boolean(repositoryId && repositories.some((r: Repository) => r.id === repositoryId));
}

function logRepositoryAutoPick(
  workspaceId: string,
  repoCount: number,
  decision: RepositoryAutoPickDecision,
) {
  if (!isDebug()) return;
  selectionDebug("repository-autopick", {
    workspace_id: workspaceId,
    settings_id: decision.settingsRepoId ?? "-",
    settings_valid: decision.settingsValid,
    repo_count: repoCount,
    source: decision.source,
    pick: decision.pickId ?? "-",
  });
}

function buildRepositoryAutoPickRow(key: string, repositoryId: string): TaskRepoRow {
  return { key, repositoryId, branch: "" };
}

function canReplaceEmptyRepositoryPlaceholder(rows: TaskRepoRow[], pickId: string | null): boolean {
  if (!pickId || rows.length !== 1) return false;
  const row = rows[0];
  return Boolean(row && !row.repositoryId && !row.localPath && !row.branch);
}
