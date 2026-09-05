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

function normalizePath(path: string): string {
  const normalized = path.replace(/\\/g, "/").replace(/\/+/g, "/");
  if (normalized === "/") return normalized;
  return normalized.replace(/\/$/, "");
}

function normalizeOptionalPath(path: string | null | undefined): string | null {
  if (!path?.trim()) return null;
  const normalized = normalizePath(path.trim());
  return normalized || null;
}

function isPathInside(path: string, root: string): boolean {
  if (root === "/") return path.startsWith("/");
  return path === root || path.startsWith(`${root}/`);
}

function relativePathFromRoot(path: string, root: string): string | null {
  if (!isPathInside(path, root)) return null;
  if (path === root) return "";
  return path.slice(root.length + (root === "/" ? 0 : 1));
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
  if (repositoryIds.size > 0) {
    for (const worktree of sessionWorktrees) {
      if (worktree.repositoryId) repositoryIds.add(worktree.repositoryId);
    }
  }
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

function looksLikeHostAbsolutePath(path: string): boolean {
  return /^\/(?:[A-Za-z]:|Users|home|root|tmp|var|etc|usr|opt|mnt|Volumes)\//i.test(path);
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
  return createFileTarget(target);
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
      return createFileTarget(relativePath);
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

/** Resolves an href to a task-workspace-relative file target or a blocked host path. */
export function resolveMarkdownFileTarget(
  href: string | undefined,
  { workspaceRoot, fileRootAliases = [] }: ResolveMarkdownFileTargetOptions = {},
): MarkdownFileTarget | null {
  if (!href || href.startsWith("#") || isExternalHref(href)) return null;

  const decodedPath = decodeHrefPath(href);
  if (!decodedPath) return href.startsWith("/") ? { kind: "blocked" } : null;
  const path = normalizePath(decodedPath);
  if (!path || path.startsWith("~/") || hasParentTraversal(path)) {
    return path.startsWith("/") ? { kind: "blocked" } : null;
  }

  if (path.startsWith("/")) {
    return resolveAbsoluteTarget(path, workspaceRoot ?? null, fileRootAliases);
  }

  const normalizedPath = path.replace(/^\.\//, "");
  if (normalizedPath.startsWith("../")) return null;
  return createFileTarget(normalizedPath);
}
