export type MarkdownFileRootAlias = {
  repositoryId: string;
  sourceRoot: string;
  workspaceRelativeRoot: string;
};

export type MarkdownAliasRepository = {
  id: string;
  localPath?: string | null;
};

export type MarkdownAliasWorktree = {
  repositoryId?: string | null;
  path?: string | null;
};

export type BuildMarkdownFileRootAliasesInput = {
  workspaceRoot?: string | null;
  taskRepositoryIds?: readonly string[];
  repositories?: readonly MarkdownAliasRepository[];
  sessionWorktrees?: readonly MarkdownAliasWorktree[];
};

export type MarkdownFileTarget = { kind: "file"; path: string } | { kind: "blocked" };

export type ResolveMarkdownFileTargetOptions = {
  workspaceRoot?: string | null;
  fileRootAliases?: readonly MarkdownFileRootAlias[];
};

const WEB_TLD_EXTENSIONS = new Set(["ai", "app", "cloud", "co", "com", "dev", "io", "net", "org"]);

function isWindowsDriveAbsolutePath(path: string): boolean {
  return /^\/?[A-Za-z]:[\\/](?![\\/])/.test(path);
}

function normalizePath(path: string): string {
  let normalized = path.replace(/\\/g, "/").replace(/\/+/g, "/");
  if (/^\/[A-Za-z]:\//.test(normalized)) normalized = normalized.slice(1);
  if (normalized === "/" || /^[A-Za-z]:\/$/.test(normalized)) return normalized;
  return normalized.replace(/\/$/, "");
}

function normalizeOptionalPath(path: string | null | undefined): string | null {
  if (!path?.trim()) return null;
  const normalized = normalizePath(path.trim());
  return normalized || null;
}

function isPathInside(path: string, root: string): boolean {
  const windowsPath = isWindowsDriveAbsolutePath(path);
  const windowsRoot = isWindowsDriveAbsolutePath(root);
  const comparablePath = windowsPath || windowsRoot ? path.toLowerCase() : path;
  const comparableRoot = windowsPath || windowsRoot ? root.toLowerCase() : root;
  if (comparableRoot === "/") return comparablePath.startsWith("/");
  const rootPrefix = comparableRoot.endsWith("/") ? comparableRoot : `${comparableRoot}/`;
  return comparablePath === comparableRoot || comparablePath.startsWith(rootPrefix);
}

function relativePathFromRoot(path: string, root: string): string | null {
  if (!isPathInside(path, root)) return null;
  if (path.length === root.length) return "";
  return path.slice(root.length + (root.endsWith("/") ? 0 : 1));
}

function uniqueNormalizedPaths(paths: readonly (string | null | undefined)[]): string[] {
  return [
    ...new Set(
      paths.flatMap((path) => {
        const normalized = normalizeOptionalPath(path);
        return normalized ? [normalized] : [];
      }),
    ),
  ];
}

/**
 * Builds identity-qualified aliases from task-linked repositories and the
 * worktrees materialized for the rendered session.
 */
export function buildMarkdownFileRootAliases({
  workspaceRoot,
  taskRepositoryIds = [],
  repositories = [],
  sessionWorktrees = [],
}: BuildMarkdownFileRootAliasesInput): MarkdownFileRootAlias[] {
  const normalizedWorkspaceRoot = normalizeOptionalPath(workspaceRoot);
  if (!normalizedWorkspaceRoot) return [];

  const aliases: MarkdownFileRootAlias[] = [];
  const repositoryIds = new Set(taskRepositoryIds.filter(Boolean));
  for (const repositoryId of repositoryIds) {
    const sourceRoots = uniqueNormalizedPaths(
      repositories
        .filter((repository) => repository.id === repositoryId)
        .map((repository) => repository.localPath),
    );
    const worktreePaths = uniqueNormalizedPaths(
      sessionWorktrees
        .filter((worktree) => worktree.repositoryId === repositoryId)
        .map((worktree) => worktree.path),
    );
    if (sourceRoots.length !== 1 || worktreePaths.length !== 1) continue;

    const workspaceRelativeRoot = relativePathFromRoot(worktreePaths[0], normalizedWorkspaceRoot);
    if (workspaceRelativeRoot === null) continue;

    aliases.push({
      repositoryId,
      sourceRoot: sourceRoots[0],
      workspaceRelativeRoot,
    });
  }
  return aliases;
}

function looksLikeFilePath(path: string): boolean {
  const lastSegment = path.split("/").pop() ?? "";
  if (!lastSegment.includes(".") || path.endsWith("/")) return false;
  const extension = lastSegment.split(".").pop() ?? "";
  if (!/^[a-z0-9]{1,8}$/i.test(extension)) return false;
  return !WEB_TLD_EXTENSIONS.has(extension.toLowerCase());
}

function isExternalHref(href: string): boolean {
  return /^[a-z][a-z\d+.-]*:/i.test(href) || href.startsWith("//");
}

function isAbsoluteFileSystemPath(path: string): boolean {
  return path.startsWith("/") || isWindowsDriveAbsolutePath(path);
}

function stripHashAndQuery(href: string): string {
  return href.split(/[?#]/, 1)[0] ?? "";
}

function stripSourceLocationSuffix(path: string): string {
  const part = String.raw`\d+(?:[-+]\d+)?(?:,\d+(?:[-+]\d+)?)*|raw|conflicts`;
  return path.replace(new RegExp(`:(?:${part})(?::(?:${part}))*$`), "");
}

function decodeHrefPath(href: string): string | null {
  try {
    return stripSourceLocationSuffix(decodeURIComponent(stripHashAndQuery(href)));
  } catch {
    return null;
  }
}

function hasParentTraversal(path: string): boolean {
  return path.split("/").includes("..");
}

function isInvalidFilePath(path: string): boolean {
  return !path || path.startsWith("~/") || hasParentTraversal(path);
}

function blockedAbsolutePath(path: string): { kind: "blocked" } | null {
  return isAbsoluteFileSystemPath(path) ? { kind: "blocked" } : null;
}

function looksLikeHostAbsolutePath(path: string): boolean {
  return (
    isWindowsDriveAbsolutePath(path) ||
    /^\/(?:Users|home|root|tmp|var|etc|usr|opt|mnt|Volumes)\//i.test(path)
  );
}

function firstAbsoluteSegment(path: string): string | null {
  const first = path.replace(/^\/+/, "").split("/")[0];
  return first || null;
}

function createFileTarget(path: string): MarkdownFileTarget | null {
  return looksLikeFilePath(path) ? { kind: "file", path } : null;
}

function resolveAliasTarget(
  path: string,
  aliases: readonly MarkdownFileRootAlias[],
): MarkdownFileTarget | null {
  const matches = aliases.flatMap((alias) => {
    const sourceRoot = normalizeOptionalPath(alias.sourceRoot);
    if (!sourceRoot) return [];
    const suffix = relativePathFromRoot(path, sourceRoot);
    return suffix === null ? [] : [{ alias, sourceRoot, suffix }];
  });
  if (matches.length === 0) return null;

  const longestRootLength = Math.max(...matches.map(({ sourceRoot }) => sourceRoot.length));
  const mostSpecific = matches.filter(({ sourceRoot }) => sourceRoot.length === longestRootLength);
  const targetIdentities = new Set(
    mostSpecific.map(
      ({ alias }) => `${alias.repositoryId}\u0000${normalizePath(alias.workspaceRelativeRoot)}`,
    ),
  );
  if (targetIdentities.size !== 1) return { kind: "blocked" };

  const targetRoot = mostSpecific[0].alias.workspaceRelativeRoot
    .replace(/\\/g, "/")
    .replace(/^\/+|\/+$/g, "");
  const suffix = mostSpecific[0].suffix;
  const target = targetRoot && suffix ? `${targetRoot}/${suffix}` : targetRoot || suffix;
  return createFileTarget(target) ?? { kind: "blocked" };
}

function resolveAbsoluteTarget(
  path: string,
  workspaceRoot: string | null,
  aliases: readonly MarkdownFileRootAlias[],
): MarkdownFileTarget | null {
  const normalizedPath = normalizePath(path);
  const normalizedWorkspaceRoot = normalizeOptionalPath(workspaceRoot);

  if (normalizedWorkspaceRoot) {
    const relativePath = relativePathFromRoot(normalizedPath, normalizedWorkspaceRoot);
    if (relativePath !== null) {
      return createFileTarget(relativePath) ?? { kind: "blocked" };
    }
  }

  const aliasTarget = resolveAliasTarget(normalizedPath, aliases);
  if (aliasTarget) return aliasTarget;

  if (
    normalizedWorkspaceRoot &&
    firstAbsoluteSegment(normalizedPath) === firstAbsoluteSegment(normalizedWorkspaceRoot)
  ) {
    return { kind: "blocked" };
  }
  if (looksLikeHostAbsolutePath(normalizedPath)) return { kind: "blocked" };

  return createFileTarget(normalizedPath.replace(/^\/+/, ""));
}

type ParsedMarkdownFileHref =
  | { kind: "path"; path: string; isAbsolute: boolean }
  | { kind: "blocked" }
  | null;

function parseMarkdownFileHref(href: string): ParsedMarkdownFileHref {
  const decodedPath = decodeHrefPath(href);
  if (!decodedPath) return blockedAbsolutePath(href);

  const isWindowsPath = isWindowsDriveAbsolutePath(decodedPath);
  if (!isWindowsPath && isExternalHref(href)) return null;

  const path = normalizePath(decodedPath);
  if (isInvalidFilePath(path)) return blockedAbsolutePath(path);

  return {
    kind: "path",
    path,
    isAbsolute: isAbsoluteFileSystemPath(path),
  };
}

/** Resolves an href to a task-workspace-relative file target or a blocked host path. */
export function resolveMarkdownFileTarget(
  href: string | undefined,
  { workspaceRoot, fileRootAliases = [] }: ResolveMarkdownFileTargetOptions = {},
): MarkdownFileTarget | null {
  if (!href || href.startsWith("#")) return null;

  const parsedHref = parseMarkdownFileHref(href);
  if (!parsedHref || parsedHref.kind === "blocked") return parsedHref;
  if (parsedHref.isAbsolute) {
    return resolveAbsoluteTarget(parsedHref.path, workspaceRoot ?? null, fileRootAliases);
  }

  const normalizedPath = parsedHref.path.replace(/^\.\//, "");
  if (normalizedPath.startsWith("../")) return null;
  return createFileTarget(normalizedPath);
}
