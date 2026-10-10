import { getBackendConfig } from "@/lib/config";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";

type SharedRead<T> = (signal: AbortSignal) => Promise<T>;

type SharedReadRecord<T> = {
  consumers: number;
  controller?: AbortController;
  request?: Promise<T>;
  trailingRefresh?: Promise<T>;
  refreshQueued: boolean;
};

/** Coalesces reads by resource key and cancels them after their final consumer leaves. */
export class SharedResourceReads<T> {
  private records = new Map<string, SharedReadRecord<T>>();

  constructor(private readonly scopeIsCurrent: () => boolean = () => true) {}

  retain(key: string): () => void {
    const record = this.record(key);
    record.consumers++;
    let released = false;
    return () => {
      if (released) return;
      released = true;
      record.consumers = Math.max(0, record.consumers - 1);
      if (record.consumers === 0 && this.records.get(key) === record) {
        record.refreshQueued = false;
        this.records.delete(key);
        record.controller?.abort();
      }
    };
  }

  read(key: string, load: SharedRead<T>, options: { refresh?: boolean } = {}): Promise<T> {
    if (!this.scopeIsCurrent()) return Promise.reject(abortError());
    const record = this.record(key);
    if (record.request) {
      if (options.refresh) return this.queueRefresh(key, record, load, record.request);
      return record.request;
    }
    return this.start(key, record, load);
  }

  isCurrent(key?: string): boolean {
    return (
      this.scopeIsCurrent() && (key === undefined || (this.records.get(key)?.consumers ?? 0) > 0)
    );
  }

  isReading(key: string): boolean {
    return this.scopeIsCurrent() && this.records.get(key)?.request !== undefined;
  }

  dispose() {
    for (const record of this.records.values()) record.controller?.abort();
    this.records.clear();
  }

  private record(key: string): SharedReadRecord<T> {
    let record = this.records.get(key);
    if (!record) {
      record = { consumers: 0, refreshQueued: false };
      this.records.set(key, record);
    }
    return record;
  }

  private start(key: string, record: SharedReadRecord<T>, load: SharedRead<T>): Promise<T> {
    const controller = new AbortController();
    record.controller = controller;
    let result: Promise<T>;
    try {
      result = Promise.resolve(load(controller.signal));
    } catch (error) {
      result = Promise.reject(error);
    }
    const attemptIsCurrent = () =>
      this.scopeIsCurrent() &&
      this.records.get(key) === record &&
      record.controller === controller &&
      !controller.signal.aborted;
    const request = result
      .then(
        (value) => {
          if (!attemptIsCurrent()) throw abortError();
          return value;
        },
        (error) => {
          if (!attemptIsCurrent()) throw abortError();
          throw error;
        },
      )
      .finally(() => {
        if (record.request === request) record.request = undefined;
        if (record.controller === controller) record.controller = undefined;
      });
    record.request = request;
    return request;
  }

  private queueRefresh(
    key: string,
    record: SharedReadRecord<T>,
    load: SharedRead<T>,
    activeRequest: Promise<T>,
  ): Promise<T> {
    record.refreshQueued = true;
    if (record.trailingRefresh) return record.trailingRefresh;
    const trailing = activeRequest
      .catch(() => undefined)
      .then(async () => {
        let latest: T | undefined;
        let lastError: unknown;
        while (
          record.refreshQueued &&
          record.consumers > 0 &&
          this.scopeIsCurrent() &&
          this.records.get(key) === record
        ) {
          record.refreshQueued = false;
          try {
            latest = await this.start(key, record, load);
            lastError = undefined;
          } catch (error) {
            lastError = error;
          }
        }
        if (!this.scopeIsCurrent() || this.records.get(key) !== record) throw abortError();
        if (lastError !== undefined) throw lastError;
        if (latest !== undefined) return latest;
        throw abortError();
      })
      .finally(() => {
        if (record.trailingRefresh === trailing) record.trailingRefresh = undefined;
      });
    record.trailingRefresh = trailing;
    return trailing;
  }
}

export function storeReadScopeIdentity(store: StoreApi<AppState>): string {
  return stateReadScopeIdentity(store.getState());
}

export function stateReadScopeIdentity(state: AppState): string {
  return JSON.stringify([
    getBackendConfig().apiBaseUrl,
    state?.auth?.mode ?? "optional",
    state?.auth?.authenticated ?? false,
    state?.auth?.user?.id ?? null,
    state?.workspaceContextGeneration ?? 0,
  ]);
}

export function raceSharedRead<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<T>((resolve, reject) => {
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error) => {
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

function abortError() {
  return new DOMException("The operation was aborted", "AbortError");
}
