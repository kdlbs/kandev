import { uploadFrontendBundleChunk } from "@/lib/api/domains/system-api";
import type { PreparedLogEntry } from "./buffer";
import type { LogPage, LogPageCursor } from "./indexeddb-store";
import {
  beginBrowserLogCapture,
  browserInstallationID,
  browserLogMetadata,
  type BrowserLogCapture,
} from "./runtime";

const FIRST_PAGE_BYTES = 128 * 1024;
const TARGET_CHUNK_BYTES = 800 * 1024;
const CAPTURE_METADATA_BYTES = 8 * 1024;

export type CaptureRequest = {
  bundle_id: string;
  capture_deadline: string;
  capture_timeout_ms?: number;
  max_chunk_bytes: number;
  max_browser_profiles: number;
};

export async function handleBrowserLogCapture(
  request: CaptureRequest,
  identityScope: string | null,
): Promise<void> {
  const deadline = localDeadline(request);
  if (!identityScope || !request.bundle_id || deadline === null || performance.now() >= deadline) {
    return;
  }
  const capture = await beginBrowserLogCapture(identityScope);
  const pageLimit = effectivePageLimit(request.max_chunk_bytes);
  if (pageLimit <= 0 || performance.now() >= deadline) return;

  await uploadCapturePages({
    request,
    capture,
    browserID: browserInstallationID(),
    streamID: randomID(),
    pageLimit,
    deadline,
  });
}

type CaptureContext = {
  request: CaptureRequest;
  capture: BrowserLogCapture;
  browserID: string;
  streamID: string;
  pageLimit: number;
  deadline: number;
};

async function uploadCapturePages({
  request,
  capture,
  browserID,
  streamID,
  pageLimit,
  deadline,
}: CaptureContext): Promise<void> {
  let source = capture.storageMode;
  let indexedCursor: LogPageCursor | null = null;
  let memoryOffset = 0;
  let firstPage = true;
  let uploaded = false;

  for (let chunkIndex = 0; ; chunkIndex += 1) {
    if (isExpired(deadline)) return;
    const result = await readCapturePage({
      capture,
      storageMode: source,
      indexedCursor,
      memoryOffset,
      maxBytes: firstPage ? Math.min(FIRST_PAGE_BYTES, pageLimit) : pageLimit,
      uploaded,
    });
    source = result.storageMode;
    if (isExpired(deadline)) return;

    const completed = await uploadCapturePage({
      request,
      capture,
      page: result.page,
      storageMode: source,
      browserID,
      streamID,
      chunkIndex,
      deadline,
    });
    if (!completed) return;
    uploaded = true;
    if (result.page.done) return;
    indexedCursor = result.page.nextCursor;
    memoryOffset = result.page.memoryOffset ?? memoryOffset;
    firstPage = false;
  }
}

type CapturePage = LogPage & { memoryOffset?: number };

type PageReadResult = {
  page: CapturePage;
  storageMode: "indexeddb" | "memory";
};

type PageReadContext = {
  capture: BrowserLogCapture;
  storageMode: "indexeddb" | "memory";
  indexedCursor: LogPageCursor | null;
  memoryOffset: number;
  maxBytes: number;
  uploaded: boolean;
};

async function readCapturePage({
  capture,
  storageMode,
  indexedCursor,
  memoryOffset,
  maxBytes,
  uploaded,
}: PageReadContext): Promise<PageReadResult> {
  try {
    const page =
      storageMode === "indexeddb"
        ? await capture.readPage!(maxBytes, indexedCursor)
        : memoryPage(capture.memoryEntries, memoryOffset, maxBytes);
    return { page, storageMode };
  } catch (error) {
    if (uploaded) throw error;
    return {
      page: memoryPage(capture.memoryEntries, 0, maxBytes),
      storageMode: "memory",
    };
  }
}

type PageUploadContext = {
  request: CaptureRequest;
  capture: BrowserLogCapture;
  page: CapturePage;
  storageMode: "indexeddb" | "memory";
  browserID: string;
  streamID: string;
  chunkIndex: number;
  deadline: number;
};

async function uploadCapturePage({
  request,
  capture,
  page,
  storageMode,
  browserID,
  streamID,
  chunkIndex,
  deadline,
}: PageUploadContext): Promise<boolean> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), remainingTime(deadline));
  try {
    await uploadFrontendBundleChunk(
      request.bundle_id,
      {
        browser_id: browserID,
        capture_stream_id: streamID,
        chunk_index: chunkIndex,
        done: page.done,
        storage_mode: storageMode,
        capture_metadata: page.done ? captureMetadata(storageMode, capture.flushTimeout) : null,
        entries: page.entries.map(({ entry }) => entry),
      },
      { init: { signal: controller.signal } },
    );
  } catch (error) {
    if (controller.signal.aborted) return false;
    throw error;
  } finally {
    clearTimeout(timeout);
  }
  return true;
}

function isExpired(deadline: number): boolean {
  return performance.now() >= deadline;
}

function remainingTime(deadline: number): number {
  return Math.max(0, deadline - performance.now());
}

function memoryPage(
  entries: PreparedLogEntry[],
  offset: number,
  maxBytes: number,
): LogPage & { memoryOffset: number } {
  const page: PreparedLogEntry[] = [];
  let bytes = 0;
  let index = offset;
  while (index < entries.length) {
    const next = entries[index];
    if (page.length > 0 && bytes + next.bytes > maxBytes) break;
    page.push(next);
    bytes += next.bytes;
    index += 1;
  }
  return { entries: page, nextCursor: null, done: index >= entries.length, memoryOffset: index };
}

function effectivePageLimit(maxChunkBytes: number): number {
  return Number.isFinite(maxChunkBytes) && maxChunkBytes > 0
    ? Math.min(TARGET_CHUNK_BYTES, maxChunkBytes)
    : 0;
}

function localDeadline(request: CaptureRequest): number | null {
  const started = performance.now();
  if (request.capture_timeout_ms !== undefined) {
    return Number.isFinite(request.capture_timeout_ms) && request.capture_timeout_ms >= 0
      ? started + request.capture_timeout_ms
      : null;
  }
  const serverDeadline = Date.parse(request.capture_deadline);
  const remaining = serverDeadline - Date.now();
  return Number.isFinite(remaining) && remaining >= 0 ? started + remaining : null;
}

function captureMetadata(
  storageMode: "indexeddb" | "memory",
  flushTimeout: boolean,
): Record<string, unknown> {
  const metadata: Record<string, unknown> = { ...browserLogMetadata(), storage_mode: storageMode };
  if (flushTimeout) metadata.flush_timeout = true;
  return boundedMetadata(metadata);
}

function boundedMetadata(metadata: Record<string, unknown>): Record<string, unknown> {
  const serialized = JSON.stringify(metadata);
  if (new TextEncoder().encode(serialized).byteLength <= CAPTURE_METADATA_BYTES) return metadata;
  return { storage_mode: metadata.storage_mode, metadata_truncated: true };
}

function randomID(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }
}
