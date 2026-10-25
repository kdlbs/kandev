import "fake-indexeddb/auto";

import { beforeEach, describe, expect, it, vi } from "vitest";
import { prepareLogEntry, type LogEntry, type PreparedLogEntry } from "./buffer";
import { IndexedDBLogStore, retentionPlan } from "./indexeddb-store";

const DATABASE_NAME = "kandev-diagnostic-logs-v1";
const ENTRIES_STORE = "entries";
const METADATA_STORE = "metadata";
const RETENTION_KEY = "retention";
const MAX_ENTRIES = 10_000;
const MAX_BYTES = 20 * 1024 * 1024;
const THREE_DAYS_MS = 3 * 24 * 60 * 60 * 1000;
const IDENTITY_A = "identity-a";
const IDENTITY_B = "identity-b";
const BEFORE_RECEIPT_MESSAGE = "before-receipt";
const RECEIPT_PREFIX_MESSAGE = "receipt-prefix";

type PersistedTestEntry = {
  id?: number;
  identity_scope: string;
  timestamp_ms: number;
  bytes: number;
  entry: LogEntry;
};

beforeEach(async () => {
  await deleteDatabase();
});

describe("IndexedDB log retention planning and schema", () => {
  it("prunes age first and then oldest records until count and byte caps hold", () => {
    const now = Date.UTC(2026, 6, 30);
    const records = [
      {
        id: 1,
        identity_scope: "a",
        timestamp_ms: now - 4 * 86_400_000,
        bytes: 1,
        entry: {} as never,
      },
      ...Array.from({ length: 10_001 }, (_, index) => ({
        id: index + 2,
        identity_scope: "a",
        timestamp_ms: now - 1_000 + index,
        bytes: 10,
        entry: {} as never,
      })),
    ];
    const result = retentionPlan(records, now);
    expect(result.removeIDs).toEqual([1, 2]);
    expect(result.retainedCount).toBe(10_000);
  });

  it("initializes version two totals without changing the database name", async () => {
    const store = new IndexedDBLogStore();

    await expect(store.snapshot("a")).resolves.toEqual([]);

    const database = await openDatabase();
    expect(database.name).toBe(DATABASE_NAME);
    expect(database.version).toBe(2);
    expect([...database.objectStoreNames]).toEqual([ENTRIES_STORE, METADATA_STORE]);
    database.close();

    await expect(readMetadata()).resolves.toEqual({
      key: RETENTION_KEY,
      count: 0,
      bytes: 0,
    });
  });

  it("rebuilds totals once while upgrading an existing version one database", async () => {
    const now = Date.now();
    const first = logEntry("a", now - 2_000, "first");
    const second = logEntry("b", now - 1_000, "second");
    await createVersionOneDatabase([
      persisted(first, now - 2_000, 11),
      persisted(second, now - 1_000, 22),
    ]);

    const store = new IndexedDBLogStore();
    await expect(store.snapshot("a")).resolves.toEqual([first]);
    await expect(readMetadata()).resolves.toEqual({
      key: RETENTION_KEY,
      count: 2,
      bytes: 33,
    });
  });

  it("rejects a blocked schema upgrade and retries after the older tab closes", async () => {
    const blocker = await openVersionOneDatabase();
    const store = new IndexedDBLogStore();

    try {
      await expect(
        store.append(prepareEntries([logEntry("a", Date.now(), "blocked")])),
      ).rejects.toThrow("IndexedDB upgrade blocked");
    } finally {
      blocker.close();
    }

    await new Promise<void>((resolve) => setTimeout(resolve, 0));
    await expect(
      store.append(prepareEntries([logEntry("a", Date.now(), "retried")])),
    ).resolves.toEqual(expect.any(Array));
    await expect(store.snapshot("a")).resolves.toEqual([
      expect.objectContaining({ message: "retried" }),
    ]);
  });
});

describe("IndexedDB log retention writes", () => {
  it("persists the prepared entry byte count without re-encoding it", async () => {
    const store = new IndexedDBLogStore();
    const prepared = prepareLogEntry(logEntry("a", Date.now(), "prepared"));

    await store.append([prepared]);

    const records = await readEntries();
    expect(records[0]?.bytes).toBe(prepared.bytes);
  });

  it("retains the newest entries and records exact transactional totals", async () => {
    const now = Date.now();
    const store = new IndexedDBLogStore();
    const entries = [
      logEntry("a", now - THREE_DAYS_MS - 1, "expired"),
      ...Array.from({ length: MAX_ENTRIES }, (_, index) =>
        logEntry("a", now - 1_000 + index, `entry-${index}`),
      ),
    ];

    await store.append(prepareEntries(entries));

    const records = await readEntries();
    expect(records).toHaveLength(MAX_ENTRIES);
    expect(records[0].entry.message).toBe("entry-0");
    expect(records.at(-1)?.entry.message).toBe(`entry-${MAX_ENTRIES - 1}`);
    await expect(readMetadata()).resolves.toEqual({
      key: RETENTION_KEY,
      count: MAX_ENTRIES,
      bytes: records.reduce((total, record) => total + record.bytes, 0),
    });
  });

  it("evicts the oldest rows only until the byte cap holds", async () => {
    const now = Date.now();
    const store = new IndexedDBLogStore();
    const entries = Array.from({ length: 4_300 }, (_, index) =>
      logEntry("a", now - 4_300 + index, `${index}-${"x".repeat(5_000)}`),
    );

    await store.append(prepareEntries(entries));

    const records = await readEntries();
    const metadata = await readMetadata();
    expect(metadata.count).toBe(records.length);
    expect(metadata.bytes).toBe(records.reduce((total, record) => total + record.bytes, 0));
    expect(metadata.count).toBeLessThan(MAX_ENTRIES);
    expect(metadata.bytes).toBeLessThanOrEqual(MAX_BYTES);
    expect(records.at(-1)?.entry.message).toContain("4299-");
    expect(records[0].entry.message).not.toContain("0-");
  });

  it("keeps identity snapshots partitioned while sharing the global caps", async () => {
    const store = new IndexedDBLogStore();
    await store.append(
      prepareEntries([
        logEntry(IDENTITY_A, Date.now(), "a"),
        logEntry(IDENTITY_B, Date.now() + 1, "b"),
      ]),
    );

    await expect(store.snapshot(IDENTITY_A)).resolves.toHaveLength(1);
    await expect(store.snapshot(IDENTITY_B)).resolves.toHaveLength(1);
  });
});

describe("IndexedDB log paging", () => {
  it("reads identity snapshots through the chronological timestamp index", async () => {
    const now = Date.now();
    const store = new IndexedDBLogStore();
    const first = logEntry(IDENTITY_A, now, "first");
    const other = logEntry(IDENTITY_B, now, "other");
    const second = logEntry(IDENTITY_A, now, "second");
    await store.append(prepareEntries([first, other, second]));

    const getAll = vi.spyOn(IDBIndex.prototype, "getAll");
    const openCursor = vi.spyOn(IDBIndex.prototype, "openCursor");
    try {
      await expect(store.snapshot(IDENTITY_A)).resolves.toEqual([first, second]);
      expect(getAll).not.toHaveBeenCalled();
      expect(openCursor).toHaveBeenCalled();
    } finally {
      getAll.mockRestore();
      openCursor.mockRestore();
    }
  });

  it("continues pages by timestamp and primary key without gaps or duplicates", async () => {
    const timestamp = Date.now();
    const store = new IndexedDBLogStore();
    const entries = [
      logEntry(IDENTITY_A, timestamp, "first"),
      logEntry(IDENTITY_B, timestamp, "other"),
      logEntry(IDENTITY_A, timestamp, "second"),
      logEntry(IDENTITY_A, timestamp, "third"),
    ];
    await store.append(prepareEntries(entries));

    const messages: string[] = [];
    let cursor: import("./indexeddb-store").LogPageCursor | null = null;
    let pageCount = 0;
    let done = false;
    while (!done) {
      const page = await store.readPage(IDENTITY_A, 1, cursor);
      messages.push(...page.entries.map(({ entry }) => entry.message));
      cursor = page.nextCursor;
      done = page.done;
      pageCount += 1;
    }

    expect(messages).toEqual(["first", "second", "third"]);
    expect(pageCount).toBe(3);
  });
});

describe("IndexedDB cursor continuation bounds", () => {
  it("keeps equal-timestamp cursor visits linear across pages", async () => {
    const timestamp = Date.now();
    const store = new IndexedDBLogStore();
    const entries = Array.from({ length: 64 }, (_, index) =>
      logEntry(IDENTITY_A, timestamp, `entry-${String(index).padStart(2, "0")}`),
    );
    await store.append(prepareEntries(entries));

    const cursorContinue = vi.spyOn(IDBCursor.prototype, "continue");
    const cursorContinuePrimaryKey = vi.spyOn(IDBCursor.prototype, "continuePrimaryKey");
    try {
      let cursor: import("./indexeddb-store").LogPageCursor | null = null;
      let done = false;
      let pageCount = 0;
      while (!done) {
        const page = await store.readPage(IDENTITY_A, prepareLogEntry(entries[0]).bytes, cursor);
        cursor = page.nextCursor;
        done = page.done;
        pageCount += 1;
      }

      expect(pageCount).toBe(entries.length);
      expect(cursorContinuePrimaryKey).toHaveBeenCalledTimes(entries.length - 2);
      expect(cursorContinue.mock.calls.length).toBeLessThanOrEqual(entries.length * 3);
    } finally {
      cursorContinue.mockRestore();
      cursorContinuePrimaryKey.mockRestore();
    }
  });
});

describe("IndexedDB capture boundaries", () => {
  it("excludes records persisted after the capture upper bound", async () => {
    const timestamp = Date.now();
    const store = new IndexedDBLogStore();
    const first = logEntry(IDENTITY_A, timestamp, "first");
    const second = logEntry(IDENTITY_A, timestamp, "second");
    await store.append(prepareEntries([first, second]));

    const maxPrimaryKey = await store.beginCaptureBoundary();
    const firstPage = await store.readPage(
      IDENTITY_A,
      prepareLogEntry(first).bytes,
      null,
      maxPrimaryKey,
    );
    expect(firstPage.entries.map(({ entry }) => entry.message)).toEqual(["first"]);
    expect(firstPage.done).toBe(false);

    await store.append(prepareEntries([logEntry(IDENTITY_A, timestamp, "after-receipt")]));

    const secondPage = await store.readPage(
      IDENTITY_A,
      MAX_BYTES,
      firstPage.nextCursor,
      maxPrimaryKey,
    );
    expect(secondPage.entries.map(({ entry }) => entry.message)).toEqual(["second"]);
    expect(secondPage.done).toBe(true);
  });

  it("freezes the boundary before a later tab commits a row", async () => {
    const firstStore = new IndexedDBLogStore();
    const secondStore = new IndexedDBLogStore();
    const timestamp = Date.now();
    await firstStore.append(
      prepareEntries([logEntry(IDENTITY_A, timestamp, BEFORE_RECEIPT_MESSAGE)]),
    );
    await secondStore.readPage(IDENTITY_A, MAX_BYTES);

    const boundaryPromise = firstStore.beginCaptureBoundary();
    const laterAppend = secondStore.append(
      prepareEntries([logEntry(IDENTITY_A, timestamp, "after-receipt")]),
    );
    const maxPrimaryKey = await boundaryPromise;
    await laterAppend;

    const page = await firstStore.readPage(IDENTITY_A, MAX_BYTES, null, maxPrimaryKey);
    expect(page.entries.map(({ entry }) => entry.message)).toEqual([BEFORE_RECEIPT_MESSAGE]);
    expect(page.done).toBe(true);
  });

  it("excludes another tab write interleaved before the receipt prefix", async () => {
    const firstStore = new IndexedDBLogStore();
    const secondStore = new IndexedDBLogStore();
    const timestamp = Date.now();
    await firstStore.append(
      prepareEntries([logEntry(IDENTITY_A, timestamp, BEFORE_RECEIPT_MESSAGE)]),
    );
    await secondStore.readPage(IDENTITY_A, MAX_BYTES);

    const boundary = await firstStore.beginCaptureBoundary();
    await secondStore.append(
      prepareEntries([logEntry(IDENTITY_A, timestamp, "interleaved-write")]),
    );
    const prefix = await firstStore.append(
      prepareEntries([logEntry(IDENTITY_A, timestamp, RECEIPT_PREFIX_MESSAGE)]),
    );

    const page = await firstStore.readPage(IDENTITY_A, MAX_BYTES, null, boundary, new Set(prefix));
    expect(page.entries.map(({ entry }) => entry.message)).toEqual([
      BEFORE_RECEIPT_MESSAGE,
      RECEIPT_PREFIX_MESSAGE,
    ]);
    expect(page.done).toBe(true);
  });
});

describe("IndexedDB log retention repair and concurrency", () => {
  it("repairs missing or invalid totals during the next write", async () => {
    const store = new IndexedDBLogStore();
    await store.append(prepareEntries([logEntry("a", Date.now(), "before repair")]));
    await overwriteMetadata({ key: RETENTION_KEY, count: -1, bytes: Number.NaN });

    await store.append(prepareEntries([logEntry("a", Date.now() + 1, "after repair")]));

    const records = await readEntries();
    await expect(readMetadata()).resolves.toEqual({
      key: RETENTION_KEY,
      count: records.length,
      bytes: records.reduce((total, record) => total + record.bytes, 0),
    });
  });

  it("clears entries and resets totals in one transaction", async () => {
    const store = new IndexedDBLogStore();
    await store.append(prepareEntries([logEntry("a", Date.now(), "to clear")]));

    await store.clear();

    await expect(readEntries()).resolves.toEqual([]);
    await expect(readMetadata()).resolves.toEqual({
      key: RETENTION_KEY,
      count: 0,
      bytes: 0,
    });
  });

  it("keeps concurrent store instances transactionally consistent", async () => {
    const stores = [new IndexedDBLogStore(), new IndexedDBLogStore()];
    const batches = Array.from({ length: 24 }, (_, batchIndex) =>
      Array.from({ length: 8 }, (_, entryIndex) =>
        logEntry(
          "a",
          Date.now() + batchIndex * 10 + entryIndex,
          `batch-${batchIndex}-${entryIndex}`,
        ),
      ),
    );

    await Promise.all(
      batches.map((batch, index) => stores[index % stores.length].append(prepareEntries(batch))),
    );

    const records = await readEntries();
    const metadata = await readMetadata();
    expect(records).toHaveLength(24 * 8);
    expect(metadata).toEqual({
      key: RETENTION_KEY,
      count: records.length,
      bytes: records.reduce((total, record) => total + record.bytes, 0),
    });
  });

  it("uses only the bounded timestamp cursor for a within-limit append", async () => {
    const store = new IndexedDBLogStore();
    await store.append(prepareEntries([logEntry("a", Date.now(), "first")]));

    const objectStoreCursor = vi.spyOn(IDBObjectStore.prototype, "openCursor");
    const timestampCursor = vi.spyOn(IDBIndex.prototype, "openCursor");

    try {
      await store.append(prepareEntries([logEntry("a", Date.now() + 1, "second")]));

      expect(objectStoreCursor).not.toHaveBeenCalled();
      expect(timestampCursor).toHaveBeenCalledTimes(1);
      expect(timestampCursor.mock.calls[0]?.[0]).toBeInstanceOf(IDBKeyRange);
      expect(timestampCursor.mock.calls[0]?.[0]).toMatchObject({
        upper: expect.any(Number),
      });
    } finally {
      objectStoreCursor.mockRestore();
      timestampCursor.mockRestore();
    }
  });
});

function logEntry(identityScope: string, timestamp: number, message: string): LogEntry {
  return {
    timestamp: new Date(timestamp).toISOString(),
    level: "info",
    source: "test",
    message,
    identity_scope: identityScope,
  };
}

function prepareEntries(entries: LogEntry[]): PreparedLogEntry[] {
  return entries.map(prepareLogEntry);
}

function persisted(entry: LogEntry, timestamp: number, bytes: number): PersistedTestEntry {
  return {
    identity_scope: entry.identity_scope ?? "",
    timestamp_ms: timestamp,
    bytes,
    entry,
  };
}

function deleteDatabase(): Promise<void> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.deleteDatabase(DATABASE_NAME);
    request.onsuccess = () => resolve();
    request.onerror = () => reject(request.error ?? new Error("failed to delete test database"));
  });
}

function openDatabase(version?: number): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request =
      version === undefined
        ? indexedDB.open(DATABASE_NAME)
        : indexedDB.open(DATABASE_NAME, version);
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("failed to open test database"));
  });
}

function createVersionOneDatabase(records: PersistedTestEntry[]): Promise<void> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE_NAME, 1);
    request.onupgradeneeded = () => {
      const store = request.result.createObjectStore(ENTRIES_STORE, {
        keyPath: "id",
        autoIncrement: true,
      });
      store.createIndex("identity_scope", "identity_scope", { unique: false });
      store.createIndex("timestamp_ms", "timestamp_ms", { unique: false });
    };
    request.onerror = () =>
      reject(request.error ?? new Error("failed to create version one database"));
    request.onsuccess = () => {
      const database = request.result;
      const transaction = database.transaction(ENTRIES_STORE, "readwrite");
      for (const record of records) transaction.objectStore(ENTRIES_STORE).add(record);
      transaction.oncomplete = () => {
        database.close();
        resolve();
      };
      transaction.onerror = () =>
        reject(transaction.error ?? new Error("failed to seed version one database"));
    };
  });
}

function openVersionOneDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE_NAME, 1);
    request.onupgradeneeded = () => {
      const store = request.result.createObjectStore(ENTRIES_STORE, {
        keyPath: "id",
        autoIncrement: true,
      });
      store.createIndex("identity_scope", "identity_scope", { unique: false });
      store.createIndex("timestamp_ms", "timestamp_ms", { unique: false });
    };
    request.onerror = () =>
      reject(request.error ?? new Error("failed to open version one blocker"));
    request.onsuccess = () => resolve(request.result);
  });
}

async function readEntries(): Promise<PersistedTestEntry[]> {
  const database = await openDatabase();
  const transaction = database.transaction(ENTRIES_STORE, "readonly");
  const request = transaction.objectStore(ENTRIES_STORE).getAll();
  const records = await requestResult(request);
  await transactionComplete(transaction);
  database.close();
  return records as PersistedTestEntry[];
}

async function readMetadata(): Promise<Record<string, unknown>> {
  const database = await openDatabase();
  const transaction = database.transaction(METADATA_STORE, "readonly");
  const value = await requestResult(transaction.objectStore(METADATA_STORE).get(RETENTION_KEY));
  await transactionComplete(transaction);
  database.close();
  return value as Record<string, unknown>;
}

async function overwriteMetadata(value: Record<string, unknown>): Promise<void> {
  const database = await openDatabase();
  const transaction = database.transaction(METADATA_STORE, "readwrite");
  transaction.objectStore(METADATA_STORE).put(value);
  await transactionComplete(transaction);
  database.close();
}

function requestResult<T>(request: IDBRequest<T>): Promise<T> {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("test request failed"));
  });
}

function transactionComplete(transaction: IDBTransaction): Promise<void> {
  return new Promise((resolve, reject) => {
    transaction.oncomplete = () => resolve();
    transaction.onerror = () => reject(transaction.error ?? new Error("test transaction failed"));
    transaction.onabort = () => reject(transaction.error ?? new Error("test transaction aborted"));
  });
}
