"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchAccessibleRepos } from "@/lib/api/domains/github-api";
import { listUserProjects } from "@/lib/api/domains/gitlab-api";
import {
  listAzureDevOpsProjects,
  listAzureDevOpsRepositories,
} from "@/lib/api/domains/azure-devops-api";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginRepositoryProviderRegistration } from "@/lib/plugins/registry";
import { looksLikeSupportedRemoteURL } from "@/components/workspace-source-picker/remote-url";

export const REMOTE_REPOSITORY_PROVIDERS = ["github", "gitlab", "azure_devops"] as const;

export type RemoteRepositoryProvider = (typeof REMOTE_REPOSITORY_PROVIDERS)[number] | (string & {});

export type RemoteRepository = {
  provider: RemoteRepositoryProvider;
  id: string;
  owner: string;
  name: string;
  fullName: string;
  url: string;
  providerHost?: string;
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
  matchesURL?: (url: string) => boolean;
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

type RepositoryRequest = {
  provider: RemoteRepositoryProvider;
  load: Promise<RemoteRepository[]>;
};

async function loadRemoteRepositories(
  workspaceId: string,
  pluginProviders: PluginRepositoryProviderRegistration[],
  signal: AbortSignal,
  query: string,
): Promise<RemoteRepositoryLoad> {
  const githubRequest = workspaceId
    ? fetchAccessibleRepos({ workspaceId, limit: 100 })
    : Promise.reject(new Error("workspace is required for GitHub repositories"));
  const gitLabRequest = workspaceId
    ? listUserProjects(workspaceId)
    : Promise.reject(new Error("workspace is required for GitLab repositories"));
  const azureRequest = workspaceId
    ? loadAzureRepositories(workspaceId)
    : Promise.reject(new Error("workspace is required for Azure DevOps repositories"));
  const requests: RepositoryRequest[] = [
    {
      provider: "github",
      load: githubRequest.then((repos) =>
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
    },
    {
      provider: "gitlab",
      load: gitLabRequest.then(({ projects = [] }) =>
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
    },
    { provider: "azure_devops", load: azureRequest },
    ...(workspaceId
      ? pluginProviders.map((provider) => ({
          provider: provider.id,
          load: listAllPluginRepositories(provider, workspaceId, query, signal),
        }))
      : []),
  ];
  const results = await Promise.allSettled(requests.map((request) => request.load));
  const availableProviders = results.flatMap((result, index) =>
    result.status === "fulfilled" ? [requests[index]!.provider] : [],
  );
  return {
    repos: results.flatMap((result) => (result.status === "fulfilled" ? result.value : [])),
    availableProviders,
  };
}

async function listAllPluginRepositories(
  provider: PluginRepositoryProviderRegistration,
  workspaceId: string,
  query: string,
  signal: AbortSignal,
): Promise<RemoteRepository[]> {
  const repositories: RemoteRepository[] = [];
  const seenCursors = new Set<string>();
  let cursor: string | undefined;
  do {
    const result = await provider.listRepositories({
      workspaceId,
      query,
      cursor,
      limit: 100,
      signal,
    });
    const page = Array.isArray(result) ? { repositories: result } : result;
    repositories.push(
      ...page.repositories.flatMap((repository) => toRemoteRepository(provider.id, repository)),
    );
    cursor = page.nextCursor;
    if (cursor) {
      if (seenCursors.has(cursor))
        throw new Error("Repository provider pagination did not advance");
      seenCursors.add(cursor);
    }
  } while (cursor);
  return repositories;
}

function toRemoteRepository(provider: string, value: unknown): RemoteRepository[] {
  if (!value || typeof value !== "object") return [];
  const repository = value as Record<string, unknown>;
  const repositoryId = readString(repository.repositoryId) ?? readString(repository.id);
  const owner = readString(repository.ownerOrProject) ?? readString(repository.owner);
  const name = readString(repository.repositoryName) ?? readString(repository.name);
  const url = readString(repository.cloneUrl) ?? readString(repository.url);
  if (!repositoryId || !owner || !name || !url) return [];
  return [
    {
      provider,
      id: repositoryId,
      owner,
      name,
      fullName: readString(repository.fullName) ?? `${owner}/${name}`,
      url,
      providerHost: readString(repository.providerHost),
      defaultBranch: readString(repository.defaultBranch) ?? "",
      private: repository.private === true,
    },
  ];
}

function readString(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

export function useRemoteRepositories(workspaceId: string): UseRemoteRepositoriesResult {
  const [allRepos, setAllRepos] = useState<RemoteRepository[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<Error | null>(null);
  const [availableProviders, setAvailableProviders] = useState<RemoteRepositoryProvider[]>([]);
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const pluginProviders = useMemo(
    () => registry.getRepositoryProviders(),
    [registry, registryVersion],
  );

  useEffect(() => {
    let cancelled = false;
    setAllRepos([]);
    setAvailableProviders([]);
    setError(null);
    setLoading(true);
    const controller = new AbortController();
    loadRemoteRepositories(workspaceId, pluginProviders, controller.signal, query)
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
      controller.abort();
    };
  }, [workspaceId, pluginProviders, query]);

  const repos = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return allRepos;
    return allRepos.filter((repo) => repo.fullName.toLowerCase().includes(needle));
  }, [allRepos, query]);
  const search = useCallback((value: string) => setQuery(value), []);
  const matchesURL = useCallback(
    (url: string) =>
      looksLikeSupportedRemoteURL(url) ||
      pluginProviders.some((provider) => provider.matchesURL(url)),
    [pluginProviders],
  );
  return {
    repos,
    availableProviders,
    loading,
    error,
    unavailable: !loading && availableProviders.length === 0,
    search,
    matchesURL,
  };
}
