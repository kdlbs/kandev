"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { listRepositories, listRepositorySets } from "@/lib/api/domains/workspace-api";
import type { Repository, RepositorySet } from "@/lib/types/http";
import type { ProjectChoice } from "@/lib/coordinators/control-draft";

export type ProjectsStatus = "loading" | "ready" | "error";

export type ProjectSetChoice = ProjectChoice & { kind: "repository_set"; repositoryCount: number };

const byName = (a: ProjectChoice, b: ProjectChoice) => {
  const x = a.name.toLowerCase();
  const y = b.name.toLowerCase();
  if (x !== y) return x < y ? -1 : 1;
  if (a.id === b.id) return 0;
  return a.id < b.id ? -1 : 1;
};

/** Sets, and the repositories that belong to no set, each ordered by lowercase name then id. */
export function projectChoices(
  sets: readonly RepositorySet[],
  repositories: readonly Repository[],
): { sets: ProjectSetChoice[]; loose: ProjectChoice[] } {
  const inSet = new Set(sets.flatMap((set) => set.repositories.map((item) => item.repository_id)));
  return {
    sets: sets
      .map((set) => ({
        kind: "repository_set" as const,
        id: set.id,
        name: set.name,
        repositoryCount: set.repositories.length,
      }))
      .sort(byName),
    loose: repositories
      .filter((repository) => !inSet.has(repository.id))
      .map((repository) => ({
        kind: "repository" as const,
        id: repository.id,
        name: repository.name,
      }))
      .sort(byName),
  };
}

/**
 * The workspace's selectable projects: its repository sets and the repositories
 * outside every set. The latest read wins; a failed re-read keeps the list
 * already loaded.
 */
export function useWorkspaceProjects(workspaceId: string, enabled = true) {
  const [choices, setChoices] = useState<{ sets: ProjectSetChoice[]; loose: ProjectChoice[] }>({
    sets: [],
    loose: [],
  });
  const [status, setStatus] = useState<ProjectsStatus>("loading");
  const sequenceRef = useRef(0);
  const loadedRef = useRef(false);

  const reload = useCallback(() => {
    const sequence = ++sequenceRef.current;
    Promise.all([listRepositorySets(workspaceId), listRepositories(workspaceId)])
      .then(([sets, repositories]) => {
        if (sequence !== sequenceRef.current) return;
        loadedRef.current = true;
        setChoices(projectChoices(sets.repository_sets, repositories.repositories));
        setStatus("ready");
      })
      .catch(() => {
        if (sequence !== sequenceRef.current || loadedRef.current) return;
        setStatus("error");
      });
  }, [workspaceId]);

  useEffect(() => {
    if (!enabled) return;
    loadedRef.current = false;
    setChoices({ sets: [], loose: [] });
    setStatus("loading");
    reload();
    return () => {
      sequenceRef.current += 1;
    };
  }, [enabled, reload]);

  const retry = useCallback(() => {
    setStatus("loading");
    reload();
  }, [reload]);

  return { ...choices, status, retry };
}
