"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitLabStatus } from "@/hooks/domains/gitlab/use-gitlab-status";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import { INTEGRATION_STATUS_REFRESH_MS } from "./use-integration-availability";
import { useBuiltInRepositorySource, usePluginRepositorySource } from "./remote-repository-sources";
import { usePluginRepositoryAvailability } from "./remote-repository-provider-readiness";
import type { PluginIcon } from "@/lib/plugins/types";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { repositoryProviderMatchesURL } from "@/lib/plugins/repository-provider-url-resolution";
import { parseGitHubAnyUrl } from "@/hooks/domains/github/use-pr-info-by-url";
import { looksLikeSupportedRemoteURL } from "@/components/workspace-source-picker/remote-url";

export const REMOTE_REPOSITORY_PROVIDERS = ["github", "gitlab"] as const;

export type RemoteRepositoryProvider = (typeof REMOTE_REPOSITORY_PROVIDERS)[number] | (string & {});

export type RemoteRepositoryProviderReadiness = "loading" | "ready" | "unavailable" | "failed";

export type RemoteRepositoryProviderCatalogEntry = {
  provider: RemoteRepositoryProvider;
  label?: string;
  icon?: PluginIcon;
  readiness: RemoteRepositoryProviderReadiness;
  error?: Error;
};

export type RemoteRepository = {
  provider: RemoteRepositoryProvider;
  id: string;
  owner: string;
  name: string;
  fullName: string;
  url: string;
  providerHost?: string;
  providerScope?: string;
  defaultBranch: string;
  private: boolean;
};

export type UseRemoteRepositoriesResult = {
  repos: RemoteRepository[];
  availableProviders: RemoteRepositoryProvider[];
  providerCatalog?: RemoteRepositoryProviderCatalogEntry[];
  loading: boolean;
  error: Error | null;
  sourceErrors?: RemoteRepositorySourceError[];
  unavailable: boolean;
  search: (query: string) => void;
  refresh?: () => void;
  matchesURL?: (url: string) => boolean;
  /** The workspace's GitHub host, for parsing typed repository URLs. */
  githubHost?: string;
};

export type RemoteRepositorySourceError = {
  provider: RemoteRepositoryProvider;
  error: Error;
};

export type BuiltInRepositoryEligibility = {
  providers: ReadonlySet<RemoteRepositoryProvider>;
  loading: boolean;
};

type ProviderConnectionStatus = {
  authenticated?: boolean;
  token_configured?: boolean;
};

type BuiltInRepositoryAccess = {
  eligibility: BuiltInRepositoryEligibility;
  providerCatalog: RemoteRepositoryProviderCatalogEntry[];
  refresh: () => void;
  githubHost?: string;
};

export type GitHubRepositorySource = "workspace" | "scope-editor";

function hasProviderConnection(status: ProviderConnectionStatus | null | undefined) {
  return Boolean(status?.authenticated || status?.token_configured);
}

function readBuiltInReadiness(
  enabled: { loaded: boolean; enabled: boolean },
  loading: boolean,
  status: ProviderConnectionStatus | null,
): RemoteRepositoryProviderReadiness {
  if (!enabled.loaded || (!status && loading)) return "loading";
  return enabled.enabled && status?.authenticated === true ? "ready" : "unavailable";
}

function useBuiltInRepositoryAccess(
  workspaceId: string,
  allowed: ReadonlySet<string> | null,
  requireReadiness: boolean,
): BuiltInRepositoryAccess {
  const wantsGitHub = !allowed || allowed.has("github");
  const wantsGitLab = !allowed || allowed.has("gitlab");
  const githubStatus = useGitHubStatus(wantsGitHub ? workspaceId : "");
  const gitlabStatus = useGitLabStatus(wantsGitLab ? workspaceId : "");
  const githubEnabled = useGitHubEnabled(workspaceId);
  const gitlabEnabled = useGitLabEnabled(workspaceId);
  const githubReadiness = readBuiltInReadiness(
    githubEnabled,
    githubStatus.loading || !githubStatus.loaded,
    githubStatus.status,
  );
  const gitlabReadiness = readBuiltInReadiness(
    gitlabEnabled,
    gitlabStatus.loading,
    gitlabStatus.status,
  );
  const providerCatalog = useMemo<RemoteRepositoryProviderCatalogEntry[]>(
    () => [
      ...(wantsGitHub ? [{ provider: "github", readiness: githubReadiness }] : []),
      ...(wantsGitLab ? [{ provider: "gitlab", readiness: gitlabReadiness }] : []),
    ],
    [githubReadiness, gitlabReadiness, wantsGitHub, wantsGitLab],
  );

  const hasGitHubConnection = hasProviderConnection(githubStatus.status);
  const hasGitLabConnection = hasProviderConnection(gitlabStatus.status);
  const statusLoading =
    (wantsGitHub && (githubStatus.loading || !githubStatus.loaded)) ||
    (wantsGitLab && gitlabStatus.loading);
  const eligibility = useMemo(() => {
    const providers = new Set<RemoteRepositoryProvider>();
    if (requireReadiness) {
      for (const entry of providerCatalog) {
        if (entry.readiness === "ready") providers.add(entry.provider);
      }
    } else {
      if (wantsGitHub && hasGitHubConnection) providers.add("github");
      if (wantsGitLab && hasGitLabConnection) providers.add("gitlab");
    }
    return {
      providers,
      loading:
        Boolean(workspaceId) &&
        (requireReadiness
          ? providerCatalog.some((entry) => entry.readiness === "loading")
          : statusLoading),
    };
  }, [
    hasGitHubConnection,
    hasGitLabConnection,
    providerCatalog,
    requireReadiness,
    statusLoading,
    wantsGitHub,
    wantsGitLab,
    workspaceId,
  ]);
  const refresh = useCallback(() => {
    if (wantsGitHub) githubStatus.refresh();
    if (wantsGitLab) void gitlabStatus.refresh();
  }, [githubStatus.refresh, gitlabStatus.refresh, wantsGitHub, wantsGitLab]);
  return {
    eligibility,
    providerCatalog,
    refresh,
    githubHost: githubStatus.status?.automation?.github_host,
  };
}

export type UseRemoteRepositoriesOptions = {
  /** Restricts listing to these providers. Omitted means every connected provider. */
  providers?: readonly RemoteRepositoryProvider[];
  /** Selects the scope-editing discovery route for GitHub repositories. */
  githubRepositorySource?: GitHubRepositorySource;
  /** The mixed task picker requires a positive workspace connection check. */
  requireProviderReadiness?: boolean;
};

function useProviderRestriction(providers: readonly RemoteRepositoryProvider[] | undefined) {
  const key = providers ? providers.join("\u0000") : null;
  return useMemo(() => (key === null ? null : new Set(key.split("\u0000"))), [key]);
}

export function useRemoteRepositories(
  workspaceId: string,
  options?: UseRemoteRepositoriesOptions,
): UseRemoteRepositoriesResult {
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, 250);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const requireReadiness = options?.requireProviderReadiness === true;
  const allowedProviders = useProviderRestriction(options?.providers);
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const pluginProviders = useMemo(
    () =>
      registry
        .getRepositoryProviders()
        .filter((provider) => !allowedProviders || allowedProviders.has(provider.id)),
    [allowedProviders, registry, registryVersion],
  );
  const {
    eligibility,
    providerCatalog: builtInCatalog,
    refresh: refreshBuiltIns,
    githubHost,
  } = useBuiltInRepositoryAccess(workspaceId, allowedProviders, requireReadiness);
  const { readinessVersion, invalidate } = useRepositoryReadinessRefresh(
    requireReadiness ? workspaceId : "",
    refreshBuiltIns,
  );
  const pluginAvailability = usePluginRepositoryAvailability(
    requireReadiness ? workspaceId : "",
    pluginProviders,
    readinessVersion,
  );
  const builtInSource = useBuiltInRepositorySource(
    workspaceId,
    refreshVersion,
    eligibility,
    options?.githubRepositorySource ?? "workspace",
  );
  const pluginSource = usePluginRepositorySource(
    workspaceId,
    requireReadiness ? pluginAvailability.readyProviders : pluginProviders,
    debouncedQuery,
    refreshVersion,
  );

  const repos = useMemo(
    () => filterRepositoryResults([...builtInSource.repos, ...pluginSource.repos], query),
    [builtInSource.repos, pluginSource.repos, query],
  );
  const providerCatalog = useMemo(
    () => (requireReadiness ? [...builtInCatalog, ...pluginAvailability.catalog] : undefined),
    [builtInCatalog, pluginAvailability.catalog, requireReadiness],
  );
  const availableProviders = useMemo(
    () =>
      providerCatalog
        ? providerCatalog
            .filter((entry) => entry.readiness === "ready")
            .map((entry) => entry.provider)
        : [...builtInSource.availableProviders, ...pluginSource.availableProviders],
    [builtInSource.availableProviders, pluginSource.availableProviders, providerCatalog],
  );
  const loading = builtInSource.loading || pluginSource.loading || pluginAvailability.loading;
  const sourceErrors = useMemo(
    () => [
      ...builtInSource.sourceErrors,
      ...pluginAvailability.sourceErrors,
      ...pluginSource.sourceErrors,
    ],
    [builtInSource.sourceErrors, pluginAvailability.sourceErrors, pluginSource.sourceErrors],
  );
  const error = sourceErrors[0]?.error ?? null;
  const search = useCallback((value: string) => setQuery(value), []);
  const refresh = useCallback(() => {
    refreshBuiltIns();
    setRefreshVersion((version) => version + 1);
    invalidate();
  }, [invalidate, refreshBuiltIns]);
  const matchesURL = useCallback(
    (url: string) =>
      Boolean(parseGitHubAnyUrl(url, githubHost)) ||
      looksLikeSupportedRemoteURL(url) ||
      pluginProviders.some((provider) => repositoryProviderMatchesURL(provider, url)),
    [githubHost, pluginProviders],
  );
  return {
    repos,
    availableProviders,
    providerCatalog,
    loading,
    error,
    sourceErrors,
    unavailable: !loading && availableProviders.length === 0,
    search,
    refresh,
    matchesURL,
    githubHost,
  };
}

function filterRepositoryResults(repositories: RemoteRepository[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return repositories;
  return repositories.filter((repo) =>
    [repo.fullName, repo.providerHost, repo.url].some((value) =>
      value?.toLowerCase().includes(needle),
    ),
  );
}

function useRepositoryReadinessRefresh(workspaceId: string, refreshBuiltIns: () => void) {
  const [readinessVersion, setReadinessVersion] = useState(0);
  const invalidate = useCallback(() => setReadinessVersion((version) => version + 1), []);
  useEffect(() => {
    if (!workspaceId) return;
    const unsubscribe = subscribeIntegrationAvailability(invalidate);
    const interval = window.setInterval(() => {
      refreshBuiltIns();
      invalidate();
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => {
      unsubscribe();
      window.clearInterval(interval);
    };
  }, [invalidate, refreshBuiltIns, workspaceId]);
  return { readinessVersion, invalidate };
}

function useDebouncedValue<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}
