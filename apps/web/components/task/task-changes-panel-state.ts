"use client";

import { useEffect, useMemo, useRef, type MutableRefObject, type RefObject } from "react";
import { hashDiff, reviewFileKey, type ReviewFile } from "@/components/review/types";
import type { ReviewSource } from "@/hooks/domains/session/use-review-sources";
import type { GitChangeLayer } from "@/lib/state/slices/session-runtime/types";

export function shouldDeferReviewStateForPR(
  hasSelectedPR: boolean,
  prDiffLoading: boolean,
  sourceFilter: "all" | ReviewSource,
): boolean {
  const usesPRDiff = sourceFilter === "all" || sourceFilter === "pr";
  return hasSelectedPR && prDiffLoading && usesPRDiff;
}

export function shouldBlockChangesForPR(
  sourceFilter: "all" | ReviewSource,
  visibleFiles: ReviewFile[],
): boolean {
  if (sourceFilter === "pr") return true;
  if (sourceFilter !== "all") return false;
  return !visibleFiles.some((file) => file.source !== "pr");
}

export function resolveSelectedFileRepositoryName(
  sourceFilter: "all" | ReviewSource,
  prKey: string | undefined,
  fileRepositoryName: string | undefined,
  visibleFileRepositoryName: string | undefined,
): string | undefined {
  return prKey && sourceFilter === "pr" ? visibleFileRepositoryName : fileRepositoryName;
}

type GitMembershipStatus = {
  status_state?: "ready" | "loading" | "unavailable";
  files_complete?: boolean;
  files?: Record<
    string,
    {
      staged?: boolean;
      diff?: string;
      staged_change?: unknown;
      unstaged_change?: unknown;
    }
  >;
};

type CompleteGitMembershipStatus = GitMembershipStatus & {
  files: NonNullable<GitMembershipStatus["files"]>;
};

type RepositoryGitMembership = { repository_name: string; status: GitMembershipStatus };

function isCompleteMembership(
  status: GitMembershipStatus | undefined,
): status is CompleteGitMembershipStatus {
  return Boolean(
    status &&
    status.status_state !== "loading" &&
    status.status_state !== "unavailable" &&
    status.files_complete !== false &&
    status.files !== undefined,
  );
}

function hasSelectedLayer(
  file: NonNullable<GitMembershipStatus["files"]>[string],
  changeLayer: GitChangeLayer | undefined,
): boolean {
  if (!changeLayer) return true;
  if (changeLayer === "staged") {
    return (
      file.staged_change !== undefined ||
      (file.staged === true && file.unstaged_change === undefined)
    );
  }
  return file.unstaged_change !== undefined || file.staged !== true;
}

export function shouldCloseFileDiffPanel(
  gitStatus: GitMembershipStatus | undefined,
  filePath: string,
  repositoryName?: string,
  changeLayer?: GitChangeLayer,
  statusByRepo?: RepositoryGitMembership[],
): boolean {
  const status = statusByRepo?.length
    ? statusByRepo.find((entry) => entry.repository_name === (repositoryName ?? ""))?.status
    : gitStatus;
  if (!isCompleteMembership(status)) return false;
  const file = status.files[filePath];
  return !file || !hasSelectedLayer(file, changeLayer);
}

function shouldCloseFileDiffPanelAggregate(opts: {
  prevFileSeenRef: MutableRefObject<boolean>;
  gitStatus: GitMembershipStatus | undefined;
  filePath: string;
  repositoryName: string | undefined;
  changeLayer: GitChangeLayer | undefined;
  statusByRepo: RepositoryGitMembership[] | undefined;
  onBecameEmpty: (() => void) | undefined;
}): boolean {
  const {
    prevFileSeenRef,
    gitStatus,
    filePath,
    repositoryName,
    changeLayer,
    statusByRepo,
    onBecameEmpty,
  } = opts;
  const shouldClose = shouldCloseFileDiffPanel(
    gitStatus,
    filePath,
    repositoryName,
    changeLayer,
    statusByRepo,
  );
  if (prevFileSeenRef.current && shouldClose) {
    onBecameEmpty?.();
    return true;
  }
  if (!shouldClose) prevFileSeenRef.current = true;
  return false;
}

export function useAutoCloseWhenEmpty(opts: {
  mode: "all" | "file";
  filePath: string | undefined;
  sourceFilter: "all" | ReviewSource;
  gitStatus: GitMembershipStatus | undefined;
  statusByRepo?: RepositoryGitMembership[];
  repositoryName?: string;
  changeLayer?: GitChangeLayer;
  visibleCount: number;
  prDiffLoading: boolean;
  onBecameEmpty: (() => void) | undefined;
}) {
  const {
    mode,
    filePath,
    sourceFilter,
    gitStatus,
    statusByRepo,
    repositoryName,
    changeLayer,
    visibleCount,
    prDiffLoading,
    onBecameEmpty,
  } = opts;
  const prevVisibleCountRef = useRef<number | null>(null);
  const prevFileSeenRef = useRef(false);
  const prevSourceFilterRef = useRef<typeof sourceFilter | null>(null);

  useEffect(() => {
    if (!onBecameEmpty) return;
    if (prDiffLoading) {
      prevVisibleCountRef.current = visibleCount;
      return;
    }
    if (mode === "file" && filePath) {
      if (sourceFilter === "all") {
        shouldCloseFileDiffPanelAggregate({
          prevFileSeenRef,
          gitStatus,
          filePath,
          repositoryName,
          changeLayer,
          statusByRepo,
          onBecameEmpty,
        });
        return;
      }
      if (prevSourceFilterRef.current !== sourceFilter) {
        prevSourceFilterRef.current = sourceFilter;
        prevVisibleCountRef.current = null;
      }
      const prevCount = prevVisibleCountRef.current;
      prevVisibleCountRef.current = visibleCount;
      if (prevCount !== null && prevCount > 0 && visibleCount === 0) onBecameEmpty();
      return;
    }

    if (sourceFilter !== "all") return;
    const prevCount = prevVisibleCountRef.current;
    if (prevCount !== null && prevCount > 0 && visibleCount === 0) onBecameEmpty();
    prevVisibleCountRef.current = visibleCount;
  }, [
    mode,
    filePath,
    sourceFilter,
    gitStatus,
    statusByRepo,
    repositoryName,
    changeLayer,
    onBecameEmpty,
    visibleCount,
    prDiffLoading,
  ]);
}

type FilterVisibleFilesOpts = {
  mode: "all" | "file";
  filePath: string | undefined;
  fileRepositoryName: string | undefined;
  sourceFilter: "all" | ReviewSource;
  rawPRFiles?: ReviewFile[];
  prKey?: string;
  changeLayer?: GitChangeLayer;
};

function projectReviewFileLayer(file: ReviewFile, layer: GitChangeLayer): ReviewFile | null {
  if (file.source !== "uncommitted") return file;
  const facet = layer === "staged" ? file.staged_change : file.unstaged_change;
  if (!facet) {
    if (file.staged !== (layer === "staged")) return null;
    return { ...file, change_layer: layer };
  }
  return {
    ...file,
    ...facet,
    staged: layer === "staged",
    change_layer: layer,
  };
}

type PersistedReviewState = { reviewed: boolean; diffHash: string };

function reviewStateFiles(files: ReviewFile[]): ReviewFile[] {
  const stateFiles: ReviewFile[] = [];
  for (const file of files) {
    stateFiles.push(file);
    if (
      file.source !== "uncommitted" ||
      file.change_layer ||
      !file.staged_change ||
      !file.unstaged_change
    ) {
      continue;
    }
    for (const layer of ["staged", "unstaged"] as const) {
      const projected = projectReviewFileLayer(file, layer);
      if (projected) stateFiles.push(projected);
    }
  }
  return stateFiles;
}

export function computeChangesReviewSets(
  files: ReviewFile[],
  reviews: ReadonlyMap<string, PersistedReviewState>,
): { reviewedFiles: Set<string>; staleFiles: Set<string> } {
  const reviewedFiles = new Set<string>();
  const staleFiles = new Set<string>();
  for (const file of reviewStateFiles(files)) {
    if (file.diff_state === "pending" || file.diff_state === "unavailable") continue;
    const key = reviewFileKey(file);
    const reviewState = reviews.get(key);
    if (!reviewState?.reviewed) continue;
    const currentHash = hashDiff(file.diff);
    if (reviewState.diffHash && reviewState.diffHash !== currentHash) staleFiles.add(key);
    else reviewedFiles.add(key);
  }
  return { reviewedFiles, staleFiles };
}

export function reviewDiffHashForKey(files: ReviewFile[], key: string): string | null {
  const file = reviewStateFiles(files).find((candidate) => reviewFileKey(candidate) === key);
  if (!file || file.diff_state === "pending" || file.diff_state === "unavailable") return null;
  return hashDiff(file.diff);
}

function projectRequestedLayer(
  files: ReviewFile[],
  mode: FilterVisibleFilesOpts["mode"],
  changeLayer: GitChangeLayer | undefined,
): ReviewFile[] {
  if (mode !== "file" || !changeLayer) return files;
  return files
    .map((file) => projectReviewFileLayer(file, changeLayer))
    .filter((file): file is ReviewFile => file !== null);
}

export function filterVisibleFiles(
  allFiles: ReviewFile[],
  opts: FilterVisibleFilesOpts,
): ReviewFile[] {
  const { mode, filePath, fileRepositoryName, sourceFilter, rawPRFiles, prKey, changeLayer } = opts;
  const repositoryFilter = prKey && sourceFilter === "pr" ? undefined : fileRepositoryName;
  if (mode === "file" && filePath && sourceFilter === "pr" && rawPRFiles?.length) {
    let prFiles = rawPRFiles.filter((file) => file.path === filePath && file.source === "pr");
    if (repositoryFilter !== undefined) {
      prFiles = prFiles.filter((file) => (file.repository_name ?? "") === repositoryFilter);
    }
    if (prFiles.length > 0) return prFiles;
  }

  let files = allFiles;
  if (mode === "file" && filePath) {
    files = files.filter((file) => file.path === filePath);
    if (repositoryFilter !== undefined) {
      files = files.filter((file) => (file.repository_name ?? "") === repositoryFilter);
    }
  }
  if (sourceFilter !== "all") files = files.filter((file) => file.source === sourceFilter);
  return projectRequestedLayer(files, mode, changeLayer);
}

export function useVisibleDiffState(opts: {
  allFiles: ReviewFile[];
  rawPRFiles: ReviewFile[];
  mode: "all" | "file";
  filePath: string | undefined;
  fileRepositoryName: string | undefined;
  sourceFilter: "all" | ReviewSource;
  prKey: string | undefined;
  changeLayer: GitChangeLayer | undefined;
  fileRefs: Map<string, RefObject<HTMLDivElement | null>>;
  reviewedFiles: Set<string>;
  staleFiles: Set<string>;
}) {
  const {
    allFiles,
    rawPRFiles,
    mode,
    filePath,
    fileRepositoryName,
    sourceFilter,
    prKey,
    changeLayer,
    fileRefs,
    reviewedFiles,
    staleFiles,
  } = opts;
  const visibleFiles = useMemo(
    () =>
      filterVisibleFiles(allFiles, {
        mode,
        filePath,
        fileRepositoryName,
        sourceFilter,
        rawPRFiles,
        prKey,
        changeLayer,
      }),
    [allFiles, mode, filePath, fileRepositoryName, sourceFilter, rawPRFiles, prKey, changeLayer],
  );
  const visibleFileRefs = useMemo(() => {
    if (mode !== "file" || !filePath) return fileRefs;
    const refs = new Map<string, RefObject<HTMLDivElement | null>>();
    for (const file of visibleFiles) {
      const visibleKey = reviewFileKey(file);
      const rawKey = reviewFileKey({ path: file.path, repository_name: file.repository_name });
      const ref = fileRefs.get(visibleKey) ?? fileRefs.get(rawKey);
      if (ref) refs.set(visibleKey, ref);
    }
    return refs;
  }, [mode, filePath, fileRefs, visibleFiles]);
  const reviewedCount = useMemo(
    () =>
      visibleFiles.reduce((count, file) => {
        const key = reviewFileKey(file);
        return !staleFiles.has(key) && reviewedFiles.has(key) ? count + 1 : count;
      }, 0),
    [visibleFiles, reviewedFiles, staleFiles],
  );
  const totalCount = visibleFiles.length;
  const progressPercent = totalCount > 0 ? (reviewedCount / totalCount) * 100 : 0;
  return { visibleFiles, visibleFileRefs, reviewedCount, totalCount, progressPercent };
}
