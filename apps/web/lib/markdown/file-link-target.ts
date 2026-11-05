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
  /**
   * Confirmed native-session working directory. Relative links resolve from
   * this checkout, then convert to a Files-root-relative target. It can differ
   * from workspaceRoot after live attachment promotes the task root.
   */
  relativeBasePath?: string | null;
  /**
   * Files-root-relative checkout directories. They preserve explicit source
   * prefixes even when the repository has no host source path (remote
   * executors) or the workspace repository list has not refreshed yet.
   */
  sourcePrefixes?: readonly string[];
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

export function isBlockedMarkdownImageSource(href?: string): boolean {
  if (!href || isExternalHref(href)) return false;
  const decodedPath = decodeHrefPath(href);
  return decodedPath === null || isInvalidFilePath(normalizePath(decodedPath));
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
    /^\/(?:Users|home|root|tmp|private|var|etc|usr|opt|mnt|srv|Volumes)\//i.test(path)
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
    return { kind: "file", path: normalizedPath };
  }
  if (looksLikeHostAbsolutePath(normalizedPath)) return { kind: "file", path: normalizedPath };

  return createFileTarget(normalizedPath.replace(/^\/+/, ""));
}

/** Builds explicit source prefixes from exact session worktree placement. */
export function buildMarkdownWorkspaceSourcePrefixes({
  workspaceRoot,
  sessionWorktrees = [],
}: {
  workspaceRoot?: string | null;
  sessionWorktrees?: readonly MarkdownAliasWorktree[];
}): string[] {
  const root = normalizeOptionalPath(workspaceRoot);
  if (!root) return [];
  return uniqueNormalizedPaths(
    sessionWorktrees.map((worktree) => {
      const worktreePath = normalizeOptionalPath(worktree.path);
      if (!worktreePath) return null;
      return relativePathFromRoot(worktreePath, root);
    }),
  );
}

/**
 * Preserves a link that already names one of the current source prefixes,
 * such as `master/docs/guide.md` after a root promotion. Prefixes come from
 * exact session worktree placement, so nested and remote layouts stay valid
 * without probing a repository for a matching filename.
 */
function matchesSourcePrefix(path: string, prefix: string, caseInsensitive: boolean): boolean {
  const comparablePath = caseInsensitive ? path.toLowerCase() : path;
  const comparablePrefix = caseInsensitive ? prefix.toLowerCase() : prefix;
  return comparablePath === comparablePrefix || comparablePath.startsWith(`${comparablePrefix}/`);
}

function resolveExplicitSourcePrefixTarget(
  path: string,
  aliases: readonly MarkdownFileRootAlias[],
  sourcePrefixes: readonly string[],
  caseInsensitive: boolean,
): MarkdownFileTarget | null {
  const matches = aliases.flatMap((alias) => {
    const relativeRoot = normalizeOptionalPath(alias.workspaceRelativeRoot);
    if (!relativeRoot || !matchesSourcePrefix(path, relativeRoot, caseInsensitive)) return [];
    return [{ alias, relativeRoot }];
  });
  if (matches.length > 0) {
    const longestRootLength = Math.max(...matches.map(({ relativeRoot }) => relativeRoot.length));
    const targetIdentities = new Set(
      matches
        .filter(({ relativeRoot }) => relativeRoot.length === longestRootLength)
        .map(
          ({ alias }) => `${alias.repositoryId}\u0000${normalizePath(alias.workspaceRelativeRoot)}`,
        ),
    );
    if (targetIdentities.size !== 1) return { kind: "blocked" };
  } else {
    const matchesPrefix = sourcePrefixes.some((prefix) =>
      matchesSourcePrefix(path, prefix, caseInsensitive),
    );
    if (!matchesPrefix) return null;
  }
  return createFileTarget(path) ?? { kind: "blocked" };
}

function hasExpandedWorkspaceRoots(
  aliases: readonly MarkdownFileRootAlias[],
  sourcePrefixes: readonly string[],
): boolean {
  return (
    sourcePrefixes.length > 0 ||
    aliases.some((alias) => normalizeOptionalPath(alias.workspaceRelativeRoot) !== null)
  );
}

/**
 * Resolves an unqualified relative href beneath the confirmed native
 * workspace. The result is Files-root-relative, so a link emitted from
 * `<task>/master` still opens `<task>/master/...` after the root moves to
 * `<task>`. Without a confirmed base, the established root-relative behavior
 * survives only for a single known root; an expanded workspace cannot choose
 * a checkout safely.
 */
function resolveRelativeTarget(
  path: string,
  workspaceRoot: string | null,
  relativeBasePath: string | null,
  aliases: readonly MarkdownFileRootAlias[],
  sourcePrefixes: readonly string[],
): MarkdownFileTarget | null {
  const explicitPrefix = resolveExplicitSourcePrefixTarget(
    path,
    aliases,
    sourcePrefixes,
    isWindowsDriveAbsolutePath(workspaceRoot ?? ""),
  );
  if (explicitPrefix) return explicitPrefix;

  if (!relativeBasePath) {
    return hasExpandedWorkspaceRoots(aliases, sourcePrefixes)
      ? { kind: "blocked" }
      : createFileTarget(path);
  }
  if (!workspaceRoot) return createFileTarget(path);

  const baseRelativeToRoot = relativePathFromRoot(relativeBasePath, workspaceRoot);
  if (baseRelativeToRoot === null) return { kind: "blocked" };
  const target = baseRelativeToRoot ? `${baseRelativeToRoot}/${path}` : path;
  return createFileTarget(target) ?? { kind: "blocked" };
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

/** Resolves a file href to a workspace alias or an absolute current-file read target. */
export function resolveMarkdownFileTarget(
  href: string | undefined,
  {
    workspaceRoot,
    relativeBasePath,
    sourcePrefixes = [],
    fileRootAliases = [],
  }: ResolveMarkdownFileTargetOptions = {},
): MarkdownFileTarget | null {
  if (!href || href.startsWith("#")) return null;

  const parsedHref = parseMarkdownFileHref(href);
  if (!parsedHref || parsedHref.kind === "blocked") return parsedHref;
  if (parsedHref.isAbsolute) {
    return resolveAbsoluteTarget(parsedHref.path, workspaceRoot ?? null, fileRootAliases);
  }

  const normalizedPath = parsedHref.path.replace(/^\.\//, "");
  if (normalizedPath.startsWith("../")) return null;
  return resolveRelativeTarget(
    normalizedPath,
    normalizeOptionalPath(workspaceRoot),
    normalizeOptionalPath(relativeBasePath),
    fileRootAliases,
    sourcePrefixes.flatMap((prefix) => {
      const normalized = normalizeOptionalPath(prefix);
      return normalized ? [normalized] : [];
    }),
  );
}
