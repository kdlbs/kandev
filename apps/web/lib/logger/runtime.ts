import { encodedBytes, getLogBuffer, snapshotLogs, type LogEntry, type LogLevel } from "./buffer";
import { IndexedDBLogStore } from "./indexeddb-store";

const DRAIN_ENTRY_LIMIT = 50;
const DRAIN_BYTE_LIMIT = 256 * 1024;
const STAGING_ENTRY_LIMIT = 500;
const STAGING_BYTE_LIMIT = 2 * 1024 * 1024;

type Staged = { entry: LogEntry; bytes: number };

const store = new IndexedDBLogStore();
let identityScope: string | null = "default-user";
let staging: Staged[] = [];
let stagingBytes = 0;
let scheduled = false;
let drainPromise: Promise<void> | null = null;
let storageMode: "indexeddb" | "memory" = "indexeddb";
let persistenceFailures = 0;
let stagingDropped = 0;

export function setLogIdentity(scope: string | null): void {
  identityScope = scope;
}

export function stageLogEntry(entry: Omit<LogEntry, "identity_scope">): void {
  const scoped = { ...entry, identity_scope: identityScope ?? undefined };
  if (!getLogBuffer().push(scoped)) return;
  if (!identityScope || storageMode === "memory") return;
  const bytes = encodedBytes(scoped);
  if (!makeStagingRoom(entry.level, bytes)) {
    stagingDropped += 1;
    return;
  }
  staging.push({ entry: scoped, bytes });
  stagingBytes += bytes;
  scheduleDrain();
}

export async function snapshotBrowserLogs(scope: string): Promise<LogEntry[]> {
  await flushStaging();
  if (storageMode === "indexeddb") {
    try {
      return await store.snapshot(scope);
    } catch {
      degradePersistence();
    }
  }
  return snapshotLogs(scope);
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
  if (scheduled || drainPromise) return;
  scheduled = true;
  const drain = () => {
    scheduled = false;
    void requestDrain();
  };
  if (typeof requestIdleCallback === "function") {
    requestIdleCallback(drain, { timeout: 1_000 });
  } else {
    setTimeout(drain, 250);
  }
}

function requestDrain(): Promise<void> {
  if (drainPromise) return drainPromise;
  if (storageMode === "memory" || staging.length === 0) return Promise.resolve();
  drainPromise = drainLoop().finally(() => {
    drainPromise = null;
    if (storageMode === "indexeddb" && staging.length > 0) scheduleDrain();
  });
  return drainPromise;
}

async function drainLoop(): Promise<void> {
  while (storageMode === "indexeddb" && staging.length > 0) {
    if (!(await drainBatch())) return;
  }
}

async function drainBatch(): Promise<boolean> {
  if (storageMode === "memory" || staging.length === 0) return true;
  const batch: Staged[] = [];
  let bytes = 0;
  while (staging.length > 0 && batch.length < DRAIN_ENTRY_LIMIT) {
    const next = staging[0];
    if (batch.length > 0 && bytes + next.bytes > DRAIN_BYTE_LIMIT) break;
    staging.shift();
    stagingBytes -= next.bytes;
    batch.push(next);
    bytes += next.bytes;
  }
  try {
    await store.append(batch.map(({ entry }) => entry));
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

async function flushStaging(): Promise<void> {
  while (drainPromise || (storageMode === "indexeddb" && staging.length > 0)) {
    await requestDrain();
  }
}

function degradePersistence(): void {
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
  identityScope = "default-user";
  staging = [];
  stagingBytes = 0;
  scheduled = false;
  drainPromise = null;
  storageMode = "indexeddb";
  persistenceFailures = 0;
  stagingDropped = 0;
}
