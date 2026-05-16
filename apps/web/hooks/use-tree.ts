"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";

/**
 * Headless tree state. Owns expansion + derived flat visibleRows + search
 * filtering. Selection, lazy-load, drag/drop, rename, etc. are the caller's
 * concern — keep this hook small and pure.
 *
 * Generic over the node type so it can drive every file-tree in the app,
 * including ones that use snake_case fields. Adapter functions
 * (getPath/getChildren/isDir) decouple the hook from any specific shape.
 */

export type SearchMode = "hide" | "collapse" | "expand";

export interface UseTreeOptions<N> {
  nodes: N[];
  getPath: (node: N) => string;
  getChildren: (node: N) => N[] | undefined;
  isDir: (node: N) => boolean;
  /** "all" pre-expands every directory once on mount; iterable seeds the set. */
  defaultExpanded?: "all" | Iterable<string>;
  search?: string;
  /**
   * - "hide": non-matching nodes are removed. Ancestors of matches are kept
   *   and force-expanded so matches stay reachable.
   * - "collapse": every node stays visible. Matching subtrees force-expanded,
   *   non-matching dirs force-collapsed.
   * - "expand": every node stays visible. Matching dirs and their ancestors
   *   force-expanded; other dirs retain their stored expand state.
   * Defaults to "hide".
   */
  searchMode?: SearchMode;
  /**
   * When true, single-child dir chains are merged into one row with a slash-
   * joined displayName (e.g. "src/ui"). The effective node is the deepest in
   * the chain; expanding it reveals the deepest dir's children.
   */
  chainCollapse?: boolean;
}

export interface VisibleRow<N> {
  /** The effective node — the deepest in a collapsed chain, else === chainRoot. */
  node: N;
  /** The top of the chain. Same as `node` when chainCollapse is off. */
  chainRoot: N;
  /** "src/ui" for chains, else the last path segment. */
  displayName: string;
  /** Path of the effective node. */
  path: string;
  depth: number;
  /** Effective expansion (after applying search-mode overrides). */
  isExpanded: boolean;
  isDir: boolean;
}

export interface UseTreeResult<N> {
  visibleRows: VisibleRow<N>[];
  expanded: ReadonlySet<string>;
  isExpanded: (path: string) => boolean;
  toggle: (path: string) => void;
  expand: (path: string) => void;
  collapse: (path: string) => void;
  expandAll: () => void;
  collapseAll: () => void;
  /** Adds every ancestor prefix of `path` to the expanded set. */
  expandAncestorsOf: (path: string) => void;
}

function lastSegment(p: string): string {
  const i = p.lastIndexOf("/");
  return i < 0 ? p : p.slice(i + 1);
}

function collectAllDirPaths<N>(
  nodes: N[],
  isDir: (n: N) => boolean,
  getPath: (n: N) => string,
  getChildren: (n: N) => N[] | undefined,
): string[] {
  const out: string[] = [];
  const stack = [...nodes];
  while (stack.length) {
    const n = stack.pop() as N;
    if (isDir(n)) {
      out.push(getPath(n));
      const ch = getChildren(n);
      if (ch && ch.length) stack.push(...ch);
    }
  }
  return out;
}

function ancestorPaths(path: string): string[] {
  const parts = path.split("/");
  const out: string[] = [];
  for (let i = 1; i < parts.length; i++) out.push(parts.slice(0, i).join("/"));
  return out;
}

interface ComputeOpts<N> {
  nodes: N[];
  expanded: Set<string>;
  search: string;
  searchMode: SearchMode;
  chainCollapse: boolean;
  getPath: (n: N) => string;
  getChildren: (n: N) => N[] | undefined;
  isDir: (n: N) => boolean;
}

/** Walk the chain of single-child dirs starting at `root`, returning the
 *  deepest effective node and the slash-joined display name. */
function walkChain<N>(
  root: N,
  chainCollapse: boolean,
  isDir: (n: N) => boolean,
  getPath: (n: N) => string,
  getChildren: (n: N) => N[] | undefined,
): { effective: N; displayName: string } {
  let effective = root;
  let displayName = lastSegment(getPath(root));
  if (!chainCollapse || !isDir(root)) return { effective, displayName };
  let kids = getChildren(effective) ?? [];
  while (kids.length === 1 && isDir(kids[0])) {
    effective = kids[0];
    displayName = `${displayName}/${lastSegment(getPath(effective))}`;
    kids = getChildren(effective) ?? [];
  }
  return { effective, displayName };
}

/** Precompute the "self-or-descendant matches search" map. */
function buildMatchMap<N>(
  nodes: N[],
  lowerSearch: string,
  getPath: (n: N) => string,
  getChildren: (n: N) => N[] | undefined,
): Map<string, boolean> {
  const out = new Map<string, boolean>();
  const recurse = (n: N): boolean => {
    const path = getPath(n);
    const self = lastSegment(path).toLowerCase().includes(lowerSearch);
    let any = self;
    for (const c of getChildren(n) ?? []) if (recurse(c)) any = true;
    out.set(path, any);
    return any;
  };
  for (const n of nodes) recurse(n);
  return out;
}

/** Decide a dir's effective expansion under the current search mode. */
function effectiveExpansion(
  storedExpanded: boolean,
  matches: boolean,
  hasSearch: boolean,
  searchMode: SearchMode,
): boolean {
  if (!hasSearch) return storedExpanded;
  if (searchMode === "hide") return true;
  if (searchMode === "expand") return storedExpanded || matches;
  // "collapse"
  return matches;
}

function computeVisibleRows<N>(opts: ComputeOpts<N>): VisibleRow<N>[] {
  const { nodes, expanded, search, searchMode, chainCollapse, getPath, getChildren, isDir } = opts;
  const lowerSearch = search.toLowerCase();
  const hasSearch = lowerSearch.length > 0;
  const matchMap = hasSearch ? buildMatchMap(nodes, lowerSearch, getPath, getChildren) : null;

  const rows: VisibleRow<N>[] = [];
  const walk = (list: N[], depth: number) => {
    for (const root of list) {
      // Skip non-matching subtrees in "hide" mode.
      if (hasSearch && searchMode === "hide" && !(matchMap?.get(getPath(root)) ?? false)) continue;

      const { effective, displayName } = walkChain(
        root,
        chainCollapse,
        isDir,
        getPath,
        getChildren,
      );
      const effPath = getPath(effective);
      const effIsDir = isDir(effective);
      const isExpandedEff = effIsDir
        ? effectiveExpansion(
            expanded.has(effPath),
            matchMap?.get(effPath) ?? false,
            hasSearch,
            searchMode,
          )
        : false;

      rows.push({
        node: effective,
        chainRoot: root,
        displayName,
        path: effPath,
        depth,
        isExpanded: isExpandedEff,
        isDir: effIsDir,
      });

      if (effIsDir && isExpandedEff) walk(getChildren(effective) ?? [], depth + 1);
    }
  };
  walk(nodes, 0);
  return rows;
}

function useExpandedState<N>(
  nodes: N[],
  defaultExpanded: UseTreeOptions<N>["defaultExpanded"],
  getPath: (n: N) => string,
  getChildren: (n: N) => N[] | undefined,
  isDir: (n: N) => boolean,
) {
  const nodesRef = useRef(nodes);
  const getPathRef = useRef(getPath);
  const getChildrenRef = useRef(getChildren);
  const isDirRef = useRef(isDir);
  useEffect(() => {
    nodesRef.current = nodes;
    getPathRef.current = getPath;
    getChildrenRef.current = getChildren;
    isDirRef.current = isDir;
  });

  const [expanded, setExpanded] = useState<Set<string>>(() => {
    if (defaultExpanded === "all") {
      return new Set(collectAllDirPaths(nodes, isDir, getPath, getChildren));
    }
    if (defaultExpanded) return new Set(defaultExpanded);
    return new Set<string>();
  });

  const isExpandedFn = useCallback((p: string) => expanded.has(p), [expanded]);
  const toggle = useCallback((p: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(p)) next.delete(p);
      else next.add(p);
      return next;
    });
  }, []);
  const expand = useCallback((p: string) => {
    setExpanded((prev) => (prev.has(p) ? prev : new Set(prev).add(p)));
  }, []);
  const collapse = useCallback((p: string) => {
    setExpanded((prev) => {
      if (!prev.has(p)) return prev;
      const next = new Set(prev);
      next.delete(p);
      return next;
    });
  }, []);
  const expandAll = useCallback(() => {
    setExpanded(
      new Set(
        collectAllDirPaths(
          nodesRef.current,
          isDirRef.current,
          getPathRef.current,
          getChildrenRef.current,
        ),
      ),
    );
  }, []);
  const collapseAll = useCallback(() => setExpanded(new Set<string>()), []);
  const expandAncestorsOf = useCallback((p: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      for (const a of ancestorPaths(p)) next.add(a);
      return next;
    });
  }, []);

  return {
    expanded,
    isExpanded: isExpandedFn,
    toggle,
    expand,
    collapse,
    expandAll,
    collapseAll,
    expandAncestorsOf,
  };
}

export function useTree<N>(opts: UseTreeOptions<N>): UseTreeResult<N> {
  const {
    nodes,
    getPath,
    getChildren,
    isDir,
    defaultExpanded,
    search = "",
    searchMode = "hide",
    chainCollapse = false,
  } = opts;
  const state = useExpandedState(nodes, defaultExpanded, getPath, getChildren, isDir);
  const visibleRows = useMemo<VisibleRow<N>[]>(
    () =>
      computeVisibleRows({
        nodes,
        expanded: state.expanded,
        search,
        searchMode,
        chainCollapse,
        getPath,
        getChildren,
        isDir,
      }),
    [nodes, state.expanded, search, searchMode, chainCollapse, getPath, getChildren, isDir],
  );
  return { ...state, visibleRows };
}
