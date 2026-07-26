import type { ExecutorType } from "@/lib/types/http";

type SavedRepositoryIdentity = {
  remote_url?: string;
  provider?: string;
  provider_repo_id?: string;
  provider_host?: string;
  provider_owner?: string;
  provider_name?: string;
};

export function getWorkspaceSourceCapabilities(
  executorType: ExecutorType | string | null | undefined,
): {
  canAddFolders: boolean;
  canChooseCheckoutBranch: boolean;
  requiresCloneableLocalRepository: boolean;
} {
  const usesLiveCheckout = executorType === "local" || executorType === "local_pc";
  const requiresClone =
    executorType === "local_docker" ||
    executorType === "remote_docker" ||
    executorType === "ssh" ||
    executorType === "sprites";
  return {
    canAddFolders: usesLiveCheckout || executorType === "worktree",
    canChooseCheckoutBranch: !usesLiveCheckout,
    requiresCloneableLocalRepository: requiresClone,
  };
}

/** Mirrors the backend remoteRepositoryLocator eligibility check. */
export function hasCloneableSavedRepository(repository: SavedRepositoryIdentity): boolean {
  return hasSafeRemoteURL(repository.remote_url) || hasProviderRepositoryLocator(repository);
}

function hasProviderRepositoryLocator(repository: SavedRepositoryIdentity): boolean {
  const provider = repository.provider?.trim().toLowerCase();
  if (!provider || !repository.provider_owner?.trim() || !repository.provider_name?.trim()) {
    return false;
  }
  if (provider === "gitlab" && !repository.provider_host?.trim()) return false;
  return provider === "github" || provider === "gitlab" || provider === "bitbucket";
}

function hasSafeRemoteURL(remoteURL: string | undefined): boolean {
  const locator = remoteURL?.trim() ?? "";
  if (!locator || locator.startsWith("/") || locator.startsWith("file:")) return false;
  if (locator.startsWith("git@")) return true;
  try {
    const parsed = new URL(locator);
    return Boolean(parsed.host) && ["https:", "http:", "ssh:", "git:"].includes(parsed.protocol);
  } catch {
    return false;
  }
}

export function canChooseCheckoutBranchForSource(
  sourceType: "saved_repository" | "local_repository" | "remote_repository" | "folder",
  executorType: ExecutorType | string | null | undefined,
): boolean {
  return (
    sourceType !== "folder" && getWorkspaceSourceCapabilities(executorType).canChooseCheckoutBranch
  );
}
