"use client";

import { useEffect, useRef, useState } from "react";
import type React from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import { findUnloadedAncestor, getAncestorPaths } from "./file-tree-utils";

const DEFAULT_RETRY_DELAYS_MS = [250, 1_000] as const;

type RevealRun = {
  key: string;
  attemptsByPath: Map<string, number>;
  inFlightPath: string | null;
  retryTimer: ReturnType<typeof setTimeout> | null;
};

export type FileTreeRevealParams = {
  activeFilePath: string | null | undefined;
  sessionId: string;
  tree: FileTreeNode | null;
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>;
  isLoading: (path: string) => boolean;
  loadChildren: (node: FileTreeNode, shouldApply: () => boolean) => Promise<boolean>;
  retryDelaysMs?: readonly number[];
};

function clearRetry(run: RevealRun | null) {
  if (!run?.retryTimer) return;
  clearTimeout(run.retryTimer);
  run.retryTimer = null;
}

function createRun(key: string): RevealRun {
  return { key, attemptsByPath: new Map(), inFlightPath: null, retryTimer: null };
}

function expandAncestors(
  targetPath: string,
  setExpandedPaths: FileTreeRevealParams["setExpandedPaths"],
) {
  const ancestors = getAncestorPaths(targetPath);
  setExpandedPaths((previous) => {
    if (ancestors.every((path) => previous.has(path))) return previous;
    const expanded = new Set(previous);
    for (const path of ancestors) expanded.add(path);
    return expanded;
  });
}

/** Reveal an active file by expanding and lazily materializing its ancestor chain. */
export function useFileTreeReveal(params: FileTreeRevealParams) {
  const {
    activeFilePath,
    sessionId,
    tree,
    setExpandedPaths,
    isLoading,
    loadChildren,
    retryDelaysMs = DEFAULT_RETRY_DELAYS_MS,
  } = params;
  const latestRef = useRef({ isLoading, loadChildren, retryDelaysMs });
  latestRef.current = { isLoading, loadChildren, retryDelaysMs };
  const runRef = useRef<RevealRun | null>(null);
  const [revision, setRevision] = useState(0);
  const revealKey = activeFilePath ? `${sessionId}\0${activeFilePath}` : null;
  const treeAvailable = tree !== null;

  useEffect(() => {
    if (!activeFilePath || !treeAvailable) return;
    expandAncestors(activeFilePath, setExpandedPaths);
  }, [activeFilePath, revealKey, setExpandedPaths, treeAvailable]);

  useEffect(() => {
    if (!activeFilePath || !revealKey || !tree) {
      clearRetry(runRef.current);
      runRef.current = null;
      return;
    }

    if (runRef.current?.key !== revealKey) {
      clearRetry(runRef.current);
      runRef.current = createRun(revealKey);
    }
    const run = runRef.current;
    const ancestor = findUnloadedAncestor(tree, activeFilePath, getAncestorPaths(activeFilePath));
    if (!ancestor) {
      clearRetry(run);
      return;
    }
    if (run.retryTimer || run.inFlightPath) return;

    const latest = latestRef.current;
    if (latest.isLoading(ancestor.path)) {
      run.retryTimer = setTimeout(() => {
        if (runRef.current !== run) return;
        run.retryTimer = null;
        setRevision((value) => value + 1);
      }, latest.retryDelaysMs[0] ?? DEFAULT_RETRY_DELAYS_MS[0]);
      return;
    }

    const attempts = run.attemptsByPath.get(ancestor.path) ?? 0;
    if (attempts >= latest.retryDelaysMs.length + 1) return;
    run.attemptsByPath.set(ancestor.path, attempts + 1);
    run.inFlightPath = ancestor.path;
    void latest
      .loadChildren(ancestor, () => runRef.current === run)
      .then((loaded) => {
        if (runRef.current !== run) return;
        run.inFlightPath = null;
        if (loaded) {
          setRevision((value) => value + 1);
          return;
        }
        const delay = latest.retryDelaysMs[attempts];
        if (delay === undefined) return;
        run.retryTimer = setTimeout(() => {
          if (runRef.current !== run) return;
          run.retryTimer = null;
          setRevision((value) => value + 1);
        }, delay);
      });
  }, [activeFilePath, revealKey, revision, tree]);

  useEffect(
    () => () => {
      clearRetry(runRef.current);
      runRef.current = null;
    },
    [],
  );
}
