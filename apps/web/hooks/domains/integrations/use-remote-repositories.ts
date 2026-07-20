"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchAccessibleRepos } from "@/lib/api/domains/github-api";
import { listUserProjects } from "@/lib/api/domains/gitlab-api";
import {
  listAzureDevOpsProjects,
  listAzureDevOpsRepositories,
} from "@/lib/api/domains/azure-devops-api";

export type RemoteRepository = {
  provider: "github" | "gitlab" | "azure_devops";
  id: string;
  owner: string;
  name: string;
  fullName: string;
  url: string;
  defaultBranch: string;
  private: boolean;
};

export type UseRemoteRepositoriesResult = {
  repos: RemoteRepository[];
  loading: boolean;
  error: Error | null;
  unavailable: boolean;
  search: (query: string) => void;
};

async function loadAzureRepositories(workspaceId: string): Promise<RemoteRepository[]> {
  if (!workspaceId) return [];
  const { projects = [] } = await listAzureDevOpsProjects(workspaceId);
  const batches = await Promise.all(
    projects.map((project) =>
      listAzureDevOpsRepositories(workspaceId, project.id).then(({ repositories = [] }) =>
        repositories.map((repo) => ({
          provider: "azure_devops" as const,
          id: repo.id,
          owner: repo.projectId,
          name: repo.name,
          fullName: `${repo.projectName}/${repo.name}`,
          url: repo.webUrl,
          defaultBranch: (repo.defaultBranch || "").replace(/^refs\/heads\//, ""),
          private: true,
        })),
      ),
    ),
  );
  return batches.flat();
}

type RemoteRepositoryLoad = {
  repos: RemoteRepository[];
  providerAvailable: boolean;
};

async function loadRemoteRepositories(workspaceId: string): Promise<RemoteRepositoryLoad> {
  const azureRequest = workspaceId
    ? loadAzureRepositories(workspaceId)
    : Promise.reject(new Error("workspace is required for Azure DevOps repositories"));
  const results = await Promise.allSettled([
    fetchAccessibleRepos({ limit: 100 }).then((repos) =>
      repos.map((repo) => ({
        provider: "github" as const,
        id: repo.full_name,
        owner: repo.owner,
        name: repo.name,
        fullName: repo.full_name,
        url: `https://github.com/${repo.owner}/${repo.name}`,
        defaultBranch: repo.default_branch,
        private: repo.private,
      })),
    ),
    listUserProjects().then(({ projects = [] }) =>
      projects.map((project) => ({
        provider: "gitlab" as const,
        id: String(project.id),
        owner: project.namespace,
        name: project.path,
        fullName: project.path_with_namespace,
        url: project.web_url || `https://gitlab.com/${project.path_with_namespace}.git`,
        defaultBranch: project.default_branch || "main",
        private: project.visibility === "private",
      })),
    ),
    azureRequest,
  ]);
  return {
    repos: results.flatMap((result) => (result.status === "fulfilled" ? result.value : [])),
    providerAvailable: results.some((result) => result.status === "fulfilled"),
  };
}

export function useRemoteRepositories(workspaceId: string): UseRemoteRepositoriesResult {
  const [allRepos, setAllRepos] = useState<RemoteRepository[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [providerAvailable, setProviderAvailable] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setAllRepos([]);
    setProviderAvailable(false);
    setError(null);
    setLoading(true);
    loadRemoteRepositories(workspaceId)
      .then((result) => {
        if (cancelled) return;
        setAllRepos(result.repos);
        setProviderAvailable(result.providerAvailable);
      })
      .catch((cause) => {
        if (!cancelled) setError(cause instanceof Error ? cause : new Error(String(cause)));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [workspaceId]);

  const repos = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return allRepos;
    return allRepos.filter((repo) => repo.fullName.toLowerCase().includes(needle));
  }, [allRepos, query]);
  const search = useCallback((value: string) => setQuery(value), []);
  return { repos, loading, error, unavailable: !loading && !providerAvailable, search };
}
