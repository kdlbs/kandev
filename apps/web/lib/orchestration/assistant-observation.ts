import { ApiError } from "@/lib/api/client";
import {
  getAssistant,
  type AssistantBinding,
  type AssistantPage,
} from "@/lib/api/domains/assistant-api";

type BindingSnapshot = {
  binding: AssistantBinding | null | undefined;
  loading: boolean;
  error: unknown;
  revision: number;
};
const initialBinding = (): BindingSnapshot => ({
  binding: undefined,
  loading: false,
  error: null,
  revision: 0,
});
export class AssistantObservation {
  private snapshot = initialBinding();
  private listeners = new Set<() => void>();
  private generation = 0;
  private disposed = false;
  private request?: AbortController;
  constructor(private load = getAssistant) {}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  activate = () => {
    this.disposed = false;
  };
  dispose = () => {
    this.disposed = true;
    ++this.generation;
    this.request?.abort();
    this.snapshot = initialBinding();
  };
  private update(patch: Partial<BindingSnapshot>) {
    this.snapshot = { ...this.snapshot, ...patch };
    this.listeners.forEach((listener) => listener());
  }
  refresh = async () => {
    if (this.disposed) return;
    this.request?.abort();
    const controller = new AbortController();
    this.request = controller;
    const generation = ++this.generation;
    this.update({ loading: true });
    try {
      const binding = await this.load(controller.signal);
      if (this.disposed || generation !== this.generation) return;
      this.update({ binding, error: null, revision: this.snapshot.revision + 1 });
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const denied = error instanceof ApiError && [401, 403, 404].includes(error.status);
      this.update({ error, ...(denied ? { binding: undefined } : {}) });
    } finally {
      if (!this.disposed && generation === this.generation) this.update({ loading: false });
    }
  };
}

type CollectionSnapshot<T> = {
  entries: T[];
  nextCursor: string;
  loading: boolean;
  loaded: boolean;
  error: unknown;
};
const initialCollection = <T>(): CollectionSnapshot<T> => ({
  entries: [],
  nextCursor: "",
  loading: false,
  loaded: false,
  error: null,
});
export class AssistantCollection<T extends { id: string }> {
  private snapshot = initialCollection<T>();
  private listeners = new Set<() => void>();
  private generation = 0;
  private disposed = false;
  private request?: AbortController;
  private pages = 1;
  constructor(private load: (after: string, signal: AbortSignal) => Promise<AssistantPage<T>>) {}
  getSnapshot = () => this.snapshot;
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  activate = () => {
    this.disposed = false;
  };
  dispose = () => {
    this.disposed = true;
    ++this.generation;
    this.request?.abort();
    this.snapshot = initialCollection<T>();
  };
  private update(patch: Partial<CollectionSnapshot<T>>) {
    this.snapshot = { ...this.snapshot, ...patch };
    this.listeners.forEach((listener) => listener());
  }
  private async readWindow(signal: AbortSignal) {
    const entries = new Map<string, T>();
    let after = "";
    for (let page = 0; page < this.pages; page++) {
      const result = await this.load(after, signal);
      for (const entry of result.entries) entries.set(entry.id, entry);
      after = result.next_cursor;
      if (!after) break;
    }
    return { entries: [...entries.values()], nextCursor: after };
  }
  refresh = async () => {
    if (this.disposed) return;
    this.request?.abort();
    const controller = new AbortController();
    this.request = controller;
    const generation = ++this.generation;
    this.update({ loading: true });
    try {
      const result = await this.readWindow(controller.signal);
      if (this.disposed || generation !== this.generation) return;
      this.update({ ...result, loaded: true, error: null });
    } catch (error) {
      if (this.disposed || generation !== this.generation) return;
      const denied = error instanceof ApiError && [401, 403, 404, 409].includes(error.status);
      if (denied) {
        this.pages = 1;
        this.update(initialCollection<T>());
      }
      this.update({ error });
    } finally {
      if (!this.disposed && generation === this.generation) this.update({ loading: false });
    }
  };
  loadMore = async () => {
    if (this.snapshot.loading || !this.snapshot.nextCursor) return;
    ++this.pages;
    await this.refresh();
  };
}
