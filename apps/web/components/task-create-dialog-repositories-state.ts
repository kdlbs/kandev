"use client";

import { useCallback, useRef, useState } from "react";
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

  const addRemoteRepo = useCallback(() => {
    const key = newKey();
    setRemoteRepos((rows) => [...rows, { key, url: "", branch: "", source: "paste" }]);
  }, [newKey]);

  const removeRemoteRepo = useCallback((key: string) => {
    setRemoteRepos((rows) => rows.filter((r) => r.key !== key));
  }, []);

  const updateRemoteRepo = useCallback((key: string, patch: Partial<TaskRemoteRepoRow>) => {
    setRemoteRepos((rows) => rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));
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
