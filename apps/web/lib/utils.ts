import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * Format a cost stored in subcents (hundredths of a cent) as a USD
 * dollar string. The backend persists every cost figure as int64
 * subcents so token-rate math stays integer-only;
 * docs/specs/office-costs/spec.md. UI consumers should never multiply
 * or divide the raw value themselves — call this helper so the unit
 * boundary lives in one place.
 */
export function formatDollars(subcents: number | null | undefined): string {
  if (subcents == null || !Number.isFinite(subcents)) {
    return "$0.00";
  }
  return `$${(subcents / 10000).toFixed(2)}`;
}

let uuidFallbackCounter = 0;
let uuidFallbackContextCounter = 0;

function createUuidFallbackContext(): string {
  const entropy = new Uint32Array(2);
  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    try {
      crypto.getRandomValues(entropy);
      return entropy[0].toString(16).padStart(8, "0");
    } catch {
      // Fall through to the non-cryptographic legacy-environment fallback.
    }
  }

  const monotonic = typeof performance !== "undefined" ? Math.floor(performance.now() * 1000) : 0;
  const counter = uuidFallbackContextCounter++;
  return ((Date.now() ^ monotonic ^ counter) >>> 0).toString(16).padStart(8, "0");
}

// Keep a per-module component so two tabs generating IDs in the same
// millisecond do not share the timestamp/counter prefix. Prefer Web Crypto
// entropy when available; old environments get a monotonic best-effort ID.
const uuidFallbackContext = createUuidFallbackContext();

/**
 * Generate a UUID. Falls back to crypto.getRandomValues when randomUUID is
 * unavailable (e.g., HTTP on non-localhost), then to a UUID-shaped fallback
 * with a per-context component for legacy environments without any Web Crypto API.
 */
export function generateUUID(): string {
  if (typeof crypto !== "undefined" && crypto.randomUUID) {
    return crypto.randomUUID();
  }

  if (typeof crypto !== "undefined" && crypto.getRandomValues) {
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    const hex = Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
  }

  const context = uuidFallbackContext;
  const timestamp = Date.now().toString(16).padStart(12, "0").slice(-12);
  const counter = (uuidFallbackCounter++ >>> 0).toString(16).padStart(8, "0");
  const chars = `${context}${timestamp}${counter}`.padEnd(32, "0").slice(0, 32).split("");
  chars[12] = "4";
  chars[16] = "8";
  const hex = chars.join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
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
 * Format a date string with minute-level precision and short context.
 * Examples: "just now", "5m ago", "Today, 14:32", "Yesterday, 14:32",
 * "Apr 25, 14:32", "Apr 25 2025, 14:32".
 */
export function formatPreciseTime(dateString: string): string {
  const date = new Date(dateString);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);

  if (diffSec < 10) return "just now";
  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffMin < 60) return `${diffMin}m ago`;

  const time = date.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate();
  if (sameDay) return `Today, ${time}`;

  const yesterday = new Date(now);
  yesterday.setDate(now.getDate() - 1);
  const isYesterday =
    date.getFullYear() === yesterday.getFullYear() &&
    date.getMonth() === yesterday.getMonth() &&
    date.getDate() === yesterday.getDate();
  if (isYesterday) return `Yesterday, ${time}`;

  const sameYear = date.getFullYear() === now.getFullYear();
  const dateLabel = date.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: sameYear ? undefined : "numeric",
  });
  return `${dateLabel}, ${time}`;
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
