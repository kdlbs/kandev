import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import { REMOTE_REPOSITORY_PROVIDERS } from "@/hooks/domains/integrations/use-remote-repositories";
import type { LocalRepository, Repository } from "@/lib/types/http";
import { repositorySlug } from "@/lib/repository-slug";
import {
  repositoryIdentityForLocalRepository,
  repositoryIdentityForRemoteRepository,
  repositoryIdentityForSavedRepository,
  type RepositoryRuleTarget,
} from "./repository-rule-identity";

export type RepositoryRuleCatalogGroup =
  | "workspace"
  | "local"
  | "remote"
  | "plugin"
  | "unavailable";

export type RepositoryRuleCatalogOption = {
  key: string;
  label: string;
  secondaryLabel?: string;
  group: RepositoryRuleCatalogGroup;
  target: RepositoryRuleTarget;
  available: boolean;
};

export type RepositoryRuleCatalogInput = {
  workspaceRepositories: readonly Repository[];
  localRepositories: readonly LocalRepository[];
  remoteRepositories: readonly RemoteRepository[];
  unavailableTargets?: readonly { target: RepositoryRuleTarget; label?: string }[];
  query?: string;
};

/**
 * Builds one repository picker from every source already available to the
 * workspace-source UI. Stable repository identities deduplicate saved and
 * discovered rows, while unavailable persisted targets remain selectable so a
 * user can replace or remove an old rule.
 */
export function buildRepositoryRuleCatalog(
  input: RepositoryRuleCatalogInput,
): RepositoryRuleCatalogOption[] {
  const options: RepositoryRuleCatalogOption[] = [];
  const seen = new Set<string>();

  const add = (option: Omit<RepositoryRuleCatalogOption, "key" | "available">) => {
    const key = repositoryRuleTargetKey(option.target);
    if (seen.has(key)) return;
    seen.add(key);
    options.push({ ...option, key, available: true });
  };

  for (const repository of input.workspaceRepositories) {
    add({
      label: repositorySlug(repository),
      secondaryLabel: repository.local_path || undefined,
      group: "workspace",
      target: repositoryIdentityForSavedRepository(repository),
    });
  }
  for (const repository of input.localRepositories) {
    add({
      label: repository.name || repository.path,
      secondaryLabel: repository.path,
      group: "local",
      target: repositoryIdentityForLocalRepository(repository),
    });
  }
  for (const repository of input.remoteRepositories) {
    add({
      label: repository.fullName || `${repository.owner}/${repository.name}`,
      secondaryLabel: repository.providerHost || repository.url,
      group: isBuiltInProvider(repository.provider) ? "remote" : "plugin",
      target: repositoryIdentityForRemoteRepository(repository),
    });
  }

  for (const unavailable of input.unavailableTargets ?? []) {
    const key = repositoryRuleTargetKey(unavailable.target);
    if (seen.has(key)) continue;
    seen.add(key);
    options.push({
      key,
      label: unavailable.label || key,
      group: "unavailable",
      target: unavailable.target,
      available: false,
    });
  }

  const needle = input.query?.trim().toLowerCase();
  if (!needle) return options;
  return options.filter((option) =>
    [option.label, option.secondaryLabel].some((value) => value?.toLowerCase().includes(needle)),
  );
}

export function repositoryRuleTargetKey(target: RepositoryRuleTarget): string {
  switch (target.kind) {
    case "workspace":
      return JSON.stringify([target.kind, target.workspace_id, target.repository_id]);
    case "provider":
      return JSON.stringify([
        target.kind,
        target.provider_id,
        target.host,
        target.scope,
        target.provider_repository_id,
      ]);
    case "local":
      return JSON.stringify([target.kind, target.path]);
  }
}

function isBuiltInProvider(provider: string): boolean {
  return (REMOTE_REPOSITORY_PROVIDERS as readonly string[]).includes(provider);
}
