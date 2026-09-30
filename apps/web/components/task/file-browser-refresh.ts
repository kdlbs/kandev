import type React from "react";
import { getWebSocketClient } from "@/lib/ws/connection";
import { requestFileTree } from "@/lib/ws/workspace-files";
import type { FileTreeNode } from "@/lib/types/backend";
import { isWorkspaceTreePath } from "@/lib/workspace-file-path";
import { findNodeByPath } from "./file-tree-utils";
import { mergeLoadedFolder } from "./file-browser-restore";
import type { LoadState } from "./file-browser-hooks";

function nearestExpandedFolder(parentPath: string, expandedPaths: ReadonlySet<string>): string {
  let candidate = parentPath;
  while (candidate) {
    if (expandedPaths.has(candidate)) return candidate;
    const lastSlash = candidate.lastIndexOf("/");
    candidate = lastSlash === -1 ? "" : candidate.substring(0, lastSlash);
  }
  return "";
}

function isExpandedPathInRefreshScope(path: string, repositoryName?: string): boolean {
  if (!isWorkspaceTreePath(path)) return false;
  return !repositoryName || path === repositoryName || path.startsWith(`${repositoryName}/`);
}

type RefreshTicket = { current: boolean; treeApplied: boolean; stateApplied: boolean };

export class FolderRefreshes {
  private readonly latest = new Map<string, RefreshTicket>();

  begin(path: string) {
    const previous = this.latest.get(path);
    if (previous) previous.current = false;
    const ticket = { current: true, treeApplied: false, stateApplied: false };
    this.latest.set(path, ticket);
    return ticket;
  }

  fail(path: string, ticket: RefreshTicket) {
    ticket.current = false;
    if (this.latest.get(path) === ticket) this.latest.delete(path);
  }

  committed() {
    // Keep queued publication tokens until their updater has committed.
    for (const [path, ticket] of this.latest) {
      if (ticket.treeApplied && ticket.stateApplied) this.latest.delete(path);
    }
  }

  retire() {
    for (const ticket of this.latest.values()) ticket.current = false;
    this.latest.clear();
  }
}

type RefreshContext = {
  client: ReturnType<typeof getWebSocketClient>;
  sessionId: string;
  expandedPaths: ReadonlySet<string>;
  changes: Array<{ path: string; operation?: string; repository_name?: string }>;
  setTree: React.Dispatch<React.SetStateAction<FileTreeNode | null>>;
  setLoadState: React.Dispatch<React.SetStateAction<LoadState>>;
  isCurrent?: () => boolean;
  refreshes?: FolderRefreshes;
};

function changedFolders({ changes, expandedPaths }: RefreshContext) {
  const folders = new Set<string>();
  for (const change of changes) {
    if (change.operation === "refresh") {
      folders.add("");
      for (const path of expandedPaths) {
        if (isExpandedPathInRefreshScope(path, change.repository_name)) folders.add(path);
      }
    } else if (isWorkspaceTreePath(change.path)) {
      const slash = change.path.lastIndexOf("/");
      const parent = slash === -1 ? "" : change.path.substring(0, slash);
      folders.add(nearestExpandedFolder(parent, expandedPaths));
      if (change.path === "" || expandedPaths.has(change.path)) folders.add(change.path);
    }
  }
  return folders;
}

type FolderUpdate = { path: string; children?: FileTreeNode[]; ticket: RefreshTicket };
function mergeUpdates(tree: FileTreeNode, updates: FolderUpdate[]) {
  let next = tree;
  for (const update of updates
    .filter((item) => item.ticket.current)
    .sort((a, b) => a.path.length - b.path.length)) {
    const target = findNodeByPath(next, update.path || tree.path);
    if (target?.is_dir) next = mergeLoadedFolder(next, { ...target, children: update.children });
  }
  return next;
}

/** Apply incoming file changes to the current tree by refreshing affected folders. */
export function applyFileChanges(ctx: RefreshContext) {
  const {
    client,
    sessionId,
    setTree,
    setLoadState,
    isCurrent = () => true,
    refreshes = new FolderRefreshes(),
  } = ctx;
  if (!isCurrent()) return;
  const requests = Array.from(changedFolders(ctx), (path) => ({
    path,
    ticket: refreshes.begin(path),
  }));
  if (requests.length === 0) return;
  void Promise.all(
    requests.map(async ({ path, ticket }): Promise<FolderUpdate | null> => {
      try {
        const response = await requestFileTree(client!, sessionId, path, 1);
        return { path, ticket, children: response.root?.children };
      } catch {
        // A failed read does not establish authoritative absence.
        refreshes.fail(path, ticket);
        return null;
      }
    }),
  ).then((results) => {
    const updates = results.filter(
      (result): result is FolderUpdate => result !== null && result.ticket.current,
    );
    if (!isCurrent() || updates.length === 0) return;
    setTree((previous) => {
      for (const update of updates) update.ticket.treeApplied = true;
      if (!previous || !isCurrent()) return previous;
      return mergeUpdates(previous, updates);
    });
    setLoadState((previous) => {
      for (const update of updates) update.ticket.stateApplied = true;
      return isCurrent() && updates.some((update) => update.ticket.current) ? "loaded" : previous;
    });
  });
}
