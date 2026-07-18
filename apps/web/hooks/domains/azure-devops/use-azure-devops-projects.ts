"use client";

import { useCallback, useEffect, useState } from "react";
import {
  listAzureDevOpsProjects,
  listAzureDevOpsRepositories,
} from "@/lib/api/domains/azure-devops-api";
import type { AzureDevOpsProject, AzureDevOpsRepository } from "@/lib/types/azure-devops";

type Loadable<T> = { data: T; loading: boolean; error: string | null; refresh: () => void };

function useAzureDevOpsList<T>(load: () => Promise<T>, empty: T, active: boolean): Loadable<T> {
  const [data, setData] = useState(empty);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!active) {
      setData(empty);
      setError(null);
      return;
    }
    let cancelled = false;
    setLoading(true);
    setError(null);
    load()
      .then((next) => {
        if (!cancelled) setData(next);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [active, empty, load, revision]);

  return { data, loading, error, refresh: () => setRevision((value) => value + 1) };
}

const EMPTY_PROJECTS: AzureDevOpsProject[] = [];
const EMPTY_REPOSITORIES: AzureDevOpsRepository[] = [];

export function useAzureDevOpsProjects(
  workspaceId: string,
  active: boolean = true,
): Loadable<AzureDevOpsProject[]> {
  const load = useCallback(
    () => listAzureDevOpsProjects(workspaceId).then((result) => result.projects ?? []),
    [workspaceId],
  );
  return useAzureDevOpsList(load, EMPTY_PROJECTS, active);
}

export function useAzureDevOpsRepositories(
  workspaceId: string,
  projectId: string,
): Loadable<AzureDevOpsRepository[]> {
  const load = useCallback(
    () =>
      listAzureDevOpsRepositories(workspaceId, projectId).then(
        (result) => result.repositories ?? [],
      ),
    [projectId, workspaceId],
  );
  return useAzureDevOpsList(load, EMPTY_REPOSITORIES, !!projectId);
}
