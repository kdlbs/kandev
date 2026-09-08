import type { LogEntry, PreparedLogEntry } from "./buffer";

const DATABASE_NAME = "kandev-diagnostic-logs-v1";
const DATABASE_VERSION = 2;
const STORE_NAME = "entries";
const METADATA_STORE_NAME = "metadata";
const RETENTION_METADATA_KEY = "retention";
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

type RetentionTotals = { count: number; bytes: number };

type RetentionMetadata = RetentionTotals & { key: string };

const EMPTY_TOTALS: RetentionTotals = { count: 0, bytes: 0 };

export class IndexedDBLogStore {
  private database: Promise<IDBDatabase> | null = null;

  async append(entries: readonly PreparedLogEntry[]): Promise<void> {
    const persistedEntries = entries.flatMap(({ entry, bytes }) => {
      const identity = entry.identity_scope;
      if (!identity) return [];

      const timestamp = Date.parse(entry.timestamp);
      return [
        {
          identity_scope: identity,
          timestamp_ms: Number.isNaN(timestamp) ? Date.now() : timestamp,
          bytes,
          entry,
        } satisfies PersistedEntry,
      ];
    });
    if (persistedEntries.length === 0) return;

    const database = await this.open();
    const transaction = database.transaction([STORE_NAME, METADATA_STORE_NAME], "readwrite");
    const store = transaction.objectStore(STORE_NAME);
    const metadataStore = transaction.objectStore(METADATA_STORE_NAME);
    const storedMetadata = await requestResult<RetentionMetadata | undefined>(
      metadataStore.get(RETENTION_METADATA_KEY),
    );
    const totals = isValidTotals(storedMetadata)
      ? { count: storedMetadata.count, bytes: storedMetadata.bytes }
      : await scanTotals(store);

    for (const entry of persistedEntries) {
      store.add(entry);
      totals.count += 1;
      totals.bytes += entry.bytes;
    }

    const cutoff = Date.now() - THREE_DAYS_MS;
    await deleteExpiredPrefix(store.index("timestamp_ms"), cutoff, totals);
    await deleteOldestUntilWithinBounds(store.index("timestamp_ms"), totals);
    metadataStore.put({ key: RETENTION_METADATA_KEY, ...totals } satisfies RetentionMetadata);
    await transactionDone(transaction);
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
    const transaction = database.transaction([STORE_NAME, METADATA_STORE_NAME], "readwrite");
    transaction.objectStore(STORE_NAME).clear();
    const metadataStore = transaction.objectStore(METADATA_STORE_NAME);
    metadataStore.clear();
    metadataStore.put({ key: RETENTION_METADATA_KEY, ...EMPTY_TOTALS } satisfies RetentionMetadata);
    await transactionDone(transaction);
  }

  private open(): Promise<IDBDatabase> {
    if (this.database) return this.database;
    if (typeof indexedDB === "undefined") return Promise.reject(new Error("IndexedDB unavailable"));

    let failed = false;
    const databasePromise = new Promise<IDBDatabase>((resolve, reject) => {
      const fail = (error: Error) => {
        if (failed) return;
        failed = true;
        if (this.database === databasePromise) this.database = null;
        reject(error);
      };
      const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION);
      request.onupgradeneeded = () => {
        const database = request.result;
        const transaction = request.transaction;
        if (!transaction) {
          fail(new Error("IndexedDB upgrade transaction unavailable"));
          return;
        }

        const entries = database.objectStoreNames.contains(STORE_NAME)
          ? transaction.objectStore(STORE_NAME)
          : createEntriesStore(database);
        const metadata = database.objectStoreNames.contains(METADATA_STORE_NAME)
          ? transaction.objectStore(METADATA_STORE_NAME)
          : database.createObjectStore(METADATA_STORE_NAME, { keyPath: "key" });

        rebuildTotalsDuringUpgrade(entries, metadata);
      };
      request.onblocked = () => fail(new Error("IndexedDB upgrade blocked"));
      request.onsuccess = () => {
        const database = request.result;
        if (failed) {
          database.close();
          return;
        }
        database.onversionchange = () => database.close();
        resolve(database);
      };
      request.onerror = () => fail(request.error ?? new Error("IndexedDB open failed"));
    });
    this.database = databasePromise;
    return databasePromise;
  }
}

function createEntriesStore(database: IDBDatabase): IDBObjectStore {
  const store = database.createObjectStore(STORE_NAME, {
    keyPath: "id",
    autoIncrement: true,
  });
  store.createIndex("identity_scope", "identity_scope", { unique: false });
  store.createIndex("timestamp_ms", "timestamp_ms", { unique: false });
  return store;
}

function rebuildTotalsDuringUpgrade(entries: IDBObjectStore, metadata: IDBObjectStore): void {
  const totals = { ...EMPTY_TOTALS };
  const request = entries.openCursor();
  request.onerror = () => request.transaction?.abort();
  request.onsuccess = () => {
    const cursor = request.result;
    if (!cursor) {
      metadata.put({ key: RETENTION_METADATA_KEY, ...totals } satisfies RetentionMetadata);
      return;
    }
    const record = cursor.value as PersistedEntry;
    totals.count += 1;
    totals.bytes += record.bytes;
    cursor.continue();
  };
}

function isValidTotals(value: RetentionMetadata | undefined): value is RetentionMetadata {
  return (
    value?.key === RETENTION_METADATA_KEY &&
    Number.isSafeInteger(value.count) &&
    value.count >= 0 &&
    Number.isFinite(value.bytes) &&
    value.bytes >= 0
  );
}

function scanTotals(store: IDBObjectStore): Promise<RetentionTotals> {
  return new Promise((resolve, reject) => {
    const totals = { ...EMPTY_TOTALS };
    const request = store.openCursor();
    request.onerror = () => reject(request.error ?? new Error("IndexedDB cursor failed"));
    request.onsuccess = () => {
      const cursor = request.result;
      if (!cursor) {
        resolve(totals);
        return;
      }
      const record = cursor.value as PersistedEntry;
      totals.count += 1;
      totals.bytes += record.bytes;
      cursor.continue();
    };
  });
}

function deleteExpiredPrefix(
  index: IDBIndex,
  cutoff: number,
  totals: RetentionTotals,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const request = index.openCursor(IDBKeyRange.upperBound(cutoff, true));
    request.onerror = () => reject(request.error ?? new Error("IndexedDB cursor failed"));
    request.onsuccess = () => {
      const cursor = request.result;
      if (!cursor) {
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
