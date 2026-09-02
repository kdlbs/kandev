"use client";

import { useCallback, useRef, useState } from "react";
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
};

type PendingBatch = {
  dir: string;
  repo?: string;
  entries: UploadSelectionEntry[];
  resolve: (result: UploadFilesResult) => void;
};

type ItemPatcher = (id: string, patch: Partial<UploadItem>) => void;

const EMPTY_RESULT: UploadFilesResult = { uploaded: [], cancelled: false, failed: 0 };
const CANCELLED_RESULT: UploadFilesResult = { uploaded: [], cancelled: true, failed: 0 };

function itemId(destinationPath: string, index: number): string {
  return `${index}:${destinationPath}`;
}

function buildUploadItems(dir: string, entries: UploadSelectionEntry[]): UploadItem[] {
  return entries.map((entry, index) => {
    const destinationPath = joinDestination(dir, entry.relativePath);
    return {
      id: itemId(destinationPath, index),
      relativePath: entry.relativePath,
      destinationPath,
      status: "pending" as const,
    };
  });
}

function destinationIndex(dir: string, entries: UploadSelectionEntry[]): Map<string, string> {
  const byDestination = new Map<string, string>();
  for (const entry of entries) {
    byDestination.set(joinDestination(dir, entry.relativePath), entry.relativePath);
  }
  return byDestination;
}

/**
 * Upload each surviving file in the batch, one request per file so a rejection
 * of one does not fail the rest.
 */
async function performUploads(
  sessionId: string,
  batch: PendingBatch,
  choices: Map<string, ConflictChoice>,
  patch: ItemPatcher,
  drop: (id: string) => void,
): Promise<UploadFilesResult> {
  const uploaded: UploadedWorkspaceFile[] = [];
  let failed = 0;

  for (const [index, entry] of batch.entries.entries()) {
    const destinationPath = joinDestination(batch.dir, entry.relativePath);
    const id = itemId(destinationPath, index);
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

  return { uploaded, cancelled: false, failed };
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

  const patchItem = useCallback<ItemPatcher>((id, patch) => {
    setUploads((prev) => prev.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  }, []);

  const dropItem = useCallback((id: string) => {
    setUploads((prev) => prev.filter((item) => item.id !== id));
  }, []);

  const runBatch = useCallback(
    (batch: PendingBatch, choices: Map<string, ConflictChoice>) =>
      sessionId
        ? performUploads(sessionId, batch, choices, patchItem, dropItem)
        : Promise.resolve(EMPTY_RESULT),
    [sessionId, patchItem, dropItem],
  );

  const uploadFiles = useCallback(
    async (dir: string, files: ArrayLike<File>, repo?: string): Promise<UploadFilesResult> => {
      if (!sessionId) return EMPTY_RESULT;
      const { entries } = normalizeUploadSelection(files);
      if (entries.length === 0) return EMPTY_RESULT;

      setUploads(buildUploadItems(dir, entries));
      const batch: PendingBatch = { dir, repo, entries, resolve: () => {} };

      let found: UploadConflict[];
      try {
        found = await preflightWorkspaceUpload({
          sessionId,
          dir,
          repo,
          paths: entries.map((entry) => entry.relativePath),
        });
      } catch (error) {
        const message = error instanceof Error ? error.message : undefined;
        setUploads((prev) => prev.map((item) => ({ ...item, status: "failed", error: message })));
        return { uploaded: [], cancelled: false, failed: entries.length };
      }

      if (found.length === 0) return runBatch(batch, new Map());

      const conflicting = new Set(found.map((conflict) => conflict.path));
      setUploads((prev) =>
        prev.map((item) =>
          conflicting.has(item.destinationPath) ? { ...item, status: "blocked" } : item,
        ),
      );

      // Park the batch and hand control to the dialog. Nothing has been written.
      return new Promise<UploadFilesResult>((resolve) => {
        pendingRef.current = { ...batch, resolve };
        setConflicts({ conflicts: found, byDestination: destinationIndex(dir, entries) });
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
      batch.resolve(await runBatch(batch, choices));
    },
    [runBatch],
  );

  /** Cancel the parked batch. Nothing is uploaded, not even unconflicted files. */
  const cancelConflicts = useCallback(() => {
    const batch = pendingRef.current;
    pendingRef.current = null;
    setConflicts(null);
    setUploads([]);
    batch?.resolve(CANCELLED_RESULT);
  }, []);

  const clearUploads = useCallback(() => setUploads([]), []);

  return { uploads, conflicts, uploadFiles, resolveConflicts, cancelConflicts, clearUploads };
}
