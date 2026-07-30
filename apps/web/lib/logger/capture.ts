import { uploadFrontendBundleChunk } from "@/lib/api/domains/system-api";
import type { LogEntry } from "./buffer";
import { browserInstallationID, browserLogMetadata, snapshotBrowserLogs } from "./runtime";

const TARGET_CHUNK_BYTES = 800 * 1024;

export type CaptureRequest = {
  bundle_id: string;
  capture_deadline: string;
  max_chunk_bytes: number;
  max_browser_profiles: number;
};

export async function handleBrowserLogCapture(
  request: CaptureRequest,
  identityScope: string | null,
): Promise<void> {
  if (!identityScope || !request.bundle_id || Date.now() >= Date.parse(request.capture_deadline)) {
    return;
  }
  const streamID = randomID();
  const entries = await snapshotBrowserLogs(identityScope);
  const chunks = chunkEntries(entries, Math.min(TARGET_CHUNK_BYTES, request.max_chunk_bytes));
  const storageMode = browserLogMetadata().storage_mode === "memory" ? "memory" : "indexeddb";
  for (let index = 0; index < chunks.length; index++) {
    const done = index === chunks.length - 1;
    await uploadFrontendBundleChunk(request.bundle_id, {
      browser_id: browserInstallationID(),
      capture_stream_id: streamID,
      chunk_index: index,
      done,
      storage_mode: storageMode,
      capture_metadata: done ? boundedMetadata(browserLogMetadata()) : null,
      entries: chunks[index],
    });
  }
}

export function chunkEntries(entries: LogEntry[], maxBytes: number): LogEntry[][] {
  const chunks: LogEntry[][] = [[]];
  let currentBytes = 0;
  for (const entry of entries) {
    const bytes = new TextEncoder().encode(JSON.stringify(entry)).byteLength + 1;
    if (chunks.at(-1)!.length > 0 && currentBytes + bytes > maxBytes) {
      chunks.push([]);
      currentBytes = 0;
    }
    chunks.at(-1)!.push(entry);
    currentBytes += bytes;
  }
  return chunks;
}

function boundedMetadata(metadata: Record<string, unknown>): Record<string, unknown> {
  const serialized = JSON.stringify(metadata);
  if (new TextEncoder().encode(serialized).byteLength <= 8 * 1024) return metadata;
  return { storage_mode: metadata.storage_mode, metadata_truncated: true };
}

function randomID(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
  }
}
