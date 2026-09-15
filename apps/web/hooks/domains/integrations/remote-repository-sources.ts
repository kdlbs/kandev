"use client";

import { useEffect, useRef, useState } from "react";
import { fetchAccessibleRepos } from "@/lib/api/domains/github-api";
import { listUserProjects } from "@/lib/api/domains/gitlab-api";
import {
  listAzureDevOpsProjects,
  listAzureDevOpsRepositories,
} from "@/lib/api/domains/azure-devops-api";
import type { PluginRepositoryProviderRegistration } from "@/lib/plugins/registry";
import type {
  BuiltInRepositoryEligibility,
  RemoteRepository,
  RemoteRepositoryProvider,
  RemoteRepositorySourceError,
} from "./use-remote-repositories";

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
  sourceErrors: RemoteRepositorySourceError[];
};

type RepositoryRequest = {
  provider: RemoteRepositoryProvider;
  load: Promise<RemoteRepository[]>;
};

async function loadBuiltInRepositories(
  workspaceId: string,
  eligibleProviders: ReadonlySet<RemoteRepositoryProvider>,
): Promise<RemoteRepositoryLoad> {
  if (!workspaceId) return { repos: [], availableProviders: [], sourceErrors: [] };
  const requests: RepositoryRequest[] = [];
  if (eligibleProviders.has("github")) {
    requests.push({
      provider: "github",
      load: fetchAccessibleRepos({ workspaceId, limit: 100 }).then((repos) =>
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
    });
  }
  if (eligibleProviders.has("gitlab")) {
    requests.push({
      provider: "gitlab",
      load: listUserProjects(workspaceId).then(({ projects = [] }) =>
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
    });
  }
  if (eligibleProviders.has("azure_devops")) {
    requests.push({ provider: "azure_devops", load: loadAzureRepositories(workspaceId) });
  }
  return settleRepositoryRequests(requests);
}

async function loadPluginRepositories(
  workspaceId: string,
  pluginProviders: PluginRepositoryProviderRegistration[],
  query: string,
  signal: AbortSignal,
): Promise<RemoteRepositoryLoad> {
  if (!workspaceId) return { repos: [], availableProviders: [], sourceErrors: [] };
  return settleRepositoryRequests(
    pluginProviders.map((provider) => ({
      provider: provider.id,
      load: listAllPluginRepositories(provider, workspaceId, query, signal),
    })),
  );
}

async function settleRepositoryRequests(
  requests: RepositoryRequest[],
): Promise<RemoteRepositoryLoad> {
  const results = await Promise.allSettled(requests.map((request) => request.load));
  const availableProviders = results.flatMap((result, index) =>
    result.status === "fulfilled" ? [requests[index]!.provider] : [],
  );
  const sourceErrors = results.flatMap((result, index) =>
    result.status === "rejected"
      ? [{ provider: requests[index]!.provider, error: toError(result.reason) }]
      : [],
  );
  return {
    repos: results.flatMap((result) => (result.status === "fulfilled" ? result.value : [])),
    availableProviders,
    sourceErrors,
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
      providerScope: readString(repository.providerScope),
      defaultBranch: readString(repository.defaultBranch) ?? "",
      private: repository.private === true,
    },
  ];
}

function readString(value: unknown): string | undefined {
  return typeof value === "string" && value ? value : undefined;
}

function toError(cause: unknown): Error {
  return cause instanceof Error ? cause : new Error(String(cause));
}

type RepositorySourceState = {
  repos: RemoteRepository[];
  availableProviders: RemoteRepositoryProvider[];
  sourceErrors: RemoteRepositorySourceError[];
  loading: boolean;
};

export function useBuiltInRepositorySource(
  workspaceId: string,
  refreshVersion: number,
  eligibility: BuiltInRepositoryEligibility,
): RepositorySourceState {
  const [repos, setRepos] = useState<RemoteRepository[]>([]);
  const [loading, setLoading] = useState(true);
  const [sourceErrors, setSourceErrors] = useState<RemoteRepositorySourceError[]>([]);
  const [availableProviders, setAvailableProviders] = useState<RemoteRepositoryProvider[]>([]);
  const workspaceRef = useRef(workspaceId);

  useEffect(() => {
    let cancelled = false;
    const sameWorkspace = workspaceRef.current === workspaceId;
    workspaceRef.current = workspaceId;
    setRepos([]);
    setAvailableProviders((current) =>
      sameWorkspace ? current.filter((provider) => eligibility.providers.has(provider)) : [],
    );
    setSourceErrors([]);
    setLoading(true);
    if (eligibility.loading) {
      return () => {
        cancelled = true;
      };
    }
    loadBuiltInRepositories(workspaceId, eligibility.providers)
      .then((result) => {
        if (cancelled) return;
        setRepos(result.repos);
        setAvailableProviders(result.availableProviders);
        setSourceErrors(result.sourceErrors);
      })
      .catch((cause) => {
        if (!cancelled) setSourceErrors([{ provider: "built-in", error: toError(cause) }]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [eligibility.loading, eligibility.providers, refreshVersion, workspaceId]);

  return { repos, availableProviders, sourceErrors, loading };
}

export function usePluginRepositorySource(
  workspaceId: string,
  pluginProviders: PluginRepositoryProviderRegistration[],
  debouncedQuery: string,
  refreshVersion: number,
): RepositorySourceState {
  const [repos, setRepos] = useState<RemoteRepository[]>([]);
  const [loading, setLoading] = useState(true);
  const [sourceErrors, setSourceErrors] = useState<RemoteRepositorySourceError[]>([]);
  const [availableProviders, setAvailableProviders] = useState<RemoteRepositoryProvider[]>([]);
  const generationRef = useRef(0);

  useEffect(() => {
    setRepos([]);
    setAvailableProviders([]);
    setSourceErrors([]);
  }, [workspaceId, pluginProviders]);

  useEffect(() => {
    const generation = ++generationRef.current;
    const controller = new AbortController();
    setSourceErrors([]);
    setLoading(true);
    loadPluginRepositories(workspaceId, pluginProviders, debouncedQuery, controller.signal)
      .then((result) => {
        if (generation !== generationRef.current) return;
        setRepos(result.repos);
        setAvailableProviders(result.availableProviders);
        setSourceErrors(result.sourceErrors);
      })
      .catch((cause) => {
        if (generation !== generationRef.current || controller.signal.aborted) return;
        setSourceErrors([{ provider: "plugin", error: toError(cause) }]);
      })
      .finally(() => {
        if (generation === generationRef.current) setLoading(false);
      });
    return () => controller.abort();
  }, [debouncedQuery, pluginProviders, refreshVersion, workspaceId]);

  return { repos, availableProviders, sourceErrors, loading };
}
