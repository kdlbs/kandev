import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { LocalRepository } from "@/lib/types/http";

export type RepositoryRuleTarget =
  | { kind: "workspace"; workspace_id: string; repository_id: string }
  | {
      kind: "provider";
      provider_id: string;
      host: string;
      scope: string;
      provider_repository_id: string;
    }
  | { kind: "local"; path: string };

export type TaskRepositoryRuleIdentity = RepositoryRuleTarget;

type RepositoryIdentitySource = {
  id: string;
  workspace_id: string;
  provider: string;
  provider_repo_id: string;
  provider_host?: string;
  provider_scope?: string;
  provider_owner?: string;
  remote_url?: string;
  local_path: string;
};

export function repositoryIdentityForSavedRepository(
  repository: RepositoryIdentitySource,
): RepositoryRuleTarget {
  if (repository.provider && repository.provider_repo_id) {
    return providerTarget(
      repository.provider,
      repository.provider_repo_id,
      repository.provider_host,
      repository.provider_scope || repository.provider_owner,
      repository.remote_url,
    );
  }
  if (repository.local_path)
    return { kind: "local", path: normalizeLocalRepositoryPath(repository.local_path) };
  return {
    kind: "workspace",
    workspace_id: repository.workspace_id,
    repository_id: repository.id,
  };
}

export function repositoryIdentityForTaskRepository(repository: {
  repository_id: string;
  workspace_id?: string;
  provider?: string;
  provider_repo_id?: string;
  provider_host?: string;
  provider_scope?: string;
  local_path?: string;
}): RepositoryRuleTarget {
  if (repository.provider && repository.provider_repo_id) {
    return providerTarget(
      repository.provider,
      repository.provider_repo_id,
      repository.provider_host,
      repository.provider_scope,
      undefined,
    );
  }
  if (repository.local_path)
    return { kind: "local", path: normalizeLocalRepositoryPath(repository.local_path) };
  return {
    kind: "workspace",
    workspace_id: repository.workspace_id ?? "",
    repository_id: repository.repository_id,
  };
}

export function repositoryIdentityForRemoteRepository(
  repository: RemoteRepository,
): RepositoryRuleTarget {
  return providerTarget(
    repository.provider,
    repository.id,
    repository.providerHost,
    repository.providerScope || repository.owner,
    repository.url,
  );
}

export function repositoryIdentityForLocalRepository(
  repository: LocalRepository,
): RepositoryRuleTarget {
  return { kind: "local", path: normalizeLocalRepositoryPath(repository.path) };
}

export function repositoryRuleTargetMatches(
  target: RepositoryRuleTarget,
  taskWorkspaceId: string | undefined,
  identities: readonly TaskRepositoryRuleIdentity[],
): boolean {
  return identities.some((identity) => {
    if (target.kind !== identity.kind) return false;
    if (target.kind === "workspace" && identity.kind === "workspace") {
      return (
        target.workspace_id === taskWorkspaceId &&
        identity.workspace_id === taskWorkspaceId &&
        target.repository_id === identity.repository_id
      );
    }
    if (target.kind === "provider" && identity.kind === "provider") {
      return (
        target.provider_id === identity.provider_id &&
        target.host === identity.host &&
        target.scope === identity.scope &&
        target.provider_repository_id === identity.provider_repository_id
      );
    }
    return (
      target.kind === "local" &&
      identity.kind === "local" &&
      normalizeLocalRepositoryPath(target.path) === normalizeLocalRepositoryPath(identity.path)
    );
  });
}

export function isRepositoryRuleTarget(value: unknown): value is RepositoryRuleTarget {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const candidate = value as Record<string, unknown>;
  if (candidate.kind === "workspace") {
    return (
      typeof candidate.workspace_id === "string" && typeof candidate.repository_id === "string"
    );
  }
  if (candidate.kind === "provider") {
    return (
      typeof candidate.provider_id === "string" &&
      typeof candidate.host === "string" &&
      typeof candidate.scope === "string" &&
      typeof candidate.provider_repository_id === "string"
    );
  }
  return candidate.kind === "local" && typeof candidate.path === "string";
}

export function normalizeLocalRepositoryPath(path: string): string {
  const normalized = path
    .replaceAll("\\", "/")
    .replace(/\/+$|^\/+/, (match) => (match.startsWith("/") ? "/" : ""));
  if (normalized.length > 1 && normalized.endsWith("/")) return normalized.slice(0, -1);
  return normalized;
}

function providerTarget(
  provider: string,
  repositoryId: string,
  host: string | undefined,
  scope: string | undefined,
  remoteURL: string | undefined,
): RepositoryRuleTarget {
  return {
    kind: "provider",
    provider_id: provider,
    host:
      normalizeProviderHost(host) ||
      normalizeProviderHostFromURL(remoteURL) ||
      defaultProviderHost(provider),
    scope: scope ?? "",
    provider_repository_id: repositoryId,
  };
}

function normalizeProviderHost(value: string | undefined): string | undefined {
  if (!value) return undefined;
  const trimmed = value.trim().replace(/\/+$/, "");
  if (!trimmed) return undefined;
  try {
    const parsed = new URL(trimmed.includes("://") ? trimmed : `https://${trimmed}`);
    return `${parsed.host}${parsed.pathname === "/" ? "" : parsed.pathname}`.replace(/\/+$/, "");
  } catch {
    return trimmed;
  }
}

function normalizeProviderHostFromURL(value: string | undefined): string | undefined {
  if (!value) return undefined;
  try {
    return new URL(value).host;
  } catch {
    return undefined;
  }
}

function defaultProviderHost(provider: string): string {
  switch (provider) {
    case "github":
      return "github.com";
    case "gitlab":
      return "gitlab.com";
    case "azure_devops":
      return "dev.azure.com";
    default:
      return "";
  }
}
