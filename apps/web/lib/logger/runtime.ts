import {
  getLogBuffer,
  prepareLogEntry,
  snapshotPreparedLogs,
  type LogEntry,
  type LogLevel,
  type PreparedLogEntry,
} from "./buffer";
import { IndexedDBLogStore, type LogPage, type LogPageCursor } from "./indexeddb-store";

const DRAIN_ENTRY_LIMIT = 50;
const DRAIN_BYTE_LIMIT = 256 * 1024;
const STAGING_ENTRY_LIMIT = 500;
const STAGING_BYTE_LIMIT = 2 * 1024 * 1024;
const COLLECTION_WINDOW_MS = 250;
const IDLE_DEADLINE_MS = 1_000;
const CAPTURE_FLUSH_TIMEOUT_MS = 1_000;
const POST_COLLECTION_IDLE_TIMEOUT_MS = IDLE_DEADLINE_MS - COLLECTION_WINDOW_MS;
const SNAPSHOT_PAGE_BYTES = 20 * 1024 * 1024;

type Staged = PreparedLogEntry & { sequence: number };
type DrainResult = { completed: boolean; primaryKeys: number[] };
type CaptureBoundaryResult = { proven: boolean; maxPrimaryKey: number | null };

export type BrowserLogCapture = {
  storageMode: "indexeddb" | "memory";
  flushTimeout: boolean;
  memoryEntries: PreparedLogEntry[];
  readPage: ((maxBytes: number, after: LogPageCursor | null) => Promise<LogPage>) | null;
};

const store = new IndexedDBLogStore();
let identityScope: string | null = "default-user";
let staging: Staged[] = [];
let stagingBytes = 0;
let collectionTimer: ReturnType<typeof setTimeout> | null = null;
let idleCallbackHandle: number | null = null;
let idleFallbackTimer: ReturnType<typeof setTimeout> | null = null;
let idleGeneration = 0;
let drainPromise: Promise<DrainResult> | null = null;
let nextSequence = 0;
let storageMode: "indexeddb" | "memory" = "indexeddb";
let persistenceFailures = 0;
let stagingDropped = 0;

export function setLogIdentity(scope: string | null): void {
  identityScope = scope;
}

export function stageLogEntry(entry: Omit<LogEntry, "identity_scope">): void {
  const scoped = { ...entry, identity_scope: identityScope ?? undefined };
  const prepared = prepareLogEntry(scoped);
  if (!getLogBuffer().pushPrepared(prepared)) return;
  if (!identityScope || storageMode === "memory") return;
  if (!makeStagingRoom(prepared.entry.level, prepared.bytes)) {
    stagingDropped += 1;
    return;
  }
  staging.push({ ...prepared, sequence: ++nextSequence });
  stagingBytes += prepared.bytes;
  scheduleDrain();
}

export async function beginBrowserLogCapture(scope: string): Promise<BrowserLogCapture> {
  const watermark = nextSequence;
  const memoryEntries = snapshotPreparedLogs(scope);
  const boundaryPromise = captureBoundary();
  let flushResult: DrainResult | undefined;
  let boundaryResult: CaptureBoundaryResult | undefined;
  const flushAndBoundary = Promise.all([
    flushStaging(watermark).then((result) => {
      flushResult = result;
    }),
    boundaryPromise.then((result) => {
      boundaryResult = result;
    }),
  ]);
  const completed = await waitForCaptureFlush(flushAndBoundary);
  const completedFlush = flushResult;
  const completedBoundary = boundaryResult;
  if (
    !completed ||
    storageMode === "memory" ||
    !completedFlush ||
    !completedFlush.completed ||
    !completedBoundary ||
    !completedBoundary.proven
  ) {
    return { storageMode: "memory", flushTimeout: !completed, memoryEntries, readPage: null };
  }
  const receiptPrimaryKeys = new Set(completedFlush.primaryKeys);
  return {
    storageMode: "indexeddb",
    flushTimeout: false,
    memoryEntries,
    readPage: (maxBytes, after) =>
      store.readPage(scope, maxBytes, after, completedBoundary.maxPrimaryKey, receiptPrimaryKeys),
  };
}

export async function snapshotBrowserLogs(scope: string): Promise<LogEntry[]> {
  const capture = await beginBrowserLogCapture(scope);
  if (capture.storageMode === "memory") {
    return capture.memoryEntries.map(({ entry }) => entry);
  }
  try {
    return await readCapturePages(capture);
  } catch {
    degradePersistence();
    return capture.memoryEntries.map(({ entry }) => entry);
  }
}

export function browserLogMetadata(): Record<string, unknown> {
  return {
    storage_mode: storageMode,
    persistence_failures: persistenceFailures,
    staging_dropped: stagingDropped,
    memory_loss: getLogBuffer().statistics(),
  };
}

export function browserInstallationID(): string {
  const key = "kandev-diagnostic-browser-id";
  try {
    const existing = localStorage.getItem(key);
    if (existing) return existing;
    const created = randomID();
    localStorage.setItem(key, created);
    return created;
  } catch {
    return randomID();
  }
}

function makeStagingRoom(level: LogLevel, bytes: number): boolean {
  while (staging.length >= STAGING_ENTRY_LIMIT || stagingBytes + bytes > STAGING_BYTE_LIMIT) {
    const low = staging.findIndex(
      (candidate) => candidate.entry.level === "debug" || candidate.entry.level === "info",
    );
    let index = low;
    if (index < 0) index = level === "warn" || level === "error" ? 0 : -1;
    if (index < 0) return false;
    const [removed] = staging.splice(index, 1);
    stagingBytes -= removed.bytes;
    stagingDropped += 1;
  }
  return true;
}

function scheduleDrain(): void {
  if (
    collectionTimer !== null ||
    idleCallbackHandle !== null ||
    idleFallbackTimer !== null ||
    drainPromise
  ) {
    return;
  }
  collectionTimer = setTimeout(() => {
    collectionTimer = null;
    schedulePostWindowDrain();
  }, COLLECTION_WINDOW_MS);
}

function schedulePostWindowDrain(): void {
  if (drainPromise || storageMode === "memory" || staging.length === 0) return;
  if (typeof requestIdleCallback !== "function") {
    void requestDrain();
    return;
  }

  const generation = ++idleGeneration;
  const drain = () => {
    if (generation !== idleGeneration) return;
    idleCallbackHandle = null;
    cancelIdleFallbackTimer();
    void requestDrain();
  };
  idleCallbackHandle = requestIdleCallback(drain, {
    timeout: POST_COLLECTION_IDLE_TIMEOUT_MS,
  });
  idleFallbackTimer = setTimeout(() => {
    if (generation !== idleGeneration) return;
    idleFallbackTimer = null;
    cancelIdleCallback();
    void requestDrain();
  }, POST_COLLECTION_IDLE_TIMEOUT_MS);
}

function requestDrain(maxSequence = nextSequence): Promise<DrainResult> {
  if (drainPromise) return drainPromise;
  if (storageMode === "memory" || !hasStagedThrough(maxSequence)) {
    return Promise.resolve(emptyDrainResult());
  }
  drainPromise = drainLoop(maxSequence).finally(() => {
    drainPromise = null;
    if (storageMode === "indexeddb" && staging.length > 0) scheduleDrain();
  });
  return drainPromise;
}

async function drainLoop(maxSequence: number): Promise<DrainResult> {
  let primaryKeys: number[] = [];
  while (storageMode === "indexeddb" && hasStagedThrough(maxSequence)) {
    const result = await drainBatch(maxSequence);
    primaryKeys = primaryKeys.concat(result.primaryKeys);
    if (!result.completed) return { completed: false, primaryKeys };
  }
  return { completed: true, primaryKeys };
}

async function drainBatch(maxSequence: number): Promise<DrainResult> {
  if (storageMode === "memory" || !hasStagedThrough(maxSequence)) {
    return emptyDrainResult();
  }
  const batch: Staged[] = [];
  let bytes = 0;
  while (
    staging.length > 0 &&
    staging[0].sequence <= maxSequence &&
    batch.length < DRAIN_ENTRY_LIMIT
  ) {
    const next = staging[0];
    if (batch.length > 0 && bytes + next.bytes > DRAIN_BYTE_LIMIT) break;
    staging.shift();
    stagingBytes -= next.bytes;
    batch.push(next);
    bytes += next.bytes;
  }
  try {
    const persistedPrimaryKey = await store.append(batch);
    return {
      completed: true,
      primaryKeys: normalizePrimaryKeys(persistedPrimaryKey),
    };
  } catch {
    for (const item of batch.reverse()) {
      staging.unshift(item);
      stagingBytes += item.bytes;
    }
    degradePersistence();
    return { completed: false, primaryKeys: [] };
  }
}

function hasStagedThrough(maxSequence: number): boolean {
  return staging.some((item) => item.sequence <= maxSequence);
}

async function flushStaging(maxSequence = nextSequence): Promise<DrainResult> {
  cancelCollectionTimer();
  cancelIdleCallback();
  let result = emptyDrainResult();
  while (drainPromise || (storageMode === "indexeddb" && hasStagedThrough(maxSequence))) {
    const drainResult = await requestDrain(maxSequence);
    result = mergeDrainResults(result, drainResult);
    if (!drainResult.completed) break;
  }
  return result;
}

function waitForCaptureFlush(flush: Promise<unknown>): Promise<boolean> {
  return new Promise((resolve) => {
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      resolve(false);
    }, CAPTURE_FLUSH_TIMEOUT_MS);
    const finish = (completed: boolean) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(completed);
    };
    flush.then(
      () => finish(true),
      () => finish(true),
    );
  });
}

function captureBoundary(): Promise<CaptureBoundaryResult> {
  if (storageMode === "memory") return Promise.resolve(unprovenBoundary());
  try {
    return store.beginCaptureBoundary().then(
      (maxPrimaryKey) => ({ proven: true, maxPrimaryKey }),
      () => unprovenBoundary(),
    );
  } catch {
    return Promise.resolve(unprovenBoundary());
  }
}

async function readCapturePages(capture: BrowserLogCapture): Promise<LogEntry[]> {
  if (!capture.readPage) return capture.memoryEntries.map(({ entry }) => entry);
  const entries: LogEntry[] = [];
  let cursor: LogPageCursor | null = null;
  let done = false;
  while (!done) {
    const page = await capture.readPage(SNAPSHOT_PAGE_BYTES, cursor);
    entries.push(...page.entries.map(({ entry }) => entry));
    cursor = page.nextCursor;
    done = page.done;
  }
  return entries;
}

function emptyDrainResult(): DrainResult {
  return { completed: true, primaryKeys: [] };
}

function mergeDrainResults(left: DrainResult, right: DrainResult): DrainResult {
  return {
    completed: left.completed && right.completed,
    primaryKeys: left.primaryKeys.concat(right.primaryKeys),
  };
}

function normalizePrimaryKeys(value: readonly number[] | undefined): number[] {
  if (!Array.isArray(value)) return [];
  return value.filter((primaryKey) => Number.isSafeInteger(primaryKey));
}

function unprovenBoundary(): CaptureBoundaryResult {
  return { proven: false, maxPrimaryKey: null };
}

function cancelCollectionTimer(): void {
  if (collectionTimer === null) return;
  clearTimeout(collectionTimer);
  collectionTimer = null;
}

function cancelIdleFallbackTimer(): void {
  if (idleFallbackTimer === null) return;
  clearTimeout(idleFallbackTimer);
  idleFallbackTimer = null;
}

function cancelIdleCallback(): void {
  idleGeneration += 1;
  if (idleCallbackHandle !== null) {
    if (typeof globalThis.cancelIdleCallback === "function") {
      globalThis.cancelIdleCallback(idleCallbackHandle);
    }
    idleCallbackHandle = null;
  }
  cancelIdleFallbackTimer();
}

function degradePersistence(): void {
  cancelCollectionTimer();
  cancelIdleCallback();
  persistenceFailures += 1;
  storageMode = "memory";
  staging = [];
  stagingBytes = 0;
}

function randomID(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }
}

export function _resetRuntimeForTesting(): void {
  cancelCollectionTimer();
  cancelIdleCallback();
  identityScope = "default-user";
  staging = [];
  stagingBytes = 0;
  nextSequence = 0;
  drainPromise = null;
  storageMode = "indexeddb";
  persistenceFailures = 0;
  stagingDropped = 0;
}
