"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useGitHubStatus } from "@/hooks/domains/github/use-github-status";
import { useGitHubEnabled } from "@/hooks/domains/github/use-github-enabled";
import { useGitLabStatus } from "@/hooks/domains/gitlab/use-gitlab-status";
import { useGitLabEnabled } from "@/hooks/domains/gitlab/use-gitlab-enabled";
import { useAzureDevOpsConnection } from "@/hooks/domains/azure-devops/use-azure-devops-browse";
import { useAzureDevOpsEnabled } from "@/hooks/domains/azure-devops/use-azure-devops-enabled";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { subscribeIntegrationAvailability } from "@/lib/integrations/integration-availability-events";
import { usePluginRegistry } from "@/lib/plugins/registry";
import type { PluginRepositoryProviderRegistration } from "@/lib/plugins/registry";
import type { PluginIcon, RepositoryProviderAvailability } from "@/lib/plugins/types";
import { repositoryProviderMatchesURL } from "@/lib/plugins/repository-provider-url-resolution";
import { looksLikeSupportedRemoteURL } from "@/components/workspace-source-picker/remote-url";
import { useBuiltInRepositorySource, usePluginRepositorySource } from "./remote-repository-sources";

export const REMOTE_REPOSITORY_PROVIDERS = ["github", "gitlab", "azure_devops"] as const;

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
  /** Optional for compatibility with existing consumers that provide a test double. */
  providerCatalog?: RemoteRepositoryProviderCatalogEntry[];
  loading: boolean;
  error: Error | null;
  sourceErrors?: RemoteRepositorySourceError[];
  unavailable: boolean;
  search: (query: string) => void;
  refresh?: () => void;
  matchesURL?: (url: string) => boolean;
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
};

type BuiltInRepositoryAccess = {
  eligibility: BuiltInRepositoryEligibility;
  providerCatalog: RemoteRepositoryProviderCatalogEntry[];
  refresh: () => void;
};

function hasVerifiedProviderConnection(status: ProviderConnectionStatus | null | undefined) {
  return status?.authenticated === true;
}

function useBuiltInRepositoryAccess(workspaceId: string): BuiltInRepositoryAccess {
  const githubStatus = useGitHubStatus(workspaceId);
  const githubEnabled = useGitHubEnabled(workspaceId);
  const gitlabStatus = useGitLabStatus(workspaceId);
  const gitlabEnabled = useGitLabEnabled(workspaceId);
  const azureDevOpsConnection = useAzureDevOpsConnection(workspaceId || undefined);
  const azureDevOpsEnabled = useAzureDevOpsEnabled(workspaceId);
  const providers = useMemo(() => {
    const eligible = new Set<RemoteRepositoryProvider>();
    if (githubEnabled.enabled && hasVerifiedProviderConnection(githubStatus.status)) {
      eligible.add("github");
    }
    if (gitlabEnabled.enabled && hasVerifiedProviderConnection(gitlabStatus.status)) {
      eligible.add("gitlab");
    }
    if (
      azureDevOpsEnabled.enabled &&
      azureDevOpsConnection.data?.hasSecret &&
      azureDevOpsConnection.data.lastOk
    ) {
      eligible.add("azure_devops");
    }
    return eligible;
  }, [
    azureDevOpsConnection.data?.hasSecret,
    azureDevOpsConnection.data?.lastOk,
    azureDevOpsEnabled.enabled,
    githubEnabled.enabled,
    githubStatus.status?.authenticated,
    gitlabEnabled.enabled,
    gitlabStatus.status?.authenticated,
  ]);
  const eligibility = useMemo(
    () => ({
      providers,
      loading:
        Boolean(workspaceId) &&
        ((githubEnabled.enabled && (githubStatus.loading || !githubStatus.loaded)) ||
          (gitlabEnabled.enabled && gitlabStatus.loading) ||
          (azureDevOpsEnabled.enabled && azureDevOpsConnection.loading)),
    }),
    [
      azureDevOpsConnection.loading,
      azureDevOpsEnabled.enabled,
      githubEnabled.enabled,
      gitlabStatus.loading,
      gitlabEnabled.enabled,
      githubStatus.loaded,
      githubStatus.loading,
      providers,
      workspaceId,
    ],
  );
  const providerCatalog = useBuiltInProviderCatalog({
    githubEnabled: githubEnabled.enabled,
    githubLoading: githubStatus.loading || !githubStatus.loaded,
    githubConnected: hasVerifiedProviderConnection(githubStatus.status),
    gitlabEnabled: gitlabEnabled.enabled,
    gitlabLoading: gitlabStatus.loading,
    gitlabConnected: hasVerifiedProviderConnection(gitlabStatus.status),
    azureEnabled: azureDevOpsEnabled.enabled,
    azureLoading: azureDevOpsConnection.loading,
    azureConnected: Boolean(
      azureDevOpsConnection.data?.hasSecret && azureDevOpsConnection.data.lastOk,
    ),
    azureError: azureDevOpsConnection.error,
  });
  const refresh = useCallback(() => {
    void githubStatus.refresh();
    void gitlabStatus.refresh();
    azureDevOpsConnection.refresh();
  }, [azureDevOpsConnection.refresh, githubStatus.refresh, gitlabStatus.refresh]);
  return { eligibility, providerCatalog, refresh };
}

function useBuiltInProviderCatalog({
  githubEnabled,
  githubLoading,
  githubConnected,
  gitlabEnabled,
  gitlabLoading,
  gitlabConnected,
  azureEnabled,
  azureLoading,
  azureConnected,
  azureError,
}: {
  githubEnabled: boolean;
  githubLoading: boolean;
  githubConnected: boolean;
  gitlabEnabled: boolean;
  gitlabLoading: boolean;
  gitlabConnected: boolean;
  azureEnabled: boolean;
  azureLoading: boolean;
  azureConnected: boolean;
  azureError?: string | null;
}) {
  return useMemo<RemoteRepositoryProviderCatalogEntry[]>(
    () => [
      {
        provider: "github",
        readiness: builtInProviderReadiness(githubEnabled, githubLoading, githubConnected),
      },
      {
        provider: "gitlab",
        readiness: builtInProviderReadiness(gitlabEnabled, gitlabLoading, gitlabConnected),
      },
      {
        provider: "azure_devops",
        readiness: builtInProviderReadiness(azureEnabled, azureLoading, azureConnected),
        ...(azureError ? { error: new Error(azureError) } : {}),
      },
    ],
    [
      azureConnected,
      azureEnabled,
      azureError,
      azureLoading,
      githubConnected,
      githubEnabled,
      githubLoading,
      gitlabConnected,
      gitlabEnabled,
      gitlabLoading,
    ],
  );
}

function builtInProviderReadiness(
  enabled: boolean,
  loading: boolean,
  connected: boolean,
): RemoteRepositoryProviderReadiness {
  if (loading && enabled) return "loading";
  return enabled && connected ? "ready" : "unavailable";
}

type PluginRepositoryAvailabilityState = {
  scopeKey: string;
  providerScopeKey: string;
  providerCatalog: RemoteRepositoryProviderCatalogEntry[];
  readyProviders: PluginRepositoryProviderRegistration[];
  sourceErrors: RemoteRepositorySourceError[];
  loading: boolean;
};

function hasPositiveAvailability(
  availability: RepositoryProviderAvailability | null | undefined,
): boolean {
  return (
    availability?.configured === true &&
    availability.enabled === true &&
    availability.tested === true
  );
}

function boundedError(cause: unknown): Error {
  const error = toError(cause);
  return new Error(error.message.slice(0, 512));
}

function toError(cause: unknown): Error {
  return cause instanceof Error ? cause : new Error(String(cause));
}

function initialPluginProviderCatalog(
  providers: PluginRepositoryProviderRegistration[],
): RemoteRepositoryProviderCatalogEntry[] {
  return providers.map((provider) => ({
    provider: provider.id,
    label: provider.label,
    icon: provider.icon,
    readiness: provider.getAvailability ? "loading" : "unavailable",
  }));
}

function reusableReadyProviders(
  previous: PluginRepositoryAvailabilityState,
  providerScopeKey: string,
  providers: PluginRepositoryProviderRegistration[],
): PluginRepositoryProviderRegistration[] {
  if (previous.providerScopeKey !== providerScopeKey) return [];
  const reusable = previous.readyProviders.filter(
    (provider) => provider.getAvailability && providers.includes(provider),
  );
  return reusable.length === previous.readyProviders.length ? previous.readyProviders : reusable;
}

function initialPluginAvailabilityState(
  scopeKey: string,
  providerScopeKey: string,
  providers: PluginRepositoryProviderRegistration[],
): PluginRepositoryAvailabilityState {
  return {
    scopeKey,
    providerScopeKey,
    providerCatalog: initialPluginProviderCatalog(providers),
    readyProviders: [],
    sourceErrors: [],
    loading: false,
  };
}

function buildPluginAvailabilityLoadingState({
  previous,
  scopeKey,
  providerScopeKey,
  providerCatalog,
  providers,
  workspaceId,
}: {
  previous: PluginRepositoryAvailabilityState;
  scopeKey: string;
  providerScopeKey: string;
  providerCatalog: RemoteRepositoryProviderCatalogEntry[];
  providers: PluginRepositoryProviderRegistration[];
  workspaceId: string;
}): PluginRepositoryAvailabilityState {
  const providersWithAvailability = providers.filter((provider) => provider.getAvailability);
  return {
    scopeKey,
    providerScopeKey,
    providerCatalog,
    readyProviders: reusableReadyProviders(previous, providerScopeKey, providers),
    sourceErrors: [],
    loading: Boolean(workspaceId) && providersWithAvailability.length > 0,
  };
}

function stalePluginAvailabilityState(
  state: PluginRepositoryAvailabilityState,
  scopeKey: string,
  providerScopeKey: string,
  providers: PluginRepositoryProviderRegistration[],
  workspaceId: string,
): PluginRepositoryAvailabilityState {
  return {
    scopeKey,
    providerScopeKey,
    providerCatalog: initialPluginProviderCatalog(providers),
    readyProviders: reusableReadyProviders(state, providerScopeKey, providers),
    sourceErrors: [],
    loading: Boolean(workspaceId) && providers.some((provider) => provider.getAvailability),
  };
}

function usePluginRepositoryAvailability(
  workspaceId: string,
  providers: PluginRepositoryProviderRegistration[],
  refreshVersion: number,
  readinessVersion: number,
): PluginRepositoryAvailabilityState {
  const providerScopeKey = `${workspaceId}\u0000${providers.map((provider) => provider.id).join("\u0000")}`;
  const scopeKey = `${providerScopeKey}\u0000${refreshVersion}\u0000${readinessVersion}`;
  const [state, setState] = useState<PluginRepositoryAvailabilityState>(() =>
    initialPluginAvailabilityState(scopeKey, providerScopeKey, providers),
  );
  const generationRef = useRef(0);

  useEffect(() => {
    const generation = ++generationRef.current;
    const controller = new AbortController();
    let cancelled = false;
    const initialCatalog = initialPluginProviderCatalog(providers);
    const providersWithAvailability = providers.filter((provider) => provider.getAvailability);
    setState((previous) =>
      buildPluginAvailabilityLoadingState({
        previous,
        scopeKey,
        providerScopeKey,
        providerCatalog: initialCatalog,
        providers,
        workspaceId,
      }),
    );
    if (!workspaceId || providersWithAvailability.length === 0) {
      return () => {
        cancelled = true;
        controller.abort();
      };
    }

    const load = async () => {
      const results = await Promise.all(
        providersWithAvailability.map(async (provider) => {
          try {
            const availability = await provider.getAvailability!({
              workspaceId,
              signal: controller.signal,
            });
            return {
              provider,
              readiness: hasPositiveAvailability(availability)
                ? ("ready" as const)
                : ("unavailable" as const),
            };
          } catch (cause) {
            if (controller.signal.aborted) return { provider, readiness: "unavailable" as const };
            const error = boundedError(cause);
            return { provider, readiness: "failed" as const, error };
          }
        }),
      );
      if (cancelled || generation !== generationRef.current || controller.signal.aborted) return;
      const resultsByProvider = new Map(results.map((result) => [result.provider.id, result]));
      const providerCatalog = initialCatalog.map((entry) => {
        const result = resultsByProvider.get(entry.provider);
        return result
          ? {
              ...entry,
              readiness: result.readiness,
              ...(result.error ? { error: result.error } : {}),
            }
          : entry;
      });
      const nextReadyProviders = results
        .filter((result) => result.readiness === "ready")
        .map((result) => result.provider);
      const sourceErrors = results.flatMap((result) =>
        result.error ? [{ provider: result.provider.id, error: result.error }] : [],
      );
      setState((previous) => ({
        scopeKey,
        providerScopeKey,
        providerCatalog,
        readyProviders: sameProviders(previous.readyProviders, nextReadyProviders)
          ? previous.readyProviders
          : nextReadyProviders,
        sourceErrors,
        loading: false,
      }));
    };
    void load();
    return () => {
      cancelled = true;
      controller.abort();
    };
  }, [providers, readinessVersion, refreshVersion, workspaceId]);

  if (state.scopeKey !== scopeKey) {
    return stalePluginAvailabilityState(state, scopeKey, providerScopeKey, providers, workspaceId);
  }
  return state;
}

function sameProviders(
  left: PluginRepositoryProviderRegistration[],
  right: PluginRepositoryProviderRegistration[],
): boolean {
  return left.length === right.length && left.every((provider, index) => provider === right[index]);
}

export function useRemoteRepositories(workspaceId: string): UseRemoteRepositoriesResult {
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebouncedValue(query, 250);
  const [refreshVersion, setRefreshVersion] = useState(0);
  const [readinessVersion, setReadinessVersion] = useState(0);
  const registry = usePluginRegistry();
  const registryVersion = registry.getVersion();
  const pluginProviders = useMemo(
    () => registry.getRepositoryProviders(),
    [registry, registryVersion],
  );
  const builtInAccess = useBuiltInRepositoryAccess(workspaceId);
  const pluginAvailability = usePluginRepositoryAvailability(
    workspaceId,
    pluginProviders,
    refreshVersion,
    readinessVersion,
  );
  const { eligibility, refresh: refreshBuiltIns } = builtInAccess;
  const builtInSource = useBuiltInRepositorySource(workspaceId, refreshVersion, eligibility);
  const pluginSource = usePluginRepositorySource(
    workspaceId,
    pluginAvailability.readyProviders,
    debouncedQuery,
    refreshVersion,
  );

  const repos = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const allRepos = [...builtInSource.repos, ...pluginSource.repos];
    if (!needle) return allRepos;
    return allRepos.filter((repo) =>
      [repo.fullName, repo.providerHost, repo.url].some((value) =>
        value?.toLowerCase().includes(needle),
      ),
    );
  }, [builtInSource.repos, pluginSource.repos, query]);
  const availableProviders = useMemo(
    () => [
      ...eligibility.providers,
      ...pluginAvailability.readyProviders.map((provider) => provider.id),
    ],
    [eligibility.providers, pluginAvailability.readyProviders],
  );
  const providerCatalog = useMemo(
    () => [...builtInAccess.providerCatalog, ...pluginAvailability.providerCatalog],
    [builtInAccess.providerCatalog, pluginAvailability.providerCatalog],
  );
  const loading = builtInSource.loading || pluginAvailability.loading || pluginSource.loading;
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
  const refreshReadiness = useCallback(() => {
    refreshBuiltIns();
    setReadinessVersion((version) => version + 1);
  }, [refreshBuiltIns]);
  const refresh = useCallback(() => {
    refreshBuiltIns();
    setRefreshVersion((version) => version + 1);
    setReadinessVersion((version) => version + 1);
  }, [refreshBuiltIns]);
  useEffect(() => {
    const unsubscribe = subscribeIntegrationAvailability(refreshReadiness);
    const interval = window.setInterval(refreshReadiness, INTEGRATION_STATUS_REFRESH_MS);
    return () => {
      unsubscribe();
      window.clearInterval(interval);
    };
  }, [refreshReadiness]);
  const matchesURL = useCallback(
    (url: string) =>
      looksLikeSupportedRemoteURL(url) ||
      pluginProviders.some((provider) => repositoryProviderMatchesURL(provider, url)),
    [pluginProviders],
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
  };
}

function useDebouncedValue<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [delay, value]);
  return debounced;
}
