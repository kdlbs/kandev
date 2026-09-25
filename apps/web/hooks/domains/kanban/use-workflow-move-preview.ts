"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  previewWorkflowMove,
  type WorkflowMoveEntryOptions,
  type WorkflowMovePreviewResponse,
} from "@/lib/api";
import { normalizeWorkflowMoveEntryOptions } from "@/lib/api/domains/kanban-api";

const PREVIEW_DEBOUNCE_MS = 150;
export const MAX_PREVIEW_CONCURRENT_REQUESTS = 2;

export type WorkflowMovePreviewStatus = "idle" | "loading" | "success" | "error";

export type WorkflowMovePreviewState = {
  status: WorkflowMovePreviewStatus;
  preview: WorkflowMovePreviewResponse | null;
  error: unknown;
  retry: () => void;
};

type WorkflowMovePreviewParams = {
  taskId?: string | null;
  workflowId?: string | null;
  workflowStepId?: string | null;
  entryOptions?: WorkflowMoveEntryOptions | null;
  enabled?: boolean;
  /** Changes when an external task/session/workflow projection is known stale. */
  invalidationKey?: string | number | null;
};

type PreviewRequestEntry = {
  controller: AbortController;
  consumers: number;
  promise: Promise<WorkflowMovePreviewResponse>;
  resolve: (preview: WorkflowMovePreviewResponse) => void;
  reject: (error?: unknown) => void;
  started: boolean;
  finished: boolean;
};

type QueuedPreview = {
  key: string;
  entry: PreviewRequestEntry;
  taskId: string;
  workflowId: string;
  workflowStepId: string;
  entryOptions: WorkflowMoveEntryOptions | undefined;
};

const inFlightPreviews = new Map<string, PreviewRequestEntry>();
const previewQueue: QueuedPreview[] = [];
let activePreviewCount = 0;

function previewOptionKey(options: WorkflowMoveEntryOptions | null | undefined): string {
  const normalized = normalizeWorkflowMoveEntryOptions(options);
  return JSON.stringify({
    reset_context: normalized?.reset_context === true,
    skip_step_prompt: normalized?.skip_step_prompt === true,
    // The text itself does not change routing. Its presence changes whether a
    // skipped prompt still dispatches a turn, so only that fact invalidates a
    // pending disclosure request while the user types.
    instructions_present: Boolean(normalized?.instructions),
  });
}

function previewRequestKey(
  taskId: string | null | undefined,
  workflowId: string | null | undefined,
  workflowStepId: string | null | undefined,
  options: WorkflowMoveEntryOptions | null | undefined,
  invalidationKey: string | number | null | undefined,
): string {
  return JSON.stringify([
    taskId ?? "",
    workflowId ?? "",
    workflowStepId ?? "",
    previewOptionKey(options),
    invalidationKey ?? "",
  ]);
}

function isAbortError(error: unknown): boolean {
  return (
    typeof error === "object" &&
    error !== null &&
    "name" in error &&
    (error as { name?: unknown }).name === "AbortError"
  );
}

function previewAbortError(): Error {
  const error = new Error("Workflow move preview was cancelled");
  error.name = "AbortError";
  return error;
}

function finishPreview(key: string, entry: PreviewRequestEntry): void {
  if (entry.finished) return;
  entry.finished = true;
  if (entry.started) activePreviewCount -= 1;
  if (inFlightPreviews.get(key) === entry) inFlightPreviews.delete(key);
  pumpPreviewQueue();
}

function pumpPreviewQueue(): void {
  while (activePreviewCount < MAX_PREVIEW_CONCURRENT_REQUESTS && previewQueue.length > 0) {
    const queued = previewQueue.shift();
    if (!queued || inFlightPreviews.get(queued.key) !== queued.entry || queued.entry.finished) {
      continue;
    }

    const { key, entry } = queued;
    entry.started = true;
    activePreviewCount += 1;
    let request: Promise<WorkflowMovePreviewResponse>;
    try {
      request = previewWorkflowMove(
        queued.taskId,
        {
          workflow_id: queued.workflowId,
          workflow_step_id: queued.workflowStepId,
          entry_options: queued.entryOptions,
        },
        { cache: "no-store", init: { signal: entry.controller.signal } },
      );
    } catch (error) {
      entry.reject(error);
      finishPreview(key, entry);
      continue;
    }

    request.then(
      (preview) => {
        entry.resolve(preview);
        finishPreview(key, entry);
      },
      (error: unknown) => {
        entry.reject(error);
        finishPreview(key, entry);
      },
    );
  }
}

function acquirePreview(
  key: string,
  taskId: string,
  workflowId: string,
  workflowStepId: string,
  entryOptions: WorkflowMoveEntryOptions | undefined,
): { promise: Promise<WorkflowMovePreviewResponse>; release: () => void } {
  let entry = inFlightPreviews.get(key);
  if (!entry || entry.controller.signal.aborted) {
    const controller = new AbortController();
    let resolvePromise!: (preview: WorkflowMovePreviewResponse) => void;
    let rejectPromise!: (error?: unknown) => void;
    const promise = new Promise<WorkflowMovePreviewResponse>((resolve, reject) => {
      resolvePromise = resolve;
      rejectPromise = reject;
    });
    entry = {
      controller,
      consumers: 0,
      promise,
      resolve: resolvePromise,
      reject: rejectPromise,
      started: false,
      finished: false,
    };
    inFlightPreviews.set(key, entry);
    // A queued request can be cancelled before its consumer attaches a
    // rejection handler. Keep cancellation from becoming an unhandled
    // rejection while the hook's generation guard handles the result.
    void entry.promise.catch(() => undefined);
    previewQueue.push({ key, entry, taskId, workflowId, workflowStepId, entryOptions });
    pumpPreviewQueue();
  }

  const acquiredEntry = entry;
  acquiredEntry.consumers += 1;
  let released = false;
  return {
    promise: acquiredEntry.promise,
    release: () => {
      if (released) return;
      released = true;
      acquiredEntry.consumers -= 1;
      if (acquiredEntry.consumers === 0 && inFlightPreviews.get(key) === acquiredEntry) {
        acquiredEntry.reject(previewAbortError());
        acquiredEntry.controller.abort();
        finishPreview(key, acquiredEntry);
      }
    },
  };
}

export function useWorkflowMovePreview({
  taskId,
  workflowId,
  workflowStepId,
  entryOptions,
  enabled = true,
  invalidationKey,
}: WorkflowMovePreviewParams): WorkflowMovePreviewState {
  const normalizedEntryOptions = normalizeWorkflowMoveEntryOptions(entryOptions);
  const entryOptionsRef = useRef(normalizedEntryOptions);
  entryOptionsRef.current = normalizedEntryOptions;
  const [retrySequence, setRetrySequence] = useState(0);
  const [state, setState] = useState<Omit<WorkflowMovePreviewState, "retry">>({
    status: "idle",
    preview: null,
    error: null,
  });
  const generationRef = useRef(0);
  const requestKey = previewRequestKey(
    taskId,
    workflowId,
    workflowStepId,
    normalizedEntryOptions,
    invalidationKey,
  );

  const retry = useCallback(() => setRetrySequence((current) => current + 1), []);

  useEffect(() => {
    const generation = ++generationRef.current;
    let release: (() => void) | undefined;

    setState({ status: "idle", preview: null, error: null });
    if (!enabled || !taskId || !workflowId || !workflowStepId) {
      return () => {
        generationRef.current += 1;
      };
    }

    setState({ status: "loading", preview: null, error: null });
    const timer = window.setTimeout(() => {
      if (generationRef.current !== generation) return;
      const request = acquirePreview(
        requestKey,
        taskId,
        workflowId,
        workflowStepId,
        entryOptionsRef.current,
      );
      release = request.release;
      request.promise.then(
        (preview) => {
          if (generationRef.current !== generation) return;
          setState({ status: "success", preview, error: null });
        },
        (error: unknown) => {
          if (generationRef.current !== generation || isAbortError(error)) return;
          setState({ status: "error", preview: null, error });
        },
      );
    }, PREVIEW_DEBOUNCE_MS);

    return () => {
      generationRef.current += 1;
      window.clearTimeout(timer);
      release?.();
    };
  }, [enabled, requestKey, retrySequence, taskId, workflowId, workflowStepId]);

  return { ...state, retry };
}

export { PREVIEW_DEBOUNCE_MS };
