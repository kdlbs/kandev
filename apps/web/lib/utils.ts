import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Generate a UUID. Falls back to a custom implementation in non-secure contexts
 * (e.g., HTTP on non-localhost where crypto.randomUUID is unavailable).
 */
export function generateUUID(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }
  // Fallback implementation
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    const v = c === "x" ? r : (r & 0x3) | 0x8;
    return v.toString(16);
  });
}

/**
 * Extract organization/repository name from a repository URL.
 * Supports common Git providers and SSH/HTTPS formats.
 * @param repositoryUrl - The repository URL or local path
 * @returns The org/repo-name format, or null if unable to extract
 */
export function extractRepoName(repositoryUrl: string | null | undefined): string | null {
  if (!repositoryUrl) return null;

  try {
    const normalized = repositoryUrl.replace(/\.git$/, "");
    // SSH format: git@host:org/repo
    const sshMatch = normalized.match(/^[^@]+@[^:]+:([^/]+\/[^/]+)$/);
    if (sshMatch) {
      return sshMatch[1];
    }
    // HTTPS format: https://host/org/repo or http(s)://host/org/repo
    const httpMatch = normalized.match(/^https?:\/\/[^/]+\/([^/]+\/[^/]+)$/);
    if (httpMatch) {
      return httpMatch[1];
    }

    return null;
  } catch {
    return null;
  }
}

export function isLocalRepositoryPath(repositoryUrl: string): boolean {
  return (
    repositoryUrl.startsWith("/") ||
    repositoryUrl.startsWith("~/") ||
    /^[A-Za-z]:[\\/]/.test(repositoryUrl)
  );
}

export function getRepositoryDisplayName(repositoryUrl: string | null | undefined): string | null {
  if (!repositoryUrl) return null;
  if (isLocalRepositoryPath(repositoryUrl)) {
    return formatUserHomePath(repositoryUrl.replace(/\.git$/, ""));
  }
  return extractRepoName(repositoryUrl) ?? repositoryUrl;
}

/**
 * Normalize a local path for display, replacing the user home directory with "~".
 * Supports macOS/Linux (/Users/name, /home/name) and Windows (C:\Users\name).
 */
export function formatUserHomePath(path: string): string {
  if (!path) return path;
  const normalized = path.replace(/\\/g, "/");
  const macMatch = normalized.match(/^\/Users\/[^/]+(\/.*)?$/);
  if (macMatch) {
    return `~${macMatch[1] ?? ""}`;
  }
  const linuxMatch = normalized.match(/^\/home\/[^/]+(\/.*)?$/);
  if (linuxMatch) {
    return `~${linuxMatch[1] ?? ""}`;
  }
  const windowsMatch = normalized.match(/^[A-Za-z]:\/Users\/[^/]+(\/.*)?$/);
  if (windowsMatch) {
    return `~${windowsMatch[1] ?? ""}`;
  }
  return path;
}

/**
 * Truncate a repository path for display, favoring the last segments.
 */
export function truncateRepoPath(path: string, maxLength = 34): string {
  const displayPath = formatUserHomePath(path);
  if (displayPath.length <= maxLength) return displayPath;
  const normalizedPath = displayPath.replace(/\\/g, "/");
  const hasHomePrefix = normalizedPath.startsWith("~/");
  const hasRootPrefix = normalizedPath.startsWith("/");
  let prefix = "";
  if (hasHomePrefix) {
    prefix = "~/";
  } else if (hasRootPrefix) {
    prefix = "/";
  }
  const parts = normalizedPath.replace(/^~\//, "").replace(/^\//, "").split("/").filter(Boolean);
  if (parts.length === 0) return displayPath;
  const lastThree = parts.slice(-3).join("/");
  let result = `${prefix}.../${lastThree}`;
  if (result.length <= maxLength) return result;
  const lastTwo = parts.slice(-2).join("/");
  result = `${prefix}.../${lastTwo}`;
  if (result.length <= maxLength) return result;
  const lastOne = parts.slice(-1).join("/");
  result = `${prefix}.../${lastOne}`;
  if (result.length <= maxLength) return result;
  const remaining = Math.max(1, maxLength - prefix.length - 4);
  return `${prefix}.../${lastOne.slice(-remaining)}`;
}

type BranchSelectionCandidate = {
  name: string;
  type?: string;
  remote?: string;
};

export function selectPreferredBranch(branches: BranchSelectionCandidate[]): string | null {
  const localMain = branches.find((branch) => branch.type === "local" && branch.name === "main");
  if (localMain) return "main";

  const localMaster = branches.find(
    (branch) => branch.type === "local" && branch.name === "master",
  );
  if (localMaster) return "master";

  const originMain = branches.find(
    (branch) => branch.type === "remote" && branch.remote === "origin" && branch.name === "main",
  );
  if (originMain) return "origin/main";

  const originMaster = branches.find(
    (branch) => branch.type === "remote" && branch.remote === "origin" && branch.name === "master",
  );
  if (originMaster) return "origin/master";

  return null;
}

export const DEFAULT_LOCAL_EXECUTOR_TYPE = "worktree";

/**
 * Format a date string as a human-readable relative time (e.g., "2m ago", "1h ago", "yesterday").
 * @param dateString - ISO date string
 * @returns Formatted relative time string
 */
export function formatRelativeTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 10) return "just now";
  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  if (diffDay === 1) return "yesterday";
  if (diffDay < 7) return `${diffDay}d ago`;
  return date.toLocaleDateString();
}

/**
 * Convert an absolute file path to a relative path based on a workspace root.
 * If the path is within the workspace, returns the relative portion.
 * Otherwise, returns the original path (with home directory formatting applied).
 */
export function toRelativePath(absolutePath: string, workspaceRoot?: string | null): string {
  if (!absolutePath) return absolutePath;
  if (!workspaceRoot) return formatUserHomePath(absolutePath);

  // Normalize paths for comparison
  const normalizedPath = absolutePath.replace(/\\/g, "/");
  const normalizedRoot = workspaceRoot.replace(/\\/g, "/").replace(/\/$/, "");

  // Check if path is within the workspace
  if (normalizedPath.startsWith(normalizedRoot + "/")) {
    // Return the relative portion (strip workspace root)
    return normalizedPath.slice(normalizedRoot.length + 1);
  }

  // Path is outside workspace, use home path formatting
  return formatUserHomePath(absolutePath);
}

/**
 * Transform any absolute paths in a string to relative paths based on workspace root.
 * Useful for titles/descriptions that may contain embedded file paths.
 */
export function transformPathsInText(text: string, workspaceRoot?: string | null): string {
  if (!text || !workspaceRoot) return text;

  const normalizedRoot = workspaceRoot.replace(/\\/g, "/").replace(/\/$/, "");
  // Match the workspace root followed by a path (non-whitespace characters)
  const pattern = new RegExp(
    normalizedRoot.replace(/[.*+?^${}()|[\]\\]/g, "\\$&") + "/([^\\s]+)",
    "g",
  );

  return text.replace(pattern, (_, relativePart) => relativePart);
}
