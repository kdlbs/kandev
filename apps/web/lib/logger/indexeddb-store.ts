import type { LogEntry } from "./buffer";

const DATABASE_NAME = "kandev-diagnostic-logs-v1";
const STORE_NAME = "entries";
const THREE_DAYS_MS = 3 * 24 * 60 * 60 * 1000;
const MAX_ENTRIES = 10_000;
const MAX_BYTES = 20 * 1024 * 1024;

type PersistedEntry = {
  id?: number;
  identity_scope: string;
  timestamp_ms: number;
  bytes: number;
  entry: LogEntry;
};

export class IndexedDBLogStore {
  private database: Promise<IDBDatabase> | null = null;

  async append(entries: LogEntry[]): Promise<void> {
    if (entries.length === 0) return;
    const database = await this.open();
    const transaction = database.transaction(STORE_NAME, "readwrite");
    const store = transaction.objectStore(STORE_NAME);
    for (const entry of entries) {
      const identity = entry.identity_scope;
      if (!identity) continue;
      const serialized = JSON.stringify(entry);
      store.add({
        identity_scope: identity,
        timestamp_ms: Date.parse(entry.timestamp) || Date.now(),
        bytes: new TextEncoder().encode(serialized).byteLength,
        entry,
      } satisfies PersistedEntry);
    }
    await transactionDone(transaction);
    await this.prune();
  }

  async snapshot(identityScope: string): Promise<LogEntry[]> {
    const database = await this.open();
    const transaction = database.transaction(STORE_NAME, "readonly");
    const index = transaction.objectStore(STORE_NAME).index("identity_scope");
    const records = await requestResult<PersistedEntry[]>(
      index.getAll(IDBKeyRange.only(identityScope)),
    );
    await transactionDone(transaction);
    const cutoff = Date.now() - THREE_DAYS_MS;
    return records
      .filter((record) => record.timestamp_ms >= cutoff)
      .sort((left, right) => left.timestamp_ms - right.timestamp_ms)
      .map((record) => record.entry);
  }

  async clear(): Promise<void> {
    const database = await this.open();
    const transaction = database.transaction(STORE_NAME, "readwrite");
    transaction.objectStore(STORE_NAME).clear();
    await transactionDone(transaction);
  }

  private open(): Promise<IDBDatabase> {
    if (this.database) return this.database;
    if (typeof indexedDB === "undefined") return Promise.reject(new Error("IndexedDB unavailable"));
    this.database = new Promise((resolve, reject) => {
      const request = indexedDB.open(DATABASE_NAME, 1);
      request.onupgradeneeded = () => {
        const store = request.result.createObjectStore(STORE_NAME, {
          keyPath: "id",
          autoIncrement: true,
        });
        store.createIndex("identity_scope", "identity_scope", { unique: false });
        store.createIndex("timestamp_ms", "timestamp_ms", { unique: false });
      };
      request.onsuccess = () => resolve(request.result);
      request.onerror = () => reject(request.error ?? new Error("IndexedDB open failed"));
    });
    return this.database;
  }

  private async prune(): Promise<void> {
    const database = await this.open();
    const transaction = database.transaction(STORE_NAME, "readwrite");
    const store = transaction.objectStore(STORE_NAME);
    const index = store.index("timestamp_ms");
    const totals = await scanAndDeleteExpired(index, Date.now() - THREE_DAYS_MS);
    await deleteOldestUntilWithinBounds(index, totals);
    await transactionDone(transaction);
  }
}

type RetentionTotals = { count: number; bytes: number };

function scanAndDeleteExpired(index: IDBIndex, cutoff: number): Promise<RetentionTotals> {
  return new Promise((resolve, reject) => {
    const totals = { count: 0, bytes: 0 };
    const request = index.openCursor();
    request.onerror = () => reject(request.error ?? new Error("IndexedDB cursor failed"));
    request.onsuccess = () => {
      const cursor = request.result;
      if (!cursor) {
        resolve(totals);
        return;
      }
      const record = cursor.value as PersistedEntry;
      if (record.timestamp_ms < cutoff) {
        cursor.delete();
      } else {
        totals.count += 1;
        totals.bytes += record.bytes;
      }
      cursor.continue();
    };
  });
}

function deleteOldestUntilWithinBounds(index: IDBIndex, totals: RetentionTotals): Promise<void> {
  if (totals.count <= MAX_ENTRIES && totals.bytes <= MAX_BYTES) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const request = index.openCursor();
    request.onerror = () => reject(request.error ?? new Error("IndexedDB cursor failed"));
    request.onsuccess = () => {
      const cursor = request.result;
      if (!cursor || (totals.count <= MAX_ENTRIES && totals.bytes <= MAX_BYTES)) {
        resolve();
        return;
      }
      const record = cursor.value as PersistedEntry;
      cursor.delete();
      totals.count -= 1;
      totals.bytes -= record.bytes;
      cursor.continue();
    };
  });
}

export function retentionPlan(
  records: PersistedEntry[],
  now: number,
): { removeIDs: number[]; retainedCount: number; retainedBytes: number } {
  const cutoff = now - THREE_DAYS_MS;
  const ordered = [...records].sort((left, right) => left.timestamp_ms - right.timestamp_ms);
  let count = ordered.length;
  let bytes = ordered.reduce((total, record) => total + record.bytes, 0);
  const removeIDs: number[] = [];
  for (const record of ordered) {
    if (record.id === undefined) continue;
    if (record.timestamp_ms >= cutoff && count <= MAX_ENTRIES && bytes <= MAX_BYTES) break;
    removeIDs.push(record.id);
    count -= 1;
    bytes -= record.bytes;
  }
  return { removeIDs, retainedCount: count, retainedBytes: bytes };
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("IndexedDB request failed"));
  });
}

function transactionDone(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onerror = () =>
      reject(transaction.error ?? new Error("IndexedDB transaction failed"));
    transaction.onabort = () =>
      reject(transaction.error ?? new Error("IndexedDB transaction aborted"));
  });
}
