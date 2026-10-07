import type {
  FileChangeFacet,
  FileInfo,
  GitStatusDisplayEntry,
  GitStatusDisplayFile,
  GitStatusDisplayRepresentation,
  GitStatusEntry,
} from "./types";

type DisplayScope = Pick<
  GitStatusDisplayEntry,
  "checkoutGeneration" | "branch" | "headCommit" | "baseCommit" | "comparisonTarget"
>;

function detailState(
  status: GitStatusEntry,
  file: FileInfo,
  facet?: FileChangeFacet,
): "pending" | "ready" | "unavailable" {
  const state = facet?.diff_state ?? file.diff_state ?? status.detail_state;
  if (state === "pending" || state === "unavailable") return state;
  if (state === "ready") return "ready";
  if (status.status_state === "loading") return "pending";
  if (status.status_state === "unavailable") return "unavailable";
  return "ready";
}

function representation(file: FileInfo, facet?: FileChangeFacet): GitStatusDisplayRepresentation {
  return {
    is_symlink: facet?.is_symlink ?? file.is_symlink,
    additions: facet ? facet.additions : file.additions,
    deletions: facet ? facet.deletions : file.deletions,
    old_path: facet ? facet.old_path : file.old_path,
    diff: facet ? facet.diff : file.diff,
    diff_skip_reason: facet ? facet.diff_skip_reason : file.diff_skip_reason,
  };
}

function representationLayer(file: FileInfo): NonNullable<GitStatusDisplayFile["flatLayer"]> {
  if (file.staged_change || file.unstaged_change) return "faceted";
  return file.staged ? "staged" : "unstaged";
}

function sameScope(left: DisplayScope, right: DisplayScope): boolean {
  return (
    left.checkoutGeneration === right.checkoutGeneration &&
    left.branch === right.branch &&
    left.headCommit === right.headCommit &&
    left.baseCommit === right.baseCommit &&
    left.comparisonTarget === right.comparisonTarget
  );
}

function detailsAreIncomplete(status: GitStatusEntry): boolean {
  return (
    status.detail_state === "pending" ||
    status.detail_state === "unavailable" ||
    status.status_state === "loading" ||
    status.status_state === "unavailable"
  );
}

function scopedCommit(
  value: string | null | undefined,
  previous: string | null | undefined,
  preserveEmpty: boolean,
): string | null {
  if (value == null || (preserveEmpty && value === "")) return previous ?? null;
  return value;
}

function nextScope(
  incoming: GitStatusEntry,
  previous: GitStatusDisplayEntry | undefined,
  checkoutGeneration: number,
): DisplayScope {
  const preserveEmpty = detailsAreIncomplete(incoming);
  const branch =
    preserveEmpty && (incoming.branch === null || incoming.branch === "")
      ? (previous?.branch ?? null)
      : incoming.branch;
  return {
    checkoutGeneration,
    branch,
    headCommit: scopedCommit(incoming.head_commit, previous?.headCommit, preserveEmpty),
    baseCommit: scopedCommit(incoming.base_commit, previous?.baseCommit, preserveEmpty),
    comparisonTarget: scopedCommit(
      incoming.comparison_target,
      previous?.comparisonTarget,
      preserveEmpty,
    ),
  };
}

function writeReadyRepresentations(
  next: GitStatusDisplayFile,
  status: GitStatusEntry,
  file: FileInfo,
) {
  if (detailState(status, file) === "ready") {
    const flat = representation(file);
    next.flat = flat;
    next.flatLayer = representationLayer(file);
    if (next.flatLayer !== "faceted") {
      next[next.flatLayer] = flat;
    }
  }
  if (file.staged_change && detailState(status, file, file.staged_change) === "ready") {
    next.staged = representation(file, file.staged_change);
  }
  if (file.unstaged_change && detailState(status, file, file.unstaged_change) === "ready") {
    next.unstaged = representation(file, file.unstaged_change);
  }
}

function pruneRemovedLayers(next: GitStatusDisplayFile, file: FileInfo) {
  const flatLayer = representationLayer(file);
  if (next.flatLayer !== flatLayer) {
    delete next.flat;
    delete next.flatLayer;
  }
  if (file.staged_change || file.unstaged_change) {
    if (!file.staged_change) delete next.staged;
    if (!file.unstaged_change) delete next.unstaged;
    return;
  }
  delete next[file.staged ? "unstaged" : "staged"];
}

/** Retains one accepted ready display representation for each live file/layer. */
export function reconcileGitStatusDisplay(
  previous: GitStatusDisplayEntry | undefined,
  incoming: GitStatusEntry,
  checkoutGeneration: number,
): GitStatusDisplayEntry {
  const scope = nextScope(incoming, previous, checkoutGeneration);
  const compatible = Boolean(previous && sameScope(previous, scope));
  const files = compatible ? { ...previous!.files } : {};
  const completeMembership =
    incoming.files_complete === true &&
    incoming.status_state !== "loading" &&
    incoming.status_state !== "unavailable";
  const incomingFiles = incoming.files ?? {};

  if (completeMembership) {
    for (const path of Object.keys(files)) {
      if (!incomingFiles[path]) delete files[path];
    }
  }

  for (const [path, file] of Object.entries(incomingFiles)) {
    const next = (files[path] ??= {});
    if (completeMembership) pruneRemovedLayers(next, file);
    writeReadyRepresentations(next, incoming, file);
    if (!next.flat && !next.staged && !next.unstaged) delete files[path];
  }

  return { ...scope, files };
}

function scopeMatches(
  status: GitStatusEntry,
  display: GitStatusDisplayEntry,
  checkoutGeneration: number,
): boolean {
  return sameScope(nextScope(status, display, checkoutGeneration), display);
}

function projectRepresentation<T extends FileInfo | FileChangeFacet>(
  current: T,
  cached: GitStatusDisplayRepresentation | undefined,
  currentState: "pending" | "ready" | "unavailable",
  refreshState?: "pending" | "unavailable",
): T {
  const displayState = refreshState ?? currentState;
  let stateful = current;
  if (refreshState !== undefined) {
    stateful = { ...current, diff_state: refreshState };
  } else if (current.diff_state === undefined && currentState !== "ready") {
    stateful = { ...current, diff_state: currentState };
  }
  if (displayState === "ready") return stateful;
  if (!cached) {
    return currentState === "ready" ? { ...stateful, display_stale: true } : stateful;
  }
  return {
    ...stateful,
    diff_state: displayState,
    is_symlink: cached.is_symlink ?? stateful.is_symlink,
    additions: cached.additions ?? stateful.additions,
    deletions: cached.deletions ?? stateful.deletions,
    old_path: cached.old_path ?? stateful.old_path,
    diff: cached.diff ?? stateful.diff,
    diff_skip_reason: cached.diff_skip_reason ?? stateful.diff_skip_reason,
    display_stale: true,
  };
}

function flatCacheForFile(
  file: FileInfo,
  previous: GitStatusDisplayFile | undefined,
): GitStatusDisplayRepresentation | undefined {
  const hasLayerFacets = Boolean(file.staged_change || file.unstaged_change);
  const flatLayer = representationLayer(file);
  const matchingFlat = previous?.flatLayer === flatLayer ? previous.flat : undefined;
  if (hasLayerFacets) return matchingFlat;
  const layerCache = file.staged ? previous?.staged : previous?.unstaged;
  return layerCache ?? matchingFlat;
}

function projectFacet(
  status: GitStatusEntry,
  file: FileInfo,
  facet: FileChangeFacet | undefined,
  cached: GitStatusDisplayRepresentation | undefined,
  refreshState: "pending" | "unavailable" | undefined,
): FileChangeFacet | undefined {
  if (!facet) return undefined;
  return projectRepresentation(facet, cached, detailState(status, file, facet), refreshState);
}

function projectFileForDisplay(
  status: GitStatusEntry,
  path: string,
  file: FileInfo,
  previous: GitStatusDisplayFile | undefined,
  refreshState: "pending" | "unavailable" | undefined,
) {
  const flat = projectRepresentation(
    file,
    flatCacheForFile(file, previous),
    detailState(status, file),
    refreshState,
  );
  const stagedChange = projectFacet(
    status,
    file,
    file.staged_change,
    previous?.staged,
    refreshState,
  );
  const unstagedChange = projectFacet(
    status,
    file,
    file.unstaged_change,
    previous?.unstaged,
    refreshState,
  );
  const changed =
    flat !== file || stagedChange !== file.staged_change || unstagedChange !== file.unstaged_change;
  if (!changed) return [path, file];
  return [
    path,
    {
      ...flat,
      ...(stagedChange ? { staged_change: stagedChange } : {}),
      ...(unstagedChange ? { unstaged_change: unstagedChange } : {}),
    },
  ];
}

/** Adds eligible old display values while leaving readiness and membership current. */
export function projectGitStatusForDisplay(
  status: GitStatusEntry,
  display: GitStatusDisplayEntry | undefined,
  checkoutGeneration: number,
  refreshState?: "pending" | "unavailable",
): GitStatusEntry {
  const hasCompatibleDisplay =
    display !== undefined && scopeMatches(status, display, checkoutGeneration);
  const currentFiles = Object.entries(status.files ?? {});
  const projectedEntries = currentFiles.map(([path, file]) =>
    projectFileForDisplay(
      status,
      path,
      file,
      hasCompatibleDisplay ? display?.files[path] : undefined,
      refreshState,
    ),
  );
  const changed = projectedEntries.some((entry, index) => entry[1] !== currentFiles[index][1]);
  return changed ? { ...status, files: Object.fromEntries(projectedEntries) } : status;
}
