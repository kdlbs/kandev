"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { TaskRemoteRepoRow, TaskRepoRow } from "@/components/task-create-dialog-types";

/**
 * Manages the unified `repositories` list for task creation. Every chip
 * (one or many) is an entry; there is no "primary" or "extras" split.
 *
 * `nextKey` increments to give each row a stable client-side key without
 * relying on array indices (which would shift on removal and break
 * uncontrolled inputs).
 */
export function useRepositoriesState() {
  const [repositories, setRepositories] = useState<TaskRepoRow[]>([]);
  const nextKeyRef = useRef(0);

  const addRepository = useCallback(() => {
    nextKeyRef.current += 1;
    const key = `row-${nextKeyRef.current}`;
    setRepositories((rows) => [...rows, { key, branch: "" }]);
  }, []);

  const removeRepository = useCallback((key: string) => {
    setRepositories((rows) => rows.filter((r) => r.key !== key));
  }, []);

  const updateRepository = useCallback((key: string, patch: Partial<TaskRepoRow>) => {
    setRepositories((rows) => rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  }, []);

  return { repositories, setRepositories, addRepository, removeRepository, updateRepository };
}

/**
 * Manages the unified `remoteRepos` list for task creation. Mirrors
 * `useRepositoriesState` — same key-generation pattern, same shape of
 * add/update/remove operations — but rows carry a remote URL + branch
 * instead of a workspace repoId / localPath. Used by the GitHub Remote
 * mode of the task-create dialog.
 */
export function useRemoteReposState() {
  const [remoteRepos, setRemoteRepos] = useState<TaskRemoteRepoRow[]>([]);
  const nextKeyRef = useRef(0);

  const newKey = useCallback(() => {
    nextKeyRef.current += 1;
    return `remote-${nextKeyRef.current}`;
  }, []);

  // Allocates a fresh key by bumping the local counter until it lands on
  // one not already used by the current rows. The plain `newKey()` helper
  // can't see the rows list, so a hydrated state — e.g. `setRemoteRepos`
  // injecting `{key: "remote-1", …}` from initialValues — would collide
  // with the next addRemoteRepo() (counter starts at 0, first call hands
  // out "remote-1"). Using the setter callback gives us the current rows
  // synchronously so we can skip taken keys.
  const addRemoteRepo = useCallback(() => {
    setRemoteRepos((rows) => {
      const taken = new Set(rows.map((r) => r.key));
      let key: string;
      do {
        nextKeyRef.current += 1;
        key = `remote-${nextKeyRef.current}`;
      } while (taken.has(key));
      return [...rows, { key, url: "", branch: "", source: "paste" }];
    });
  }, []);

  const removeRemoteRepo = useCallback((key: string) => {
    setRemoteRepos((rows) => rows.filter((r) => r.key !== key));
  }, []);

  const updateRemoteRepo = useCallback((key: string, patch: Partial<TaskRemoteRepoRow>) => {
    setRemoteRepos((rows) =>
      rows.map((row) => (row.key === key ? applyRemoteRepoPatch(row, patch) : row)),
    );
  }, []);

  return {
    remoteRepos,
    setRemoteRepos,
    addRemoteRepo,
    removeRemoteRepo,
    updateRemoteRepo,
    newRemoteRepoKey: newKey,
  };
}

function applyRemoteRepoPatch(
  row: TaskRemoteRepoRow,
  patch: Partial<TaskRemoteRepoRow>,
): TaskRemoteRepoRow {
  const next = { ...row, ...patch };
  if (typeof patch.url !== "string" || patch.url.trim() === row.url.trim()) {
    return next;
  }
  return {
    ...next,
    prNumber: undefined,
    prBaseBranch: undefined,
    prHeadBranch: undefined,
  };
}

/**
 * Mirrors `useRepositoryAutoSelectEffect` for the remote-repo list: when the
 * user flips Remote mode on and the list is empty, seed a single empty paste
 * row so the URL input has somewhere to land. The list is NOT cleared on
 * Remote → off (toggle-back is non-destructive).
 *
 * Shared between the create-task dialog and the New Subtask form so both
 * surfaces get the same auto-seed behavior on Remote toggle.
 */
export function useRemoteReposSeedEffect(
  useRemote: boolean,
  rows: TaskRemoteRepoRow[],
  setRemoteRepos: React.Dispatch<React.SetStateAction<TaskRemoteRepoRow[]>>,
) {
  useEffect(() => {
    if (!useRemote || rows.length > 0) return;
    setRemoteRepos([{ key: "remote-0", url: "", branch: "", source: "paste" }]);
  }, [useRemote, rows.length, setRemoteRepos]);
}
