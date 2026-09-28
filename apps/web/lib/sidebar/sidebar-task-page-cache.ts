import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";

const MAX_PAGES = 5;
const MAX_BYTES = 2 * 1024 * 1024;
const MAX_AGE_MS = 5 * 60 * 1000;

type PageSnapshot = { page: SidebarTaskPageResponse; bytes: number; fetchedAt: number };
type Request = {
  promise: Promise<SidebarTaskPageResponse>;
  controller: AbortController;
  consumers: number;
};
type PageStore = { getState: () => AppState };

/** Bounded, ephemeral first pages and shared requests owned by one app store. */
export class SidebarTaskPageCache {
  private pages = new Map<string, PageSnapshot>();
  private requests = new Map<string, Request>();
  private scope = "";
  private revision = 0;
  private summaries: AppState["sidebarStatusSummaryByWorkspaceId"][string] | undefined;
  private bytes = 0;
  private epoch = 0;
  private accessDeniedListeners = new Set<() => void>();

  constructor(private store: PageStore) {}

  private synchronize() {
    const state = this.store.getState();
    const workspaceId = state.workspaces.activeId ?? "";
    const scope = sidebarTaskPageScope(state);
    const revision = state.sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId] ?? 0;
    const summaries = state.sidebarStatusSummaryByWorkspaceId?.[workspaceId];
    if (scope !== this.scope) {
      this.clear();
      this.scope = scope;
    } else if (revision !== this.revision) {
      this.pages.clear();
      this.bytes = 0;
      this.epoch += 1;
    } else if (summaries !== this.summaries) {
      for (const [key, entry] of this.pages) {
        if (
          entry.page.entries.some(
            (row) => row.task && summaries?.[row.task.id] !== this.summaries?.[row.task.id],
          )
        ) {
          this.remove(key);
        }
      }
      this.epoch += 1;
    }
    this.revision = revision;
    this.summaries = summaries;
    for (const [key, entry] of this.pages) {
      if (Date.now() - entry.fetchedAt >= MAX_AGE_MS) this.remove(key);
    }
    return this.epoch;
  }

  private remove(key: string) {
    const previous = this.pages.get(key);
    if (previous) this.bytes -= previous.bytes;
    this.pages.delete(key);
  }

  clear() {
    this.epoch += 1;
    this.pages.clear();
    this.bytes = 0;
    for (const request of this.requests.values()) request.controller.abort();
    this.requests.clear();
  }

  subscribeAccessDenied(listener: () => void) {
    this.accessDeniedListeners.add(listener);
    return () => {
      this.accessDeniedListeners.delete(listener);
    };
  }

  denyAccess() {
    this.clear();
    for (const listener of this.accessDeniedListeners) listener();
  }

  get(key: string): SidebarTaskPageResponse | null {
    this.synchronize();
    return this.pages.get(key)?.page ?? null;
  }

  private retain(key: string, page: SidebarTaskPageResponse) {
    if (page.page !== 1) return;
    this.remove(key);
    const bytes = new TextEncoder().encode(JSON.stringify(page)).byteLength;
    if (bytes > MAX_BYTES) return;
    this.pages.set(key, { page, bytes, fetchedAt: Date.now() });
    this.bytes += bytes;
    while (this.pages.size > MAX_PAGES || this.bytes > MAX_BYTES) {
      this.remove(this.pages.keys().next().value!);
    }
  }

  request(workspaceId: string, query: SidebarTaskQuery, cacheKey: string) {
    const epoch = this.synchronize();
    const requestKey = JSON.stringify([cacheKey, query]);
    // Recency follows a view activation/read, never extends its fetch-age expiry.
    const snapshot = this.pages.get(cacheKey);
    if (snapshot) {
      this.pages.delete(cacheKey);
      this.pages.set(cacheKey, snapshot);
    }
    let request = this.requests.get(requestKey);
    if (!request) {
      const controller = new AbortController();
      const created: Request = {
        controller,
        consumers: 0,
        promise: querySidebarTasks(workspaceId, query, {
          cache: "no-store",
          init: { signal: controller.signal },
        })
          .then((page) => {
            if (
              this.synchronize() === epoch &&
              !controller.signal.aborted &&
              this.requests.get(requestKey) === created
            ) {
              this.retain(cacheKey, page);
            }
            return page;
          })
          .finally(() => {
            if (this.requests.get(requestKey) === created) this.requests.delete(requestKey);
          }),
      };
      request = created;
      this.requests.set(requestKey, request);
    }
    request.consumers += 1;
    let released = false;
    return {
      promise: request.promise,
      release: () => {
        if (released) return;
        released = true;
        request.consumers -= 1;
        if (request.consumers === 0 && this.requests.get(requestKey) === request) {
          request.controller.abort();
          this.requests.delete(requestKey);
        }
      },
    };
  }
}

const caches = new WeakMap<PageStore, SidebarTaskPageCache>();
export function sidebarTaskPageCache(store: PageStore): SidebarTaskPageCache {
  let cache = caches.get(store);
  if (!cache) {
    cache = new SidebarTaskPageCache(store);
    caches.set(store, cache);
  }
  return cache;
}

export function sidebarTaskPageScope(state: AppState): string {
  return JSON.stringify([
    state.workspaces.activeId,
    state.workspaceContextGeneration,
    state.auth?.mode,
    state.auth?.authenticated,
    state.auth?.user?.id,
  ]);
}
