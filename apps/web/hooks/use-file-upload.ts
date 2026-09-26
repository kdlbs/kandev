"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type MutableRefObject,
  type SetStateAction,
} from "react";
import {
  preflightWorkspaceUpload,
  uploadWorkspaceFile,
  type UploadConflict,
  type UploadResolution,
  type UploadedWorkspaceFile,
} from "@/lib/api/domains/workspace-file-api";
import {
  joinDestination,
  normalizeUploadSelection,
  type UploadSelectionEntry,
} from "@/lib/utils/upload-selection";

/**
 * Per-file upload status. Mirrors the vocabulary chat attachments already use,
 * plus `blocked` for a file whose destination conflicts and is awaiting a
 * decision.
 */
export type UploadStatus = "pending" | "blocked" | "uploading" | "ready" | "failed";

export type UploadItem = {
  id: string;
  relativePath: string;
  destinationPath: string;
  status: UploadStatus;
  /** Server-reported path, authoritative after a keep-both rename. */
  writtenPath?: string;
  error?: string;
};

/** Skip is a resolution in the UI but never reaches the wire. */
export type ConflictChoice = UploadResolution | "skip";

export type PendingConflicts = {
  conflicts: UploadConflict[];
  /** Destination path -> relative path, so the dialog can label per file. */
  byDestination: Map<string, string>;
};

export type UploadFilesResult = {
  uploaded: UploadedWorkspaceFile[];
  cancelled: boolean;
  failed: number;
  skipped: string[];
};

type PendingBatch = {
  id: number;
  sessionId: string;
  dir: string;
  repo?: string;
  entries: UploadSelectionEntry[];
  skipped: string[];
  resolve: (result: UploadFilesResult) => void;
};

type ItemPatcher = (id: string, patch: Partial<UploadItem>) => void;

const EMPTY_RESULT: UploadFilesResult = { uploaded: [], cancelled: false, failed: 0, skipped: [] };
const CANCELLED_RESULT: UploadFilesResult = {
  uploaded: [],
  cancelled: true,
  failed: 0,
  skipped: [],
};

function failedUploadResult(failed: number, skipped: string[]): UploadFilesResult {
  return { uploaded: [], cancelled: false, failed, skipped };
}

function handleEmptyUploadSelection(
  dir: string,
  skipped: string[],
  batchId: number,
  setUploads: Dispatch<SetStateAction<UploadItem[]>>,
): UploadFilesResult {
  if (skipped.length === 0) return EMPTY_RESULT;
  setUploads(buildUploadItems(batchId, dir, [], skipped));
  return failedUploadResult(skipped.length, skipped);
}

function markBatchFailed(
  batchId: number,
  message: string | undefined,
  setUploads: Dispatch<SetStateAction<UploadItem[]>>,
) {
  setUploads((prev) =>
    prev.map((item) =>
      item.id.startsWith(`${batchId}:`) && item.status !== "failed"
        ? { ...item, status: "failed", error: message }
        : item,
    ),
  );
}

function useSessionChangeReset(
  sessionId: string | null,
  pendingRef: MutableRefObject<PendingBatch | null>,
  activeBatchRef: MutableRefObject<PendingBatch | null>,
  setConflicts: Dispatch<SetStateAction<PendingConflicts | null>>,
  setUploads: Dispatch<SetStateAction<UploadItem[]>>,
) {
  const previousSessionRef = useRef(sessionId);
  useEffect(() => {
    if (previousSessionRef.current === sessionId) return;
    previousSessionRef.current = sessionId;
    const pending = pendingRef.current;
    pendingRef.current = null;
    activeBatchRef.current = null;
    setConflicts(null);
    setUploads([]);
    if (pending) {
      pending.resolve({ ...CANCELLED_RESULT, skipped: pending.skipped });
    }
  }, [sessionId, pendingRef, activeBatchRef, setConflicts, setUploads]);
}

function itemId(batchId: number, destinationPath: string, index: number): string {
  return `${batchId}:${index}:${destinationPath}`;
}

function buildUploadItems(
  batchId: number,
  dir: string,
  entries: UploadSelectionEntry[],
  skipped: string[],
): UploadItem[] {
  const entryItems = entries.map((entry, index) => {
    const destinationPath = joinDestination(dir, entry.relativePath);
    return {
      id: itemId(batchId, destinationPath, index),
      relativePath: entry.relativePath,
      destinationPath,
      status: "pending" as const,
    };
  });
  const skippedItems = skipped.map((relativePath, index) => ({
    id: `${batchId}:skipped:${index}:${relativePath}`,
    relativePath,
    destinationPath: joinDestination(dir, relativePath),
    status: "failed" as const,
  }));
  return [...entryItems, ...skippedItems];
}

function destinationIndex(dir: string, entries: UploadSelectionEntry[]): Map<string, string> {
  const byDestination = new Map<string, string>();
  for (const entry of entries) {
    byDestination.set(joinDestination(dir, entry.relativePath), entry.relativePath);
  }
  return byDestination;
}

function preflightBatch(
  sessionId: string,
  dir: string,
  repo: string | undefined,
  entries: UploadSelectionEntry[],
) {
  return preflightWorkspaceUpload({
    sessionId,
    dir,
    repo,
    paths: entries.map((entry) => entry.relativePath),
  });
}

function parkUploadBatch({
  batch,
  found,
  dir,
  entries,
  pendingRef,
  setConflicts,
  setUploads,
}: {
  batch: PendingBatch;
  found: UploadConflict[];
  dir: string;
  entries: UploadSelectionEntry[];
  pendingRef: MutableRefObject<PendingBatch | null>;
  setConflicts: Dispatch<SetStateAction<PendingConflicts | null>>;
  setUploads: Dispatch<SetStateAction<UploadItem[]>>;
}): Promise<UploadFilesResult> {
  const conflicting = new Set(found.map((conflict) => conflict.path));
  setUploads((prev) =>
    prev.map((item) =>
      conflicting.has(item.destinationPath) ? { ...item, status: "blocked" } : item,
    ),
  );
  return new Promise<UploadFilesResult>((resolve) => {
    pendingRef.current = { ...batch, resolve };
    setConflicts({ conflicts: found, byDestination: destinationIndex(dir, entries) });
  });
}

/**
 * Upload each surviving file in the batch, one request per file so a rejection
 * of one does not fail the rest.
 */
async function performUploads({
  sessionId,
  batch,
  choices,
  patch,
  drop,
  isActive,
}: {
  sessionId: string;
  batch: PendingBatch;
  choices: Map<string, ConflictChoice>;
  patch: ItemPatcher;
  drop: (id: string) => void;
  isActive: () => boolean;
}): Promise<UploadFilesResult> {
  const uploaded: UploadedWorkspaceFile[] = [];
  let failed = batch.skipped.length;

  for (const [index, entry] of batch.entries.entries()) {
    if (!isActive()) {
      return { uploaded, cancelled: true, failed, skipped: batch.skipped };
    }
    const destinationPath = joinDestination(batch.dir, entry.relativePath);
    const id = itemId(batch.id, destinationPath, index);
    const choice = choices.get(destinationPath);

    if (choice === "skip") {
      drop(id);
      continue;
    }

    patch(id, { status: "uploading" });
    try {
      const result = await uploadWorkspaceFile({
        sessionId,
        dir: batch.dir,
        repo: batch.repo,
        relativePath: entry.relativePath,
        file: entry.file,
        resolution: choice,
      });
      uploaded.push(result);
      patch(id, { status: "ready", writtenPath: result.path });
    } catch (error) {
      failed += 1;
      patch(id, {
        status: "failed",
        error: error instanceof Error ? error.message : undefined,
      });
    }
  }

  return { uploaded, cancelled: !isActive(), failed, skipped: batch.skipped };
}

/**
 * Two-phase workspace upload.
 *
 * Phase one preflights the whole selection and, if anything conflicts, parks the
 * batch and exposes `conflicts` for a dialog to resolve. Phase two uploads.
 * Nothing is written until every conflict has a decision, and cancelling writes
 * nothing at all, including the files that had no conflict. That guarantee is
 * why non-conflicting files are not uploaded eagerly while the dialog is open.
 */
export function useFileUpload(sessionId: string | null) {
  const [uploads, setUploads] = useState<UploadItem[]>([]);
  const [conflicts, setConflicts] = useState<PendingConflicts | null>(null);
  const pendingRef = useRef<PendingBatch | null>(null);
  const activeBatchRef = useRef<PendingBatch | null>(null);
  const nextBatchIdRef = useRef(0);
  useSessionChangeReset(sessionId, pendingRef, activeBatchRef, setConflicts, setUploads);

  const patchItem = useCallback<ItemPatcher>((id, patch) => {
    setUploads((prev) => prev.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  }, []);

  const dropItem = useCallback((id: string) => {
    setUploads((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const runBatch = useCallback(
    (batch: PendingBatch, choices: Map<string, ConflictChoice>) =>
      performUploads({
        sessionId: batch.sessionId,
        batch,
        choices,
        patch: patchItem,
        drop: dropItem,
        isActive: () => activeBatchRef.current?.id === batch.id,
      }),
    [patchItem, dropItem],
  );

  const uploadFiles = useCallback(
    async (dir: string, files: ArrayLike<File>, repo?: string): Promise<UploadFilesResult> => {
      if (!sessionId) return EMPTY_RESULT;
      const { entries, skipped } = normalizeUploadSelection(files);
      if (activeBatchRef.current) {
        return failedUploadResult(entries.length + skipped.length, skipped);
      }
      if (entries.length === 0)
        return handleEmptyUploadSelection(dir, skipped, ++nextBatchIdRef.current, setUploads);

      const batch: PendingBatch = {
        id: ++nextBatchIdRef.current,
        sessionId,
        dir,
        repo,
        entries,
        skipped,
        resolve: () => {},
      };
      activeBatchRef.current = batch;
      setUploads(buildUploadItems(batch.id, dir, entries, skipped));

      let found: UploadConflict[];
      try {
        found = await preflightBatch(sessionId, dir, repo, entries);
      } catch (error) {
        const message = error instanceof Error ? error.message : undefined;
        markBatchFailed(batch.id, message, setUploads);
        if (activeBatchRef.current?.id === batch.id) activeBatchRef.current = null;
        return failedUploadResult(entries.length + skipped.length, skipped);
      }

      if (activeBatchRef.current?.id !== batch.id) {
        return { ...CANCELLED_RESULT, skipped: batch.skipped };
      }

      if (found.length === 0) {
        const result = await runBatch(batch, new Map());
        if (activeBatchRef.current?.id === batch.id) activeBatchRef.current = null;
        return result;
      }

      // Park the batch and hand control to the dialog. Nothing has been written.
      return parkUploadBatch({
        batch,
        found,
        dir,
        entries,
        pendingRef,
        setConflicts,
        setUploads,
      });
    },
    [sessionId, runBatch],
  );

  /** Apply the dialog's per-file decisions and upload what survives. */
  const resolveConflicts = useCallback(
    async (choices: Map<string, ConflictChoice>) => {
      const batch = pendingRef.current;
      pendingRef.current = null;
      setConflicts(null);
      if (!batch) return;
      const result = await runBatch(batch, choices);
      if (activeBatchRef.current?.id === batch.id) activeBatchRef.current = null;
      batch.resolve(result);
    },
    [runBatch],
  );

  /** Cancel the parked batch. Nothing is uploaded, not even unconflicted files. */
  const cancelConflicts = useCallback(() => {
    const batch = pendingRef.current;
    pendingRef.current = null;
    setConflicts(null);
    setUploads([]);
    if (activeBatchRef.current?.id === batch?.id) activeBatchRef.current = null;
    batch?.resolve({ ...CANCELLED_RESULT, skipped: batch.skipped });
  }, []);

  const clearUploads = useCallback(() => setUploads([]), []);

  return { uploads, conflicts, uploadFiles, resolveConflicts, cancelConflicts, clearUploads };
}
