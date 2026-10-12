import { useLayoutEffect, useMemo } from "react";
import type { CommitDetailTarget } from "@/lib/state/diff-target-types";
import type { FileInfo } from "@/lib/state/store";

export type CommitInlineFile = {
  path: string;
  status: FileInfo["status"];
  plus?: number;
  minus?: number;
  oldPath?: string;
  isSymlink?: boolean;
  repositoryName?: string;
};

export type CommitInlineDetailSnapshot = {
  expanded: boolean;
  status: "idle" | "loading" | "error" | "loaded";
  files: CommitInlineFile[];
  error?: string;
};

type RequestCommitFiles = (target: CommitDetailTarget) => Promise<Record<string, FileInfo>>;

type ChangesInlineCommitStateOptions = {
  contextKey: string;
  request: RequestCommitFiles;
  onError?: (target: CommitDetailTarget, error: unknown) => void;
  maxCollapsedTargets?: number;
  maxCollapsedFiles?: number;
};

const EMPTY_SNAPSHOT: CommitInlineDetailSnapshot = {
  expanded: false,
  status: "idle",
  files: [],
};

export function commitDetailTargetKey(target: CommitDetailTarget): string {
  return target.source === "local"
    ? JSON.stringify(["local", target.repo ?? "", target.sha])
    : JSON.stringify([
        "github",
        target.workspaceId,
        target.owner,
        target.repo,
        target.repositoryName ?? "",
        target.sha,
      ]);
}

function toInlineFiles(
  files: Record<string, FileInfo>,
  target: CommitDetailTarget,
): CommitInlineFile[] {
  const repositoryName =
    target.source === "local" ? target.repo : (target.repositoryName ?? target.repo);
  return Object.entries(files)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([path, file]) => ({
      path,
      status: file.status,
      plus: file.additions,
      minus: file.deletions,
      oldPath: file.old_path,
      isSymlink: file.is_symlink,
      repositoryName,
    }));
}

/** Owns explicit inline detail requests and a small cache of collapsed snapshots. */
export class ChangesInlineCommitState {
  private contextKey: string;
  private generation = 0;
  private retired = false;
  private version = 0;
  private readonly request: RequestCommitFiles;
  private readonly onError?: ChangesInlineCommitStateOptions["onError"];
  private readonly maxCollapsedTargets: number;
  private readonly maxCollapsedFiles: number;
  private readonly snapshots = new Map<string, CommitInlineDetailSnapshot>();
  private readonly inFlight = new Map<string, Promise<void>>();
  private readonly collapsedLru = new Map<string, number>();
  private readonly mountedTargets = new Map<string, number>();
  private readonly listeners = new Set<() => void>();

  constructor(options: ChangesInlineCommitStateOptions) {
    this.contextKey = options.contextKey;
    this.request = options.request;
    this.onError = options.onError;
    this.maxCollapsedTargets = options.maxCollapsedTargets ?? 8;
    this.maxCollapsedFiles = options.maxCollapsedFiles ?? 50_000;
  }

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getVersion = (): number => this.version;

  get(target: CommitDetailTarget): CommitInlineDetailSnapshot {
    return this.snapshots.get(commitDetailTargetKey(target)) ?? EMPTY_SNAPSHOT;
  }

  getPendingRequestCount(): number {
    let pending = 0;
    for (const snapshot of this.snapshots.values()) {
      if (snapshot.status === "loading") pending += 1;
    }
    return pending;
  }

  registerMountedTargetKey(key: string): () => void {
    if (this.retired) return () => {};
    this.mountedTargets.set(key, (this.mountedTargets.get(key) ?? 0) + 1);
    this.collapsedLru.delete(key);
    let released = false;
    return () => {
      if (released) return;
      released = true;
      const count = this.mountedTargets.get(key) ?? 0;
      if (count > 1) {
        this.mountedTargets.set(key, count - 1);
        return;
      }
      this.mountedTargets.delete(key);
      const snapshot = this.snapshots.get(key);
      if (snapshot?.status === "loaded" && !snapshot.expanded) {
        this.retainCollapsedSnapshot(key, snapshot.files.length);
      }
    };
  }

  activate(): void {
    this.retired = false;
  }

  retire(): void {
    if (this.retired) return;
    this.retired = true;
    this.invalidateRequests();
    this.listeners.clear();
  }

  setContext(contextKey: string): void {
    if (contextKey === this.contextKey) return;
    this.contextKey = contextKey;
    this.invalidateRequests();
    this.emit();
  }

  expand(target: CommitDetailTarget): Promise<void> {
    if (this.retired) return Promise.resolve();
    const key = commitDetailTargetKey(target);
    const current = this.get(target);
    if (current.status === "loaded") {
      this.collapsedLru.delete(key);
      this.setSnapshot(key, { ...current, expanded: true });
      return Promise.resolve();
    }
    if (current.status === "error") {
      this.setSnapshot(key, { ...current, expanded: true });
      return Promise.resolve();
    }
    const pending = this.inFlight.get(key);
    this.setSnapshot(key, { ...current, expanded: true });
    if (pending) return pending;
    return this.load(target, key);
  }

  retry(target: CommitDetailTarget): Promise<void> {
    if (this.retired) return Promise.resolve();
    const current = this.get(target);
    if (current.status !== "error") return this.expand(target);
    return this.load(target, commitDetailTargetKey(target));
  }

  collapse(target: CommitDetailTarget): void {
    if (this.retired) return;
    const key = commitDetailTargetKey(target);
    const current = this.get(target);
    if (!current.expanded) return;
    this.setSnapshot(key, { ...current, expanded: false });
    if (current.status === "loaded") this.retainCollapsedSnapshot(key, current.files.length);
  }

  private load(target: CommitDetailTarget, key: string): Promise<void> {
    if (this.retired) return Promise.resolve();
    const generation = this.generation;
    const contextKey = this.contextKey;
    const current = this.get(target);
    this.setSnapshot(key, { ...current, status: "loading", error: undefined });
    const request = this.request(target)
      .then((files) => {
        if (this.retired || generation !== this.generation || contextKey !== this.contextKey) {
          return;
        }
        const latest = this.snapshots.get(key);
        if (!latest) return;
        const snapshot: CommitInlineDetailSnapshot = {
          expanded: latest.expanded,
          status: "loaded",
          files: toInlineFiles(files, target),
        };
        this.setSnapshot(key, snapshot);
        if (!snapshot.expanded) this.retainCollapsedSnapshot(key, snapshot.files.length);
      })
      .catch((error: unknown) => {
        if (this.retired || generation !== this.generation || contextKey !== this.contextKey)
          return;
        const latest = this.snapshots.get(key);
        if (!latest) return;
        const message = error instanceof Error ? error.message : String(error);
        this.setSnapshot(key, { ...latest, status: "error", error: message, files: [] });
        this.onError?.(target, error);
      })
      .finally(() => {
        if (!this.retired && generation === this.generation) this.inFlight.delete(key);
      });
    this.inFlight.set(key, request);
    return request;
  }

  private retainCollapsedSnapshot(key: string, fileCount: number): void {
    if (this.mountedTargets.has(key)) {
      this.collapsedLru.delete(key);
      return;
    }
    if (fileCount > this.maxCollapsedFiles || this.maxCollapsedTargets <= 0) {
      this.evict(key);
      return;
    }
    this.collapsedLru.delete(key);
    this.collapsedLru.set(key, fileCount);
    while (
      this.collapsedLru.size > this.maxCollapsedTargets ||
      collapsedFileCount(this.collapsedLru) > this.maxCollapsedFiles
    ) {
      const oldest = this.collapsedLru.keys().next().value as string | undefined;
      if (oldest === undefined) break;
      this.evict(oldest);
    }
  }

  private invalidateRequests(): void {
    this.generation += 1;
    this.snapshots.clear();
    this.inFlight.clear();
    this.collapsedLru.clear();
    this.mountedTargets.clear();
  }

  private evict(key: string): void {
    if (this.mountedTargets.has(key)) return;
    this.collapsedLru.delete(key);
    const snapshot = this.snapshots.get(key);
    if (snapshot && !snapshot.expanded) this.snapshots.delete(key);
    this.emit();
  }

  private setSnapshot(key: string, snapshot: CommitInlineDetailSnapshot): void {
    this.snapshots.set(key, snapshot);
    this.emit();
  }

  private emit(): void {
    this.version += 1;
    for (const listener of this.listeners) listener();
  }
}

/** Keeps one inline-detail controller for a context and retires it at its owner boundary. */
export function useChangesInlineCommitState(
  contextKey: string,
  createState: (contextKey: string) => ChangesInlineCommitState,
): ChangesInlineCommitState {
  const state = useMemo(() => createState(contextKey), [contextKey, createState]);
  useLayoutEffect(() => {
    state.activate();
    return () => state.retire();
  }, [state]);
  return state;
}

function collapsedFileCount(entries: ReadonlyMap<string, number>): number {
  let count = 0;
  for (const fileCount of entries.values()) count += fileCount;
  return count;
}
