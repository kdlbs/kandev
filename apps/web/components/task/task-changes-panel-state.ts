"use client";

import { useEffect, useMemo, useRef, type MutableRefObject, type RefObject } from "react";
import { reviewFileKey, splitReviewFileKey, type ReviewFile } from "@/components/review/types";
import type { ReviewSource } from "@/hooks/domains/session/use-review-sources";

export function shouldDeferReviewStateForPR(
  hasSelectedPR: boolean,
  prDiffLoading: boolean,
  sourceFilter: "all" | ReviewSource,
): boolean {
  const usesPRDiff = sourceFilter === "all" || sourceFilter === "pr";
  return hasSelectedPR && prDiffLoading && usesPRDiff;
}

export function shouldCloseFileDiffPanel(
  gitStatus: { files?: Record<string, { diff?: string }> } | undefined,
  filePath: string,
): boolean {
  if (!gitStatus) return false;
  return !gitStatus.files?.[filePath]?.diff;
}

function shouldCloseFileDiffPanelAggregate(
  prevFileSeenRef: MutableRefObject<boolean>,
  gitStatus: { files?: Record<string, { diff?: string }> } | undefined,
  filePath: string,
  onBecameEmpty: (() => void) | undefined,
): boolean {
  const shouldClose = shouldCloseFileDiffPanel(gitStatus, filePath);
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
  gitStatus: { files?: Record<string, { diff?: string }> } | undefined;
  visibleCount: number;
  prDiffLoading: boolean;
  onBecameEmpty: (() => void) | undefined;
}) {
  const { mode, filePath, sourceFilter, gitStatus, visibleCount, prDiffLoading, onBecameEmpty } =
    opts;
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
        shouldCloseFileDiffPanelAggregate(prevFileSeenRef, gitStatus, filePath, onBecameEmpty);
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
  }, [mode, filePath, sourceFilter, gitStatus, onBecameEmpty, visibleCount, prDiffLoading]);
}

type FilterVisibleFilesOpts = {
  mode: "all" | "file";
  filePath: string | undefined;
  fileRepositoryName: string | undefined;
  sourceFilter: "all" | ReviewSource;
  rawPRFiles?: ReviewFile[];
  prKey?: string;
};

export function filterVisibleFiles(
  allFiles: ReviewFile[],
  opts: FilterVisibleFilesOpts,
): ReviewFile[] {
  const { mode, filePath, fileRepositoryName, sourceFilter, rawPRFiles, prKey } = opts;
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
  return files;
}

export function useVisibleDiffState(opts: {
  allFiles: ReviewFile[];
  rawPRFiles: ReviewFile[];
  mode: "all" | "file";
  filePath: string | undefined;
  fileRepositoryName: string | undefined;
  sourceFilter: "all" | ReviewSource;
  prKey: string | undefined;
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
      }),
    [allFiles, mode, filePath, fileRepositoryName, sourceFilter, rawPRFiles, prKey],
  );
  const repositoryFilter = prKey && sourceFilter === "pr" ? undefined : fileRepositoryName;
  const visibleFileRefs = useMemo(() => {
    if (mode !== "file" || !filePath) return fileRefs;
    const refs = new Map<string, RefObject<HTMLDivElement | null>>();
    for (const [key, ref] of fileRefs.entries()) {
      const split = splitReviewFileKey(key);
      if (split.path !== filePath) continue;
      if (repositoryFilter !== undefined && (split.repositoryName || "") !== repositoryFilter) {
        continue;
      }
      refs.set(key, ref);
    }
    return refs;
  }, [mode, filePath, fileRefs, repositoryFilter]);
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
