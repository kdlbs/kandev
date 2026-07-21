"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchAccessibleRepos } from "@/lib/api/domains/github-api";
import { listUserProjects } from "@/lib/api/domains/gitlab-api";
import {
  listAzureDevOpsProjects,
  listAzureDevOpsRepositories,
} from "@/lib/api/domains/azure-devops-api";

export const REMOTE_REPOSITORY_PROVIDERS = ["github", "gitlab", "azure_devops"] as const;

export type RemoteRepositoryProvider = (typeof REMOTE_REPOSITORY_PROVIDERS)[number];

export type RemoteRepository = {
  provider: RemoteRepositoryProvider;
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
  availableProviders: RemoteRepositoryProvider[];
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
  availableProviders: RemoteRepositoryProvider[];
};

async function loadRemoteRepositories(workspaceId: string): Promise<RemoteRepositoryLoad> {
  const gitLabRequest = workspaceId
    ? listUserProjects(workspaceId)
    : Promise.reject(new Error("workspace is required for GitLab repositories"));
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
    gitLabRequest.then(({ projects = [] }) =>
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
  const availableProviders = results.flatMap((result, index) =>
    result.status === "fulfilled" ? [REMOTE_REPOSITORY_PROVIDERS[index]] : [],
  );
  return {
    repos: results.flatMap((result) => (result.status === "fulfilled" ? result.value : [])),
    availableProviders,
  };
}

export function useRemoteRepositories(workspaceId: string): UseRemoteRepositoriesResult {
  const [allRepos, setAllRepos] = useState<RemoteRepository[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [availableProviders, setAvailableProviders] = useState<RemoteRepositoryProvider[]>([]);

  useEffect(() => {
    let cancelled = false;
    setAllRepos([]);
    setAvailableProviders([]);
    setError(null);
    setLoading(true);
    loadRemoteRepositories(workspaceId)
      .then((result) => {
        if (cancelled) return;
        setAllRepos(result.repos);
        setAvailableProviders(result.availableProviders);
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
  return {
    repos,
    availableProviders,
    loading,
    error,
    unavailable: !loading && availableProviders.length === 0,
    search,
  };
}
