import { querySidebarTasks } from "@/lib/api/domains/kanban-api";
import type { SidebarTaskPageResponse, SidebarTaskQuery } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { generateUUID } from "@/lib/utils";
import { reconcileSidebarPage, sidebarPageMembership } from "./sidebar-page-overviews";
import { repositoryIdentityForSavedRepository } from "./repository-rule-identity";

const MAX_PAGES = 5;
const MAX_BYTES = 2 * 1024 * 1024;
const MAX_AGE_MS = 5 * 60 * 1000;
const EMPTY_REPOSITORIES: NonNullable<AppState["repositories"]["itemsByWorkspaceId"][string]> = [];

type SidebarRankingInputs = {
  workspaceId: string;
  colors: AppState["userSettings"]["sidebarTaskColors"];
  automation: AppState["userSettings"]["sidebarTaskColorAutomation"];
  repositories: NonNullable<AppState["repositories"]["itemsByWorkspaceId"][string]>;
  snapshots: AppState["kanbanMulti"]["snapshots"];
};

let cachedRanking: { inputs: SidebarRankingInputs; key: string } | undefined;

type PageSnapshot = { page: SidebarTaskPageResponse; owner: string; fetchedAt: number };
type Request = {
  promise: Promise<SidebarTaskPageResponse>;
  controller: AbortController;
  consumers: number;
  owner: string;
  readId?: string;
};
type PageStore = {
  getState: () => AppState;
  subscribe?: (listener: (state: AppState, previous: AppState) => void) => () => void;
};
const byteSize = (value: unknown) => new TextEncoder().encode(JSON.stringify(value)).byteLength;

/** Bounded ID memberships and shared reads. All retained overview objects live in Zustand. */
export class SidebarTaskPageCache {
  private pages = new Map<string, PageSnapshot>();
  private requests = new Map<string, Request>();
  private scope = "";
  private rankingKey = "";
  private revision = 0;
  private summaries: AppState["sidebarStatusSummaryByWorkspaceId"][string] | undefined;
  private epoch = 0;
  private synchronizing = false;
  private expiry: ReturnType<typeof setTimeout> | undefined;
  private accessDeniedListeners = new Set<() => void>();
  private deletedTaskListeners = new Set<(taskIds: ReadonlySet<string>) => void>();

  constructor(private store: PageStore) {
    store.subscribe?.((state, previous) => {
      if (
        state.taskOverview === previous.taskOverview &&
        state.auth === previous.auth &&
        state.workspaces === previous.workspaces &&
        state.workspaceContextGeneration === previous.workspaceContextGeneration &&
        state.sidebarArchivedTasks === previous.sidebarArchivedTasks &&
        state.sidebarStatusSummaryByWorkspaceId === previous.sidebarStatusSummaryByWorkspaceId &&
        state.userSettings.sidebarTaskColors === previous.userSettings.sidebarTaskColors &&
        state.userSettings.sidebarTaskColorAutomation ===
          previous.userSettings.sidebarTaskColorAutomation &&
        state.repositories.itemsByWorkspaceId === previous.repositories.itemsByWorkspaceId &&
        state.kanbanMulti.snapshots === previous.kanbanMulti.snapshots
      )
        return;
      this.synchronize();
    });
  }

  private synchronize() {
    if (this.synchronizing) return this.epoch;
    this.synchronizing = true;
    try {
      const state = this.store.getState();
      const workspaceId = state.workspaces.activeId ?? "";
      const scope = sidebarTaskPageScope(state);
      const rankingKey = sidebarTaskPageRankingKey(state);
      const revision = state.sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId] ?? 0;
      const summaries = state.sidebarStatusSummaryByWorkspaceId?.[workspaceId];
      const oldScope = this.scope,
        oldRankingKey = this.rankingKey,
        oldRevision = this.revision,
        oldSummaries = this.summaries;
      this.scope = scope;
      this.rankingKey = rankingKey;
      this.revision = revision;
      this.summaries = summaries;
      if (scope !== oldScope) this.clear();
      else if (rankingKey !== oldRankingKey) {
        this.clearPages();
        this.cancelRequests();
        this.epoch++;
      } else if (revision !== oldRevision) {
        this.clearPages();
        this.epoch++;
      } else if (summaries !== oldSummaries) {
        for (const [key, entry] of this.pages) {
          if (
            entry.page.entries.some(
              (row) => row.task_id && summaries?.[row.task_id] !== oldSummaries?.[row.task_id],
            )
          )
            this.remove(key);
        }
        this.epoch++;
      }
      for (const [key, entry] of this.pages) {
        if (Date.now() - entry.fetchedAt >= MAX_AGE_MS) this.remove(key);
      }
      this.enforceBudget();
      this.scheduleExpiry();
      return this.epoch;
    } finally {
      this.synchronizing = false;
    }
  }

  private remove(key: string) {
    const entry = this.pages.get(key);
    this.pages.delete(key);
    if (entry) this.store.getState().releaseTaskOverviews?.(entry.owner);
  }

  private clearPages() {
    for (const key of this.pages.keys()) this.remove(key);
    if (this.expiry) clearTimeout(this.expiry);
    this.expiry = undefined;
  }

  private scheduleExpiry() {
    if (this.expiry) clearTimeout(this.expiry);
    this.expiry = undefined;
    if (!this.pages.size) return;
    const oldest = Math.min(...Array.from(this.pages.values(), (entry) => entry.fetchedAt));
    this.expiry = setTimeout(
      () => this.synchronize(),
      Math.max(0, oldest + MAX_AGE_MS - Date.now()),
    );
  }

  private retainedBytes() {
    let bytes = 0;
    const ids = new Set<string>();
    for (const [key, entry] of this.pages) {
      bytes += byteSize([key, entry.page]);
      for (const row of entry.page.entries) if (row.task_id) ids.add(row.task_id);
    }
    const entities = this.store.getState().taskOverview?.byId ?? {};
    for (const id of ids) if (entities[id]) bytes += byteSize(entities[id]);
    return bytes;
  }

  private enforceBudget() {
    while (this.pages.size > MAX_PAGES || (this.pages.size && this.retainedBytes() > MAX_BYTES)) {
      this.remove(this.pages.keys().next().value!);
    }
  }

  forget(key: string) {
    this.remove(key);
  }

  clear() {
    this.epoch++;
    this.clearPages();
    this.cancelRequests();
  }

  private cancelRequests() {
    const requests = [...this.requests.values()];
    this.requests.clear();
    for (const request of requests) {
      request.controller.abort();
      this.releaseRead(request);
    }
  }

  private releaseRead(request: Request) {
    const state = this.store.getState();
    if (request.readId) state.finishTaskOverviewRead?.(request.readId);
    state.releaseTaskOverviews?.(request.owner);
  }

  subscribeDeletedTasks(listener: (taskIds: ReadonlySet<string>) => void) {
    this.deletedTaskListeners.add(listener);
    return () => {
      this.deletedTaskListeners.delete(listener);
    };
  }

  removeTasks(taskIds: ReadonlySet<string>) {
    if (taskIds.size === 0) return;
    this.epoch++;
    this.clearPages();
    for (const listener of this.deletedTaskListeners) listener(taskIds);
  }

  subscribeAccessDenied(listener: () => void) {
    this.accessDeniedListeners.add(listener);
    return () => {
      this.accessDeniedListeners.delete(listener);
    };
  }

  denyAccess() {
    this.clear();
    this.store.getState().denyTaskOverviewAccess?.();
    for (const listener of this.accessDeniedListeners) listener();
  }

  get(key: string): SidebarTaskPageResponse | null {
    const state = this.store.getState();
    const entry = this.pages.get(key);
    const workspaceId = state.workspaces.activeId ?? "";
    if (
      !entry ||
      sidebarTaskPageScope(state) !== this.scope ||
      sidebarTaskPageRankingKey(state) !== this.rankingKey ||
      (state.sidebarArchivedTasks?.revisionByWorkspaceId?.[workspaceId] ?? 0) !== this.revision ||
      Date.now() - entry.fetchedAt >= MAX_AGE_MS
    )
      return null;
    const summaries = state.sidebarStatusSummaryByWorkspaceId?.[workspaceId];
    return entry.page.entries.some(
      (row) => row.task_id && summaries?.[row.task_id] !== this.summaries?.[row.task_id],
    )
      ? null
      : entry.page;
  }

  private retain(key: string, page: SidebarTaskPageResponse) {
    if (page.page !== 1 || page.provisional) return;
    this.remove(key);
    const owner = `sidebar:cache:${generateUUID()}`;
    const state = this.store.getState();
    const tasks = page.entries.flatMap((entry) => {
      const task = entry.task_id ? state.taskOverview?.byId[entry.task_id] : undefined;
      return task ? [task] : [];
    });
    state.retainTaskOverviews?.(owner, tasks);
    this.pages.set(key, { page, owner, fetchedAt: Date.now() });
    this.enforceBudget();
    this.scheduleExpiry();
  }

  private settle(
    request: Request,
    raw: SidebarTaskPageResponse,
    query: SidebarTaskQuery,
    identity: { key: string; epoch: number; scope: string },
  ) {
    const { key, epoch, scope } = identity;
    this.synchronize();
    if (request.controller.signal.aborted || scope !== this.scope)
      return sidebarPageMembership(raw);
    const state = this.store.getState();
    let page = sidebarPageMembership(raw);
    if (request.readId) {
      const reconciled = reconcileSidebarPage(state, raw, query, request.readId);
      if (!reconciled) throw new DOMException("", "AbortError");
      state.retainTaskOverviews(request.owner, reconciled.tasks, request.readId);
      page = { ...reconciled.page, provisional: reconciled.provisional || epoch !== this.epoch };
    } else if (epoch !== this.epoch) page = { ...page, provisional: true };
    if (!page.provisional && epoch === this.epoch) this.retain(key, page);
    return page;
  }

  request(workspaceId: string, query: SidebarTaskQuery, cacheKey: string) {
    const epoch = this.synchronize();
    const scope = this.scope;
    const requestKey = JSON.stringify([cacheKey, query]);
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
        owner: `sidebar:read:${generateUUID()}`,
        readId: this.store.getState().beginTaskOverviewRead?.(),
        promise: querySidebarTasks(workspaceId, query, {
          cache: "no-store",
          init: { signal: controller.signal },
        })
          .then((page) => this.settle(created, page, query, { key: cacheKey, epoch, scope }))
          .finally(() => {
            if (created.readId) this.store.getState().finishTaskOverviewRead?.(created.readId);
            if (this.requests.get(requestKey) === created) this.requests.delete(requestKey);
          }),
      };
      request = created;
      this.requests.set(requestKey, request);
    }
    request.consumers++;
    let released = false;
    return {
      promise: request.promise,
      release: () => {
        if (released) return;
        released = true;
        request.consumers--;
        if (request.consumers !== 0) return;
        if (this.requests.get(requestKey) === request) {
          request.controller.abort();
          this.requests.delete(requestKey);
        }
        this.releaseRead(request);
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
    state.taskOverview?.generation,
  ]);
}

/** Semantic rank inputs stay outside saved-view identity and are reduced to a bounded key. */
export function sidebarTaskPageRankingKey(state: AppState): string {
  const workspaceId = state.workspaces.activeId ?? "";
  const inputs: SidebarRankingInputs = {
    workspaceId,
    colors: state.userSettings.sidebarTaskColors,
    automation: state.userSettings.sidebarTaskColorAutomation,
    repositories: state.repositories.itemsByWorkspaceId[workspaceId] ?? EMPTY_REPOSITORIES,
    snapshots: state.kanbanMulti.snapshots,
  };
  if (
    cachedRanking &&
    cachedRanking.inputs.workspaceId === inputs.workspaceId &&
    cachedRanking.inputs.colors === inputs.colors &&
    cachedRanking.inputs.automation === inputs.automation &&
    cachedRanking.inputs.repositories === inputs.repositories &&
    cachedRanking.inputs.snapshots === inputs.snapshots
  )
    return cachedRanking.key;

  const colors = Object.entries(inputs.colors ?? {})
    .filter(([, color]) => typeof color === "string")
    .sort(([a], [b]) => compareText(a, b));
  const automation = inputs.automation;
  const rules = automation.rules.map((rule) => ({
    enabled: rule.enabled,
    dimension: rule.condition.dimension,
    value: stableSidebarValue(rule.condition.value),
    output: stableSidebarValue(rule.output),
  }));
  const repositories = inputs.repositories
    .map((repository) => repositoryIdentityForSavedRepository(repository))
    .map(stableSidebarValue)
    .sort((a, b) => compareText(JSON.stringify(a), JSON.stringify(b)));
  const stepColors = Object.values(inputs.snapshots ?? {})
    .flatMap((snapshot) => snapshot.steps.map((step) => [step.id, step.color]))
    .sort((a, b) => compareText(JSON.stringify(a), JSON.stringify(b)));
  const key = digestSidebarRanking(
    JSON.stringify([colors, automation.enabled, rules, repositories, stepColors]),
  );
  cachedRanking = { inputs, key };
  return key;
}

function stableSidebarValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(stableSidebarValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(
    Object.entries(value)
      .sort(([a], [b]) => compareText(a, b))
      .map(([key, entry]) => [key, stableSidebarValue(entry)]),
  );
}

function compareText(a: string, b: string): number {
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
}

function digestSidebarRanking(value: string): string {
  let first = 2166136261;
  let second = 2246822519;
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    first = Math.imul(first ^ code, 16777619);
    second = Math.imul(second ^ code, 3266489917);
  }
  return `${(first >>> 0).toString(16).padStart(8, "0")}${(second >>> 0).toString(16).padStart(8, "0")}`;
}
