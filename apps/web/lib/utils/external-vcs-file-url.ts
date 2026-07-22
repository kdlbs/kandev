export type ExternalVcsProvider = "github" | "gitlab" | "azure_devops";

export type ExternalVcsRepository = {
  provider: string;
  provider_host?: string;
  provider_owner: string;
  provider_name: string;
  remote_url?: string;
};

export type ExternalVcsFileURLInput = {
  repository: ExternalVcsRepository;
  path: string;
  previousPath?: string | null;
  status?: string | null;
  publishedBranch?: string | null;
  publishedPullRequestNumber?: number | null;
  baseBranch?: string | null;
};

export type ExternalVcsFileURL = {
  provider: ExternalVcsProvider;
  url: string;
  path: string;
  revision: string;
};

type ResolvedTarget = { path: string; revision: string; usesPublishedRevision: boolean };
type SSHCloneIdentity = { hostname: string; parts: string[] };

function cleanValue(value: string | null | undefined): string {
  return value?.trim() ?? "";
}

function isSafeRef(value: string): boolean {
  return value.length > 0 && !/[\u0000-\u001f\u007f]/.test(value);
}

function isSafeRepositoryPath(value: string): boolean {
  if (
    !value ||
    value.startsWith("/") ||
    value.startsWith("\\") ||
    value.includes("\\") ||
    /[\u0000-\u001f\u007f]/.test(value)
  ) {
    return false;
  }
  const segments = value.split("/");
  return segments.every((segment) => segment !== "" && segment !== "." && segment !== "..");
}

function selectTarget(input: ExternalVcsFileURLInput): ResolvedTarget | null {
  const publishedBranch = cleanValue(input.publishedBranch);
  const baseBranch = cleanValue(input.baseBranch);
  const currentPath = input.path;
  const previousPath = input.previousPath ?? "";
  const status = cleanValue(input.status).toLowerCase();

  if (status === "deleted") {
    return baseBranch
      ? { path: currentPath, revision: baseBranch, usesPublishedRevision: false }
      : null;
  }
  if (status === "renamed") {
    if (publishedBranch) {
      return { path: currentPath, revision: publishedBranch, usesPublishedRevision: true };
    }
    return baseBranch && previousPath
      ? { path: previousPath, revision: baseBranch, usesPublishedRevision: false }
      : null;
  }
  if ((status === "added" || status === "untracked") && !publishedBranch) return null;
  const revision = publishedBranch || baseBranch;
  return revision
    ? { path: currentPath, revision, usesPublishedRevision: Boolean(publishedBranch) }
    : null;
}

function parseHTTPSRemote(rawRemoteURL: string | undefined): URL | null {
  if (!rawRemoteURL || /[\u0000-\u001f\u007f]/.test(rawRemoteURL)) return null;
  try {
    const remote = new URL(cleanValue(rawRemoteURL));
    if (
      remote.protocol !== "https:" ||
      remote.username ||
      remote.password ||
      remote.search ||
      remote.hash
    ) {
      return null;
    }
    return remote;
  } catch {
    return null;
  }
}

function decodeSSHPath(rawPath: string): string[] | null {
  try {
    const rawParts = rawPath.replace(/^\/+/, "").split("/");
    if (rawParts.some((part) => !part)) return null;
    const parts = rawParts.map((part) => decodeURIComponent(part));
    const lastIndex = parts.length - 1;
    parts[lastIndex] = parts[lastIndex].replace(/\.git$/, "");
    if (
      parts.some(
        (part) =>
          !part ||
          part === "." ||
          part === ".." ||
          /[\\/\u0000-\u001f\u007f]/.test(part) ||
          /%[0-9a-f]{2}/i.test(part),
      )
    ) {
      return null;
    }
    return parts;
  } catch {
    return null;
  }
}

function hasValidSSHPort(value: string): boolean {
  const authorityEnd = value.indexOf("/", "ssh://".length);
  if (authorityEnd < 0) return false;
  const authority = value.slice("ssh://".length, authorityEnd);
  const hostnameAndPort = authority.slice(authority.lastIndexOf("@") + 1);
  const separator = hostnameAndPort.lastIndexOf(":");
  if (separator < 0) return true;
  const port = hostnameAndPort.slice(separator + 1);
  if (!/^\d+$/.test(port)) return false;
  const numericPort = Number(port);
  return numericPort >= 1 && numericPort <= 65535;
}

function parseSSHCloneIdentity(rawRemoteURL: string | undefined): SSHCloneIdentity | null {
  if (!rawRemoteURL || /[\u0000-\u001f\u007f]/.test(rawRemoteURL)) return null;
  const value = cleanValue(rawRemoteURL);
  const scpMatch = /^git@([A-Za-z0-9.-]+):([^?#]+)$/.exec(value);
  if (scpMatch) {
    const parts = decodeSSHPath(scpMatch[2]);
    return parts ? { hostname: scpMatch[1].toLowerCase(), parts } : null;
  }

  try {
    const remote = new URL(value);
    if (
      remote.protocol !== "ssh:" ||
      remote.username !== "git" ||
      remote.password ||
      remote.search ||
      remote.hash ||
      !hasValidSSHPort(value)
    ) {
      return null;
    }
    const parts = decodeSSHPath(remote.pathname);
    return parts ? { hostname: remote.hostname.toLowerCase(), parts } : null;
  } catch {
    return null;
  }
}

function decodedRemoteParts(remote: URL): string[] | null {
  try {
    const path = remote.pathname.replace(/\/+$/, "").replace(/\.git$/, "");
    return path
      .split("/")
      .filter(Boolean)
      .map((part) => decodeURIComponent(part));
  } catch {
    return null;
  }
}

function parseProviderOrigin(rawProviderHost: string | undefined): string | null {
  if (!rawProviderHost || /[\u0000-\u001f\u007f]/.test(rawProviderHost)) return null;
  try {
    const host = new URL(cleanValue(rawProviderHost));
    if (
      host.protocol !== "https:" ||
      host.username ||
      host.password ||
      host.search ||
      host.hash ||
      !/^\/?$/.test(host.pathname)
    ) {
      return null;
    }
    return host.origin;
  } catch {
    return null;
  }
}

function repositoryPathMatches(parts: string[], owner: string, name: string): boolean {
  const expected = owner.split("/").concat(name);
  return (
    expected.every(
      (part) =>
        part &&
        part !== "." &&
        part !== ".." &&
        !/[\\\u0000-\u001f\u007f]/.test(part) &&
        !/%[0-9a-f]{2}/i.test(part),
    ) &&
    parts.length === expected.length &&
    parts.every((part, index) => part === expected[index])
  );
}

function sshRemoteForRepository(repository: ExternalVcsRepository): URL | null {
  const identity = parseSSHCloneIdentity(repository.remote_url);
  if (!identity) return null;
  const provider = repository.provider.toLowerCase();

  if (
    provider === "github" &&
    identity.hostname === "github.com" &&
    repositoryPathMatches(identity.parts, repository.provider_owner, repository.provider_name)
  ) {
    return new URL(
      `https://github.com/${encodeRepositoryPath(repository.provider_owner, repository.provider_name)}`,
    );
  }

  if (provider === "gitlab") {
    const origin = parseProviderOrigin(repository.provider_host);
    if (
      origin &&
      identity.hostname === new URL(origin).hostname.toLowerCase() &&
      repositoryPathMatches(identity.parts, repository.provider_owner, repository.provider_name)
    ) {
      return new URL(
        `${origin}/${encodeRepositoryPath(repository.provider_owner, repository.provider_name)}`,
      );
    }
  }

  if (
    provider === "azure_devops" &&
    identity.hostname === "ssh.dev.azure.com" &&
    identity.parts.length === 4 &&
    identity.parts[0] === "v3" &&
    identity.parts[2] === repository.provider_owner &&
    identity.parts[3] === repository.provider_name
  ) {
    const organization = encodeURIComponent(identity.parts[1]);
    const project = encodeURIComponent(repository.provider_owner);
    const name = encodeURIComponent(repository.provider_name);
    return new URL(`https://dev.azure.com/${organization}/${project}/_git/${name}`);
  }
  return null;
}

function parseRepositoryRemote(repository: ExternalVcsRepository): URL | null {
  return parseHTTPSRemote(repository.remote_url) ?? sshRemoteForRepository(repository);
}

function githubRemoteMatches(
  repository: ExternalVcsRepository,
  remote: URL,
  parts: string[],
): boolean {
  const origin = parseProviderOrigin(repository.provider_host) ?? "https://github.com";
  return (
    remote.origin === origin &&
    origin === "https://github.com" &&
    repositoryPathMatches(parts, repository.provider_owner, repository.provider_name)
  );
}

function gitlabRemoteMatches(
  repository: ExternalVcsRepository,
  remote: URL,
  parts: string[],
): boolean {
  const origin = parseProviderOrigin(repository.provider_host);
  return Boolean(
    origin &&
    remote.origin === origin &&
    repositoryPathMatches(parts, repository.provider_owner, repository.provider_name),
  );
}

function azureRemoteMatches(
  repository: ExternalVcsRepository,
  remote: URL,
  parts: string[],
): boolean {
  return (
    remote.hostname === "dev.azure.com" &&
    parts.length === 4 &&
    parts[1] === repository.provider_owner &&
    parts[2] === "_git" &&
    parts[3] === repository.provider_name
  );
}

function resolveProvider(
  repository: ExternalVcsRepository,
  remote: URL,
): ExternalVcsProvider | null {
  const provider = repository.provider.toLowerCase();
  const parts = decodedRemoteParts(remote);
  if (!parts || !repository.provider_owner || !repository.provider_name) return null;

  if (provider === "github" && githubRemoteMatches(repository, remote, parts)) return "github";
  if (provider === "gitlab" && gitlabRemoteMatches(repository, remote, parts)) return "gitlab";
  if (provider === "azure_devops" && azureRemoteMatches(repository, remote, parts)) {
    return "azure_devops";
  }
  return null;
}

function encodeRepositoryPath(owner: string, name: string): string {
  return owner
    .split("/")
    .concat(name)
    .map((segment) => encodeURIComponent(segment))
    .join("/");
}

function buildFileURL(
  provider: ExternalVcsProvider,
  repository: ExternalVcsRepository,
  remote: URL,
  target: ResolvedTarget,
): string {
  if (provider === "azure_devops") {
    const url = new URL(remote.toString());
    url.pathname = url.pathname.replace(/\/+$/, "").replace(/\.git$/, "");
    url.searchParams.set("path", `/${target.path}`);
    url.searchParams.set("version", `GB${target.revision}`);
    return url.toString();
  }
  const repositoryPath = encodeRepositoryPath(repository.provider_owner, repository.provider_name);
  const route = provider === "gitlab" ? "-/blob" : "blob";
  const filePath = target.path.split("/").map(encodeURIComponent).join("/");
  return `${remote.origin}/${repositoryPath}/${route}/${encodeURIComponent(target.revision)}/${filePath}`;
}

function githubPullHeadRevision(
  provider: ExternalVcsProvider,
  target: ResolvedTarget,
  pullRequestNumber: number | null | undefined,
): string {
  if (
    provider !== "github" ||
    !target.usesPublishedRevision ||
    !Number.isSafeInteger(pullRequestNumber) ||
    !pullRequestNumber ||
    pullRequestNumber < 1
  ) {
    return target.revision;
  }
  return `refs/pull/${pullRequestNumber}/head`;
}

export function resolveExternalVcsFileURL(
  input: ExternalVcsFileURLInput,
): ExternalVcsFileURL | null {
  const remote = parseRepositoryRemote(input.repository);
  const provider = remote ? resolveProvider(input.repository, remote) : null;
  const target = selectTarget(input);
  if (
    !remote ||
    !provider ||
    !target ||
    !isSafeRepositoryPath(target.path) ||
    !isSafeRef(target.revision)
  ) {
    return null;
  }
  const resolvedTarget = {
    ...target,
    revision: githubPullHeadRevision(provider, target, input.publishedPullRequestNumber),
  };
  return {
    provider,
    url: buildFileURL(provider, input.repository, remote, resolvedTarget),
    path: resolvedTarget.path,
    revision: resolvedTarget.revision,
  };
}
