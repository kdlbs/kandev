import { djb2Hash } from "@/lib/utils/hash";
import { t } from "@/lib/i18n";
import type { FileChangeStatus } from "@/lib/utils/file-change-status";
import type { GitChangeLayer } from "@/lib/state/slices/session-runtime/types";

export type ReviewChangeFacet = {
  diff: string;
  status: FileChangeStatus;
  additions: number;
  deletions: number;
  old_path?: string;
  diff_skip_reason?: "too_large" | "binary" | "truncated" | "budget_exceeded";
};

export type ReviewFile = {
  path: string;
  diff: string;
  status: FileChangeStatus;
  additions: number;
  deletions: number;
  staged: boolean;
  source: "uncommitted" | "committed" | "pr";
  old_path?: string;
  diff_skip_reason?: "too_large" | "binary" | "truncated" | "budget_exceeded";
  staged_change?: ReviewChangeFacet;
  unstaged_change?: ReviewChangeFacet;
  /** Frontend-only layer selected from a mixed uncommitted file. */
  change_layer?: GitChangeLayer;
  /**
   * Repository this file belongs to. Set on multi-repo task changes so the
   * file tree groups files under per-repo top-level nodes. Optional for
   * single-repo tasks; the tree falls back to flat path-only grouping.
   */
  repository_id?: string;
  /** Human-readable repo name for tree node labels (e.g. "frontend"). */
  repository_name?: string;
  /** Exact git ref used as the old side when this committed patch was built. */
  base_ref?: string;
  /** True when this file belongs to an initialized Git submodule scope. */
  is_submodule?: boolean;
};

/**
 * Composite per-file key used by the review dialog's in-memory state
 * (reviewed set, stale set, file refs, selected file, comment counts) and
 * by the persisted `useSessionFileReviews` rows. The NUL separator is
 * impossible to embed in a real path or repository name, so the key is
 * always uniquely splittable. Single-repo files (no `repository_name`)
 * keep the legacy bare-path key for backwards compatibility with existing
 * `session_file_reviews` rows. An explicit empty name represents the real
 * workspace root in a multi-repo payload and therefore keeps the separator.
 * Layer-qualified mixed changes append a double-NUL marker plus the layer;
 * the doubled marker cannot collide with a legacy repository/path key.
 */
const FILE_KEY_SEP = "\u0000";
const FILE_LAYER_KEY_SEP = `${FILE_KEY_SEP}${FILE_KEY_SEP}`;

export function reviewFileKey(file: {
  path: string;
  repository_name?: string;
  change_layer?: GitChangeLayer;
}): string {
  const baseKey =
    file.repository_name === undefined
      ? file.path
      : `${file.repository_name}${FILE_KEY_SEP}${file.path}`;
  return file.change_layer ? `${baseKey}${FILE_LAYER_KEY_SEP}${file.change_layer}` : baseKey;
}

/** Mirrors backend `worktree.SanitizeRepoDirName`, which defines the
 * repository_name stamped on multi-repo agentctl events. */
export function sanitizeReviewRepositoryName(name: string): string {
  let sanitized = "";
  for (const character of name) {
    const code = character.charCodeAt(0);
    const isASCIIAlphaNumeric =
      (code >= 48 && code <= 57) || (code >= 65 && code <= 90) || (code >= 97 && code <= 122);
    sanitized +=
      isASCIIAlphaNumeric || character === "_" || character === "." || character === "-"
        ? character
        : "-";
  }
  return sanitized.replace(/-+/g, "-").replace(/^[-.]+|[-.]+$/g, "");
}

export function resolvePRReviewRepositoryName(
  pr: { repository_id?: string; repo: string } | null | undefined,
  workspaceRepositoryName?: string | null,
): string | undefined {
  if (!pr) return undefined;
  if (pr.repository_id && workspaceRepositoryName) {
    const canonicalName = sanitizeReviewRepositoryName(workspaceRepositoryName);
    if (canonicalName) return canonicalName;
  }
  return pr.repo || undefined;
}

export function isReviewMultiRepo(
  taskRepositoryCount: number,
  repositoryNames: Iterable<string>,
): boolean {
  if (taskRepositoryCount > 1) return true;
  const namedRepositories = new Set<string>();
  let hasWorkspaceRoot = false;
  for (const name of repositoryNames) {
    if (!name) hasWorkspaceRoot = true;
    if (name) namedRepositories.add(name);
    if (namedRepositories.size > 1) return true;
  }
  return hasWorkspaceRoot && namedRepositories.size > 0;
}

/**
 * A parent repository reports a changed submodule as a gitlink file at the
 * child's workspace-relative scope. Once the child contributes any file,
 * that synthetic parent row is redundant; when it contributes nothing, the
 * row is the only available evidence and must remain visible.
 */
export function suppressAvailableGitlinkFiles(files: ReviewFile[]): ReviewFile[] {
  const childScopesWithFiles = new Set<string>();
  for (const file of files) {
    if (file.repository_name) childScopesWithFiles.add(file.repository_name);
  }
  if (childScopesWithFiles.size === 0) return files;
  const gitlinkKeys = new Set<string>();
  for (const childScope of childScopesWithFiles) {
    const boundaries = [
      -1,
      ...Array.from(childScope.matchAll(/\//g), (match) => match.index ?? -1),
    ];
    for (const boundary of boundaries) {
      const parentScope = boundary < 0 ? "" : childScope.slice(0, boundary);
      const childPath = childScope.slice(parentScope.length + (parentScope ? 1 : 0));
      gitlinkKeys.add(reviewFileKey({ path: childPath, repository_name: parentScope }));
    }
  }
  return files.filter((file) => !gitlinkKeys.has(reviewFileKey(file)));
}

export function getCumulativeReviewRepositoryNames(
  files: Record<string, { repository_name?: string }> | null | undefined,
): string[] {
  const names = new Set<string>();
  for (const file of Object.values(files ?? {})) {
    if (file.repository_name) names.add(file.repository_name);
  }
  return Array.from(names);
}

export function splitReviewFileKey(key: string): {
  repositoryName: string;
  path: string;
  changeLayer?: GitChangeLayer;
} {
  const layerSep = key.lastIndexOf(FILE_LAYER_KEY_SEP);
  const layer = layerSep < 0 ? undefined : key.slice(layerSep + FILE_LAYER_KEY_SEP.length);
  const changeLayer = layer === "staged" || layer === "unstaged" ? layer : undefined;
  const baseKey = changeLayer ? key.slice(0, layerSep) : key;
  const sep = baseKey.indexOf(FILE_KEY_SEP);
  const identity =
    sep < 0
      ? { repositoryName: "", path: baseKey }
      : { repositoryName: baseKey.slice(0, sep), path: baseKey.slice(sep + 1) };
  return changeLayer ? { ...identity, changeLayer } : identity;
}

export function diffSkipReasonLabel(reason?: string): string {
  switch (reason) {
    case "too_large":
      return t("review:diffSkipTooLarge");
    case "binary":
      return t("review:diffSkipBinary");
    case "truncated":
      return t("review:diffSkipTruncated");
    case "budget_exceeded":
      return t("review:diffSkipBudgetExceeded");
    default:
      return t("review:diffLoading");
  }
}

const TEXTUAL_HUNK_HEADER = /^@@+ (.+?) @@+/;
const HUNK_RANGE = /[+-]\d+(?:,(\d+))?/g;

function hunkHeaderHasLines(line: string): boolean {
  const header = line.match(TEXTUAL_HUNK_HEADER);
  if (!header) return false;
  return [...header[1].matchAll(HUNK_RANGE)].some((range) => Number(range[1] ?? "1") > 0);
}

/** True when a file has a real textual delta, excluding git's synthetic rename/empty hunks. */
export function hasTextualDiff(
  file: Pick<ReviewFile, "diff" | "status" | "additions" | "deletions">,
): boolean {
  // GitHub sends 0+0 stats for pure renames even when the diff contains a
  // synthetic new-file hunk. Trust the stats because that hunk represents the
  // same file at a new path with no textual change.
  if (file.status === "renamed" && file.additions === 0 && file.deletions === 0) return false;
  return file.diff.split("\n").some(hunkHeaderHasLines);
}

export function reviewDiffUnavailableLabel(file: ReviewFile): string {
  if (file.diff_skip_reason) return diffSkipReasonLabel(file.diff_skip_reason);
  switch (file.status) {
    case "added":
      return t("review:noTextualDiffAdded");
    case "untracked":
      return t("review:noTextualDiffUntracked");
    case "deleted":
      return t("review:noTextualDiffDeleted");
    case "renamed":
      return file.old_path
        ? t("review:noTextualDiffMovedFrom", { oldPath: file.old_path })
        : t("review:noTextualDiffMoved");
    case "modified":
      return t("review:noTextualDiffModified");
    default: {
      const exhaustiveStatus: never = file.status;
      return exhaustiveStatus;
    }
  }
}

export type FileTreeNode = {
  name: string;
  path: string;
  isDir: boolean;
  children?: FileTreeNode[];
  file?: ReviewFile;
  /**
   * True when this node represents a repository root in a multi-repo file
   * tree. Repo roots are pinned (never collapsed into their first child) so
   * the user always sees the repo grouping at the top level.
   */
  isRepoRoot?: boolean;
  /** Repository id this node represents (only set when isRepoRoot is true). */
  repositoryId?: string;
  /** True when this node is the boundary of an initialized submodule scope. */
  isSubmodule?: boolean;
  /** Full task-workspace-relative repository scope for a submodule boundary. */
  repositoryName?: string;
};

/**
 * Normalize diff content by handling edge cases from the backend.
 * Handles concatenated diffs with "--- Staged changes ---" separator.
 */
export function normalizeDiffContent(diffContent: string): string {
  if (!diffContent || typeof diffContent !== "string") return "";
  let trimmed = diffContent.trim();
  if (!trimmed) return "";

  const stagedSeparator = "--- Staged changes ---";
  if (trimmed.includes(stagedSeparator)) {
    const parts = trimmed.split(stagedSeparator);
    trimmed = (parts[1] || parts[0]).trim();
  }

  return trimmed;
}

/**
 * Hash diff content for change detection.
 * Used to detect if a diff has changed since the user last reviewed it.
 */
export const hashDiff = djb2Hash;

/**
 * Build a hierarchical file tree from flat file paths.
 * Collapses single-child directories (e.g., `src/components` as one node if `src` has no other children).
 *
 * Multi-repo: when files carry repository_name and 2+ distinct repos are
 * present, the tree gets a top-level repo node per repository so the user
 * sees per-repo grouping. Repo roots are pinned (never collapsed) and the
 * file paths inside them stay relative to the repo root.
 */
export function buildFileTree(files: ReviewFile[]): FileTreeNode[] {
  if (files.some((file) => Boolean(file.repository_name))) return buildScopedTree(files);
  return buildFlatTree(files);
}

function buildScopedTree(files: ReviewFile[]): FileTreeNode[] {
  const scopeNames = new Set(
    files.map((file) => file.repository_name).filter((name): name is string => Boolean(name)),
  );
  const hasWorkspaceRootFiles = files.some((file) => !file.repository_name);
  const topLevelRepoRoots = new Set<string>();
  if (!hasWorkspaceRootFiles && scopeNames.size > 1) {
    for (const scope of scopeNames) topLevelRepoRoots.add(scope.split("/")[0]);
  }

  const repositoryIds = new Map<string, string>();
  for (const file of files) {
    if (file.repository_name && file.repository_id) {
      repositoryIds.set(file.repository_name, file.repository_id);
    }
  }

  const root: FileTreeNode = { name: "", path: "", isDir: true, children: [] };
  const context: ScopedTreeContext = {
    scopeNames,
    topLevelRepoRoots,
    repositoryIds,
  };

  for (const file of files) {
    appendScopedFile(root, file, context);
  }

  return collapseScopedTree(root);
}

type ScopedTreeContext = {
  scopeNames: Set<string>;
  topLevelRepoRoots: Set<string>;
  repositoryIds: Map<string, string>;
};

function appendScopedFile(root: FileTreeNode, file: ReviewFile, context: ScopedTreeContext) {
  const { scopeNames, topLevelRepoRoots, repositoryIds } = context;
  const scope = file.repository_name ?? "";
  const scopeParts = scope ? scope.split("/") : [];
  const parts = [...scopeParts, ...file.path.split("/")];
  let current = root;

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    const logicalPath = parts.slice(0, i + 1).join("/");
    if (i === parts.length - 1) {
      current.children!.push({ name: part, path: logicalPath, isDir: false, file });
      continue;
    }

    const isScopeBoundary = Boolean(scope) && i + 1 === scopeParts.length;
    const isTopLevelScope = i === 0 && topLevelRepoRoots.has(part);
    const child = getOrAppendScopedDirectorySegment(current, part, logicalPath);
    if (isScopeBoundary && scopeNames.has(scope)) {
      child.isSubmodule = !isTopLevelScope;
      child.repositoryName = scope;
    }
    if (isTopLevelScope) {
      child.isRepoRoot = true;
      child.repositoryName = isScopeBoundary && scopeNames.has(scope) ? scope : part;
      child.repositoryId = repositoryIds.get(part);
    }
    current = child;
  }
}

function getOrAppendScopedDirectorySegment(
  parent: FileTreeNode,
  name: string,
  logicalPath: string,
): FileTreeNode {
  const existing = parent.children!.find(
    (child) => child.isDir && child.name === name && child.path === logicalPath,
  );
  if (existing) return existing;

  const child: FileTreeNode = {
    name,
    path: logicalPath,
    isDir: true,
    children: [],
  };
  parent.children!.push(child);
  return child;
}

function collapseScopedTree(root: FileTreeNode): FileTreeNode[] {
  function collapse(node: FileTreeNode): FileTreeNode {
    if (!node.isDir || !node.children) return node;

    node.children = node.children.map(collapse);

    if (
      node.children.length === 1 &&
      node.children[0].isDir &&
      node.name !== "" &&
      !node.isRepoRoot &&
      !node.isSubmodule &&
      !node.children[0].isSubmodule
    ) {
      const child = node.children[0];
      return {
        ...child,
        name: `${node.name}/${child.name}`,
      };
    }

    return node;
  }

  return collapse(root).children ?? [];
}

function getOrAppendDirectorySegment(
  parent: FileTreeNode,
  name: string,
  logicalPath: string,
  segmentCounts: Map<string, number>,
): FileTreeNode {
  const previousChild = parent.children!.at(-1);
  if (previousChild?.isDir && previousChild.name === name) return previousChild;

  const segment = segmentCounts.get(logicalPath) ?? 0;
  segmentCounts.set(logicalPath, segment + 1);
  const child: FileTreeNode = {
    name,
    path: segment === 0 ? logicalPath : `${logicalPath}${FILE_KEY_SEP}segment:${segment}`,
    isDir: true,
    children: [],
  };
  parent.children!.push(child);
  return child;
}

function buildFlatTree(files: ReviewFile[]): FileTreeNode[] {
  const root: FileTreeNode = { name: "", path: "", isDir: true, children: [] };
  const directorySegmentCounts = new Map<string, number>();

  for (const file of files) {
    const parts = file.path.split("/");
    let current = root;

    for (let i = 0; i < parts.length; i++) {
      const part = parts[i];
      const isLast = i === parts.length - 1;
      const partPath = parts.slice(0, i + 1).join("/");

      if (isLast) {
        current.children!.push({
          name: part,
          path: file.path,
          isDir: false,
          file,
        });
      } else {
        current = getOrAppendDirectorySegment(current, part, partPath, directorySegmentCounts);
      }
    }
  }

  // Collapse single-child directories. Repo roots are never collapsed —
  // they encode user-meaningful grouping that survives even when the repo
  // has only one changed file.
  function collapse(node: FileTreeNode): FileTreeNode {
    if (!node.isDir || !node.children) return node;

    node.children = node.children.map(collapse);

    if (
      node.children.length === 1 &&
      node.children[0].isDir &&
      node.name !== "" &&
      !node.isRepoRoot
    ) {
      const child = node.children[0];
      return {
        ...child,
        name: `${node.name}/${child.name}`,
      };
    }

    return node;
  }

  const collapsed = collapse(root);

  // Directory nodes represent contiguous input segments. If a directory
  // reappears after another root entry, its unique segment node lets preorder
  // traversal preserve the caller's exact file order.
  return collapsed.children ?? [];
}
