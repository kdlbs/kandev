"use client";

import { useEffect } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState, TaskRepoRow } from "@/components/task-create-dialog-types";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { createDebugLogger, isDebug } from "@/lib/debug/log";

const selectionDebug = createDebugLogger("task-create:selection");

type RepositoryAutoPickDecision = {
  pickId: string | null;
  source: string;
  defer: boolean;
  localStorageRepoId: string | null;
  localStorageValid: boolean;
  settingsRepoId: string | null;
  settingsValid: boolean;
};

type RepositoryAutoSelectSettings = {
  lastUsedRepositoryId?: string | null;
  userSettingsLoaded?: boolean;
};

export function useRepositoryAutoSelectEffect(
  fs: DialogFormState,
  open: boolean,
  workspaceId: string | null,
  repositories: Repository[],
  settings: RepositoryAutoSelectSettings = {},
) {
  // On open, ensure there's always at least one chip rendered: prefer the
  // user's last-used repo (or the workspace's only repo) so the chip lands
  // pre-filled, but fall back to an empty row so the picker is visible
  // instead of just the "+" button. URL mode is excluded - that flow swaps
  // the chip row for a URL input.
  const { repositories: rows, useRemote, setRepositories } = fs;
  const { lastUsedRepositoryId, userSettingsLoaded = true } = settings;
  useEffect(() => {
    if (!open || !workspaceId || useRemote) return;
    const decision = decideRepositoryAutoPick(
      repositories,
      lastUsedRepositoryId,
      userSettingsLoaded,
    );
    logRepositoryAutoPick(workspaceId, repositories.length, decision);
    if (decision.defer) return;
    const { pickId } = decision;
    if (rows.length > 0 && !canReplaceEmptyRepositoryPlaceholder(rows, pickId)) return;
    void Promise.resolve().then(() => {
      setRepositories((prev) => {
        if (prev.length > 0) return replaceSeededRepositoryRows(prev, pickId);
        return [
          pickId ? buildRepositoryAutoPickRow("row-0", pickId) : { key: "row-0", branch: "" },
        ];
      });
    });
  }, [
    open,
    repositories,
    rows,
    useRemote,
    workspaceId,
    setRepositories,
    lastUsedRepositoryId,
    userSettingsLoaded,
  ]);
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
  const localStorageRepoId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_REPOSITORY_ID, null);
  const settingsRepoId = lastUsedRepositoryId ?? null;
  const localStorageValid = isRepositoryIdValid(localStorageRepoId, repositories);
  const settingsValid = isRepositoryIdValid(settingsRepoId, repositories);
  if (settingsRepoId && settingsValid) {
    return buildRepositoryAutoPickDecision("settings:taskCreateLastUsed", settingsRepoId, {
      localStorageRepoId,
      localStorageValid,
      settingsRepoId,
      settingsValid,
    });
  }
  if (!userSettingsLoaded) {
    return buildRepositoryAutoPickDecision("user-settings-loading", null, {
      defer: true,
      localStorageRepoId,
      localStorageValid,
      settingsRepoId,
      settingsValid,
    });
  }
  if (localStorageRepoId && localStorageValid) {
    return buildRepositoryAutoPickDecision("localStorage:lastRepositoryId", localStorageRepoId, {
      localStorageRepoId,
      localStorageValid,
      settingsRepoId,
      settingsValid,
    });
  }
  return buildRepositoryAutoPickDecision(
    repositories.length === 1 ? "single-workspace-repo" : "empty-row",
    repositories.length === 1 ? repositories[0].id : null,
    { localStorageRepoId, localStorageValid, settingsRepoId, settingsValid },
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
    localStorageRepoId: fields.localStorageRepoId,
    localStorageValid: fields.localStorageValid,
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
    local_storage_id: decision.localStorageRepoId ?? "-",
    local_storage_valid: decision.localStorageValid,
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
