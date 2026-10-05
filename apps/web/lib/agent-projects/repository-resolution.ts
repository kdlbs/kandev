import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { Repository } from "@/lib/types/http";
import { remoteRepositorySelectionIdentity } from "@/components/task-create-dialog-remote-repo-identity";
import { isRemoteBackedProjectRepository } from "./repositories";

export const PENDING_PROJECT_REPOSITORY_PREFIX = "remote:";

export class ProjectRepositoryResolutionError extends Error {
  constructor(readonly code: "ineligible" | "ambiguous" | "created_identity_mismatch") {
    super(code);
    this.name = "ProjectRepositoryResolutionError";
  }
}

export function projectRemoteRepositoryKey(repository: RemoteRepository): string {
  return remoteRepositorySelectionIdentity({
    ...repository,
    providerHost: repository.providerHost || providerHostFromURL(repository.url),
  });
}

export function projectRemoteSelectionId(repository: RemoteRepository): string {
  return `${PENDING_PROJECT_REPOSITORY_PREFIX}${projectRemoteRepositoryKey(repository)}`;
}

export async function resolveAgentProjectRepositories({
  workspaceId,
  selected,
  existing,
  cache,
  create,
}: {
  workspaceId: string;
  selected: RemoteRepository[];
  existing: Repository[];
  cache: Map<string, Repository>;
  create: (repository: RemoteRepository) => Promise<Repository>;
}): Promise<Repository[]> {
  const resolved: Repository[] = [];
  const seen = new Set<string>();
  for (const selectedRepository of selected) {
    assertProjectEligible(selectedRepository);
    const key = projectRemoteRepositoryKey(selectedRepository);
    if (seen.has(key)) continue;
    seen.add(key);
    const cached = cache.get(key);
    if (cached?.workspace_id === workspaceId && isRemoteBackedProjectRepository(cached)) {
      resolved.push(cached);
      continue;
    }
    const matchingRecords = existing.filter(
      (candidate) =>
        candidate.workspace_id === workspaceId &&
        isRemoteBackedProjectRepository(candidate) &&
        projectRepositoryMatchesRemote(candidate, selectedRepository),
    );
    if (matchingRecords.length > 1) {
      throw new ProjectRepositoryResolutionError("ambiguous");
    }
    const matchingRecord = matchingRecords[0];
    if (matchingRecord) {
      cache.set(key, matchingRecord);
      resolved.push(matchingRecord);
      continue;
    }
    const created = await create(selectedRepository);
    if (
      created.workspace_id !== workspaceId ||
      !isRemoteBackedProjectRepository(created) ||
      !projectRepositoryMatchesRemote(created, selectedRepository)
    ) {
      throw new ProjectRepositoryResolutionError("created_identity_mismatch");
    }
    cache.set(key, created);
    resolved.push(created);
  }
  return resolved;
}

function assertProjectEligible(repository: RemoteRepository) {
  if (
    !isRemoteBackedProjectRepository({
      source_type: "provider",
      remote_url: repository.url,
      provider_owner: repository.owner,
      provider_name: repository.name,
      default_branch: repository.defaultBranch,
    })
  ) {
    throw new ProjectRepositoryResolutionError("ineligible");
  }
}

export function projectRepositoryMatchesRemote(
  repository: Repository,
  remote: RemoteRepository,
): boolean {
  if (repository.provider.trim().toLowerCase() !== remote.provider.trim().toLowerCase()) {
    return false;
  }
  const selectedHost = selectedProviderHost(remote);
  const recordHost = repositoryProviderHost(repository);
  if (!selectedHost || !recordHost || selectedHost !== recordHost) return false;
  if ((repository.provider_scope ?? "") !== (remote.providerScope ?? "")) return false;
  if (repository.provider_repo_id && repository.provider_repo_id !== remote.id) return false;
  const normalizeName =
    remote.provider.toLowerCase() === "github"
      ? (value: string) => value.toLowerCase()
      : (value: string) => value;
  return (
    normalizeName(repository.provider_owner.trim()) === normalizeName(remote.owner.trim()) &&
    normalizeName(repository.provider_name.trim()) === normalizeName(remote.name.trim())
  );
}

function selectedProviderHost(repository: RemoteRepository): string {
  return normalizeProviderHost(
    repository.providerHost ||
      providerHostFromURL(repository.url) ||
      defaultProviderHost(repository.provider),
  );
}

function repositoryProviderHost(repository: Repository): string {
  return normalizeProviderHost(
    repository.provider_host ||
      providerHostFromURL(repository.remote_url ?? "") ||
      defaultProviderHost(repository.provider),
  );
}

function providerHostFromURL(value: string): string | undefined {
  try {
    const url = new URL(value);
    return `${url.protocol}//${url.host}`;
  } catch {
    return undefined;
  }
}

function defaultProviderHost(provider: string): string | undefined {
  switch (provider.toLowerCase()) {
    case "github":
      return "https://github.com";
    case "gitlab":
      return "https://gitlab.com";
    case "azure_devops":
      return "https://dev.azure.com";
    default:
      return undefined;
  }
}

function normalizeProviderHost(value: string | undefined): string {
  if (!value) return "";
  try {
    const candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(value) ? value : `https://${value}`;
    const url = new URL(candidate);
    const path = url.pathname.replace(/\/+$/, "");
    return `${url.protocol.toLowerCase()}//${url.host.toLowerCase()}${path}`;
  } catch {
    return value.trim().replace(/\/+$/, "").toLowerCase();
  }
}
