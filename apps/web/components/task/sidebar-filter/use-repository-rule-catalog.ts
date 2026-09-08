"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRemoteRepositories } from "@/hooks/domains/integrations/use-remote-repositories";
import { useWorkspaceRepositoryOptions } from "@/components/task/add-workspace-sources/use-workspace-repository-options";
import type { RepositoryRuleTarget } from "@/lib/sidebar/repository-rule-identity";
import {
  buildRepositoryRuleCatalog,
  type RepositoryRuleCatalogOption,
} from "@/lib/sidebar/repository-rule-catalog";

export function useRepositoryRuleCatalog(
  workspaceId: string | null,
  open: boolean,
  unavailableTargets: readonly { target: RepositoryRuleTarget; label?: string }[] = [],
) {
  const repositoryOptions = useWorkspaceRepositoryOptions(workspaceId, open);
  const remoteRepositories = useRemoteRepositories(open && workspaceId ? workspaceId : "");
  const [query, setQuery] = useState("");

  useEffect(() => {
    remoteRepositories.search(query);
  }, [query, remoteRepositories.search]);

  const options = useMemo<RepositoryRuleCatalogOption[]>(
    () =>
      buildRepositoryRuleCatalog({
        workspaceRepositories: repositoryOptions.repositories,
        localRepositories: repositoryOptions.discoveredRepositories,
        remoteRepositories: remoteRepositories.repos,
        unavailableTargets,
        query,
      }),
    [
      query,
      remoteRepositories.repos,
      repositoryOptions.discoveredRepositories,
      repositoryOptions.repositories,
      unavailableTargets,
    ],
  );

  const refresh = useCallback(() => {
    repositoryOptions.refreshRepositoryOptions();
    remoteRepositories.refresh?.();
  }, [remoteRepositories, repositoryOptions]);

  return {
    options,
    query,
    setQuery,
    loading: repositoryOptions.repositoriesRefreshing || remoteRepositories.loading,
    error: repositoryOptions.error ?? remoteRepositories.error,
    refresh,
  };
}
