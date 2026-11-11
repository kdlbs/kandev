"use client";

import { useEffect } from "react";
import type { FileTreeNode } from "@/lib/types/backend";
import { findUnloadedAncestor, getAncestorPaths } from "./file-tree-utils";

type UseRootRevealOptions = {
  rootPaths: string[];
  tree: FileTreeNode | null;
  expandedPaths: ReadonlySet<string>;
  setExpandedPaths: React.Dispatch<React.SetStateAction<Set<string>>>;
  isLoadingTree: boolean;
  loadChildren: (node: FileTreeNode, shouldApply?: () => boolean) => Promise<boolean>;
};

/**
 * Expands hidden ancestors of every repository root so an adopted worktree
 * under a collapsed folder such as `.claude/worktrees` is visible on load.
 * One missing ancestor is materialized per effect pass; the tree update
 * re-runs the effect for the next level.
 */
export function useFileTreeRootReveal({
  rootPaths,
  tree,
  expandedPaths,
  setExpandedPaths,
  isLoadingTree,
  loadChildren,
}: UseRootRevealOptions) {
  const rootKey = rootPaths.join("|");
  useEffect(() => {
    if (isLoadingTree || !tree || rootPaths.length === 0) return;
    const ancestors = rootPaths.flatMap((path) => getAncestorPaths(path));
    const missingExpanded = ancestors.filter((path) => !expandedPaths.has(path));
    if (missingExpanded.length > 0) {
      setExpandedPaths((previous) => {
        const next = new Set(previous);
        for (const path of missingExpanded) next.add(path);
        return next;
      });
    }
    let shouldApply = true;
    for (const path of rootPaths) {
      const missing = findUnloadedAncestor(tree, path, getAncestorPaths(path));
      if (missing) {
        void loadChildren(missing, () => shouldApply);
        break;
      }
    }
    return () => {
      shouldApply = false;
    };
    // rootKey covers the array identity; join avoids a new effect per render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [rootKey, tree, expandedPaths, isLoadingTree, loadChildren, setExpandedPaths]);
}
