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

type Staged = PreparedLogEntry & { sequence: number };

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
let drainPromise: Promise<void> | null = null;
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
  const flushed = await waitForCaptureFlush(flushStaging(watermark));
  if (!flushed || storageMode === "memory") {
    return { storageMode: "memory", flushTimeout: !flushed, memoryEntries, readPage: null };
  }
  return {
    storageMode: "indexeddb",
    flushTimeout: false,
    memoryEntries,
    readPage: (maxBytes, after) => store.readPage(scope, maxBytes, after),
  };
}

export async function snapshotBrowserLogs(scope: string): Promise<LogEntry[]> {
  const capture = await beginBrowserLogCapture(scope);
  if (capture.storageMode === "memory") {
    return capture.memoryEntries.map(({ entry }) => entry);
  }
  try {
    return await store.snapshot(scope);
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

function requestDrain(maxSequence = nextSequence): Promise<void> {
  if (drainPromise) return drainPromise;
  if (storageMode === "memory" || !hasStagedThrough(maxSequence)) return Promise.resolve();
  drainPromise = drainLoop(maxSequence).finally(() => {
    drainPromise = null;
    if (storageMode === "indexeddb" && staging.length > 0) scheduleDrain();
  });
  return drainPromise;
}

async function drainLoop(maxSequence: number): Promise<void> {
  while (storageMode === "indexeddb" && hasStagedThrough(maxSequence)) {
    if (!(await drainBatch(maxSequence))) return;
  }
}

async function drainBatch(maxSequence: number): Promise<boolean> {
  if (storageMode === "memory" || !hasStagedThrough(maxSequence)) return true;
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
    await store.append(batch);
  } catch {
    for (const item of batch.reverse()) {
      staging.unshift(item);
      stagingBytes += item.bytes;
    }
    degradePersistence();
    return false;
  }
  return true;
}

function hasStagedThrough(maxSequence: number): boolean {
  return staging.some((item) => item.sequence <= maxSequence);
}

async function flushStaging(maxSequence = nextSequence): Promise<void> {
  cancelCollectionTimer();
  cancelIdleCallback();
  while (drainPromise || (storageMode === "indexeddb" && hasStagedThrough(maxSequence))) {
    await requestDrain(maxSequence);
  }
}

function waitForCaptureFlush(flush: Promise<void>): Promise<boolean> {
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
