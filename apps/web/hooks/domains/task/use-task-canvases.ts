"use client";

import { useEffect, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { listTaskCanvases, type Canvas } from "@/lib/api/domains/canvas-api";
import type { Task } from "@/lib/types/http";
import { useCanvasLifecycleRevision } from "@/lib/canvas-lifecycle";

const EMPTY_CANVASES: Canvas[] = [];
export type TaskCanvasesLoadStatus = "idle" | "loading" | "success" | "error";

export type TaskCanvasesState = {
  key: string | null;
  canvases: Canvas[];
  status: TaskCanvasesLoadStatus;
};

type InFlightTaskCanvasesRequest = {
  fingerprint: string;
  promise: Promise<Canvas[]>;
};

const inFlightRequests = new Map<string, InFlightTaskCanvasesRequest>();

function taskCanvasesKey(workspaceId: string, taskId: string, authIdentity: string): string {
  return `${workspaceId}\u0000${taskId}\u0000${authIdentity}`;
}

function requestTaskCanvases(
  taskId: string,
  workspaceId: string,
  authIdentity: string,
  invalidationFingerprint: string,
): Promise<Canvas[]> {
  const key = taskCanvasesKey(workspaceId, taskId, authIdentity);
  const existing = inFlightRequests.get(key);
  if (existing?.fingerprint === invalidationFingerprint) return existing.promise;

  const request = listTaskCanvases(taskId, { workspaceId, cache: "no-store" }).then((response) => {
    if (!response || !Array.isArray(response.canvases)) throw new TypeError();
    return response.canvases;
  });
  const entry = { fingerprint: invalidationFingerprint, promise: request };
  inFlightRequests.set(key, entry);
  request.then(
    () => {
      if (inFlightRequests.get(key) === entry) inFlightRequests.delete(key);
    },
    () => {
      if (inFlightRequests.get(key) === entry) inFlightRequests.delete(key);
    },
  );
  return request;
}

const EMPTY_STATE: TaskCanvasesState = {
  key: null,
  canvases: EMPTY_CANVASES,
  status: "idle",
};

/** Loads the canvases that can be opened from a task's mobile Panels picker. */
export function useTaskCanvasesState(
  taskId: string | null | undefined,
  workspaceId: string | null | undefined,
  enabled = true,
): TaskCanvasesState {
  const connectionStatus = useAppStore((state) => state.connection.status);
  const authIdentity = useAppStore(
    (state) => `${state.auth.mode}\u0000${state.auth.user?.id ?? ""}`,
  );
  const [loaded, setLoaded] = useState<TaskCanvasesState>(EMPTY_STATE);
  const requestRef = useRef(0);
  const lifecycleRevision = useCanvasLifecycleRevision();

  useEffect(() => {
    const requestId = ++requestRef.current;
    if (!enabled || !taskId || !workspaceId) {
      setLoaded(EMPTY_STATE);
      return;
    }

    const key = taskCanvasesKey(workspaceId, taskId, authIdentity);
    const invalidationFingerprint = `${connectionStatus}\u0000${lifecycleRevision}`;
    setLoaded((current) =>
      current.key === key
        ? { ...current, status: "loading" }
        : { key, canvases: EMPTY_CANVASES, status: "loading" },
    );
    requestTaskCanvases(taskId, workspaceId, authIdentity, invalidationFingerprint)
      .then((canvases) => {
        if (requestRef.current !== requestId) return;
        setLoaded({ key, canvases, status: "success" });
      })
      .catch(() => {
        if (requestRef.current === requestId) {
          setLoaded((current) => ({
            key,
            canvases: current.key === key ? current.canvases : EMPTY_CANVASES,
            status: "error",
          }));
        }
      });

    return () => {
      if (requestRef.current === requestId) requestRef.current += 1;
    };
  }, [authIdentity, connectionStatus, enabled, lifecycleRevision, taskId, workspaceId]);

  const key = taskId && workspaceId ? taskCanvasesKey(workspaceId, taskId, authIdentity) : null;
  return loaded.key === key ? loaded : { key, canvases: EMPTY_CANVASES, status: "idle" };
}

export function useTaskCanvases(
  taskId: string | null | undefined,
  workspaceId: string | null | undefined,
  enabled = true,
): Canvas[] {
  return useTaskCanvasesState(taskId, workspaceId, enabled).canvases;
}

export function useTaskCanvasesForTask(
  task: Pick<Task, "id" | "workspace_id"> | null | undefined,
  _isMobile: boolean,
  canvasesEnabled: boolean,
): Canvas[] {
  return useTaskCanvasesStateForTask(task, canvasesEnabled).canvases;
}

export function useTaskCanvasesStateForTask(
  task: Pick<Task, "id" | "workspace_id"> | null | undefined,
  canvasesEnabled: boolean,
): TaskCanvasesState {
  return useTaskCanvasesState(task?.id ?? null, task?.workspace_id ?? null, canvasesEnabled);
}
