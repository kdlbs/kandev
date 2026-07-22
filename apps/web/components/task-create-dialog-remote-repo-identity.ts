import type { RemoteRepository } from "@/hooks/domains/integrations/use-remote-repositories";
import type { TaskRemoteRepoRow } from "@/components/task-create-dialog-types";

export function selectedRemoteRepositoryIdentity(row: TaskRemoteRepoRow): string | undefined {
  if (row.provider && row.providerRepoId) return `${row.provider}:id:${row.providerRepoId}`;
  const url = normalizeRemoteRepositoryURL(row.url);
  return url ? `url:${url}` : undefined;
}

export function remoteRepositoryMatchesSelection(
  repo: RemoteRepository,
  selectedIdentity: string,
): boolean {
  if (selectedIdentity === `${repo.provider}:id:${repo.id}`) return true;
  const normalizedURL = normalizeRemoteRepositoryURL(repo.url);
  return Boolean(normalizedURL && selectedIdentity === `url:${normalizedURL}`);
}

export function normalizeRemoteRepositoryURL(value: string): string | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;
  const sshIdentity = remoteSSHRepositoryIdentity(trimmed);
  if (sshIdentity) return sshIdentity;
  const candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(trimmed) ? trimmed : `https://${trimmed}`;
  try {
    const url = new URL(candidate);
    const providerIdentity = remoteHTTPSRepositoryIdentity(url);
    if (providerIdentity) return providerIdentity;
    const path = url.pathname.replace(/\/+$/, "").replace(/\.git$/i, "");
    return `${url.protocol.toLowerCase()}//${url.hostname.toLowerCase()}${path}`;
  } catch {
    return trimmed
      .replace(/\/+$/, "")
      .replace(/\.git$/i, "")
      .toLowerCase();
  }
}

function remoteSSHRepositoryIdentity(value: string): string | undefined {
  const github = value.match(/^git@github\.com:([^/\s]+)\/([^/\s]+?)(?:\.git)?\/?$/i);
  if (github) return githubRepositoryIdentity(github[1], github[2]);

  const gitlab = value.match(/^git@gitlab\.com:([^\s]+?)(?:\.git)?\/?$/i);
  if (gitlab) return `gitlab:${gitlab[1]}`;

  const azure = value.match(
    /^git@ssh\.dev\.azure\.com:v3\/([^/\s]+)\/([^/\s]+)\/([^/\s]+?)(?:\.git)?\/?$/i,
  );
  return azure ? `azure_devops:${azure[1]}/${azure[2]}/${azure[3]}` : undefined;
}

function remoteHTTPSRepositoryIdentity(url: URL): string | undefined {
  const segments = url.pathname.split("/").filter(Boolean);
  switch (url.hostname.toLowerCase()) {
    case "github.com":
    case "www.github.com":
      return segments.length >= 2
        ? githubRepositoryIdentity(segments[0], stripGitSuffix(segments[1]))
        : undefined;
    case "gitlab.com": {
      const separatorIndex = segments.indexOf("-");
      const repositorySegments = separatorIndex >= 0 ? segments.slice(0, separatorIndex) : segments;
      return repositorySegments.length >= 2
        ? `gitlab:${repositorySegments.map(stripGitSuffix).join("/")}`
        : undefined;
    }
    case "dev.azure.com":
      return segments.length >= 4 && segments[2]?.toLowerCase() === "_git"
        ? `azure_devops:${segments[0]}/${segments[1]}/${stripGitSuffix(segments[3])}`
        : undefined;
    default:
      return undefined;
  }
}

function githubRepositoryIdentity(owner: string, repository: string): string {
  return `github:${owner.toLowerCase()}/${repository.toLowerCase()}`;
}

function stripGitSuffix(value: string): string {
  return value.replace(/\.git$/i, "");
}
