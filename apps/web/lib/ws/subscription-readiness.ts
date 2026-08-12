export type SubscriptionReadiness = {
  promise: Promise<void>;
  resolve: () => void;
  reject: (reason: unknown) => void;
  requestStarted: boolean;
  settled: boolean;
};

export class SubscriptionReadinessRegistry {
  private entries = new Map<string, SubscriptionReadiness>();

  has(id: string): boolean {
    return this.entries.has(id);
  }

  getOrCreate(id: string, forceNewAfterSettled = false): SubscriptionReadiness {
    const existing = this.entries.get(id);
    if (existing && !(forceNewAfterSettled && existing.settled)) return existing;

    let resolve!: () => void;
    let reject!: (reason: unknown) => void;
    const promise = new Promise<void>((resolvePromise, rejectPromise) => {
      resolve = resolvePromise;
      reject = rejectPromise;
    });
    const readiness: SubscriptionReadiness = {
      promise,
      resolve,
      reject,
      requestStarted: false,
      settled: false,
    };
    // Legacy consumers do not await readiness. Consume failures while returning
    // the original promise to callers that need registration ordering.
    void promise.catch(() => undefined);
    this.entries.set(id, readiness);
    return readiness;
  }

  start(id: string, readiness: SubscriptionReadiness, request: () => Promise<unknown>) {
    if (readiness.requestStarted) return;
    readiness.requestStarted = true;
    void request().then(
      () => {
        if (this.entries.get(id) !== readiness) return;
        readiness.settled = true;
        readiness.resolve();
      },
      (error: unknown) => {
        if (this.entries.get(id) === readiness) {
          this.entries.delete(id);
        }
        readiness.settled = true;
        readiness.reject(error);
      },
    );
  }

  cancel(id: string, releaseError: string) {
    const readiness = this.entries.get(id);
    if (!readiness) return;
    this.entries.delete(id);
    if (!readiness.settled) {
      readiness.settled = true;
      readiness.reject(new Error(releaseError));
    }
  }

  reset(activeIds: Iterable<string>, error: Error, retainActiveSubscriptions = false) {
    const readinessEntries = [...this.entries.values()];
    this.entries.clear();
    for (const readiness of readinessEntries) {
      if (readiness.settled) continue;
      readiness.settled = true;
      readiness.reject(error);
    }
    if (retainActiveSubscriptions) {
      for (const id of activeIds) this.getOrCreate(id);
    }
  }
}
