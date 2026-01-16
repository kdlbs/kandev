import { clsx, type ClassValue } from "clsx"
import { twMerge } from "tailwind-merge"

export function cn(...inputs: ClassValue[]) {
    return twMerge(clsx(inputs))
}

/**
 * Generate a UUID. Falls back to a custom implementation in non-secure contexts
 * (e.g., HTTP on non-localhost where crypto.randomUUID is unavailable).
 */
export function generateUUID(): string {
    if (typeof crypto !== 'undefined' && crypto.randomUUID) {
        return crypto.randomUUID();
    }
    // Fallback implementation
    return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
        const r = (Math.random() * 16) | 0;
        const v = c === 'x' ? r : (r & 0x3) | 0x8;
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
        const normalized = repositoryUrl.replace(/\.git$/, '');
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
        repositoryUrl.startsWith('/') ||
        repositoryUrl.startsWith('~/') ||
        /^[A-Za-z]:[\\/]/.test(repositoryUrl)
    );
}

export function getRepositoryDisplayName(repositoryUrl: string | null | undefined): string | null {
    if (!repositoryUrl) return null;
    if (isLocalRepositoryPath(repositoryUrl)) {
        return formatUserHomePath(repositoryUrl.replace(/\.git$/, ''));
    }
    return extractRepoName(repositoryUrl) ?? repositoryUrl;
}

/**
 * Normalize a local path for display, replacing the user home directory with "~".
 * Supports macOS/Linux (/Users/name, /home/name) and Windows (C:\Users\name).
 */
export function formatUserHomePath(path: string): string {
    if (!path) return path;
    const normalized = path.replace(/\\/g, '/');
    const macMatch = normalized.match(/^\/Users\/[^/]+(\/.*)?$/);
    if (macMatch) {
        return `~${macMatch[1] ?? ''}`;
    }
    const linuxMatch = normalized.match(/^\/home\/[^/]+(\/.*)?$/);
    if (linuxMatch) {
        return `~${linuxMatch[1] ?? ''}`;
    }
    const windowsMatch = normalized.match(/^[A-Za-z]:\/Users\/[^/]+(\/.*)?$/);
    if (windowsMatch) {
        return `~${windowsMatch[1] ?? ''}`;
    }
    return path;
}

/**
 * Truncate a repository path for display, favoring the last segments.
 */
export function truncateRepoPath(path: string, maxLength = 34): string {
    const displayPath = formatUserHomePath(path);
    if (displayPath.length <= maxLength) return displayPath;
    const normalizedPath = displayPath.replace(/\\/g, '/');
    const hasHomePrefix = normalizedPath.startsWith('~/');
    const hasRootPrefix = normalizedPath.startsWith('/');
    const prefix = hasHomePrefix ? '~/' : hasRootPrefix ? '/' : '';
    const parts = normalizedPath.replace(/^~\//, '').replace(/^\//, '').split('/').filter(Boolean);
    if (parts.length === 0) return displayPath;
    const lastThree = parts.slice(-3).join('/');
    let result = `${prefix}.../${lastThree}`;
    if (result.length <= maxLength) return result;
    const lastTwo = parts.slice(-2).join('/');
    result = `${prefix}.../${lastTwo}`;
    if (result.length <= maxLength) return result;
    const lastOne = parts.slice(-1).join('/');
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

export function selectPreferredBranch(
    branches: BranchSelectionCandidate[]
): string | null {
    const originMain = branches.find(
        (branch) => branch.type === 'remote' && branch.remote === 'origin' && branch.name === 'main'
    );
    if (originMain) return 'origin/main';

    const originMaster = branches.find(
        (branch) => branch.type === 'remote' && branch.remote === 'origin' && branch.name === 'master'
    );
    if (originMaster) return 'origin/master';

    const localMain = branches.find((branch) => branch.type === 'local' && branch.name === 'main');
    if (localMain) return 'main';

    const localMaster = branches.find((branch) => branch.type === 'local' && branch.name === 'master');
    if (localMaster) return 'master';

    return null;
}

export const DEFAULT_LOCAL_ENVIRONMENT_KIND = 'local_pc';
export const DEFAULT_LOCAL_EXECUTOR_TYPE = 'local_pc';
