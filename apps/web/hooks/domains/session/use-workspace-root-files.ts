"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { listWorkspaceRootFiles } from "@/lib/ws/workspace-files";
import type { WorkspaceRootFilesResponse } from "@/lib/types/workspace-files";

const ROOT_FILES_CHANGE_DEBOUNCE_MS = 300;

type PendingRootFilesRequest = {
  generation: number;
  promise: Promise<void>;
};

const latestGenerationByEnvironment = new Map<string, number>();
const pendingByEnvironment = new Map<string, PendingRootFilesRequest>();
const refetchTimersByEnvironment = new Map<string, ReturnType<typeof setTimeout>>();
const observerCountByEnvironment = new Map<string, number>();

function releaseEnvironmentObserver(environmentId: string) {
  const observerCount = (observerCountByEnvironment.get(environmentId) ?? 1) - 1;
  if (observerCount > 0) {
    observerCountByEnvironment.set(environmentId, observerCount);
    return;
  }

  observerCountByEnvironment.delete(environmentId);
  const timer = refetchTimersByEnvironment.get(environmentId);
  if (timer) {
    clearTimeout(timer);
    refetchTimersByEnvironment.delete(environmentId);
  }
  pendingByEnvironment.delete(environmentId);
  latestGenerationByEnvironment.set(
    environmentId,
    (latestGenerationByEnvironment.get(environmentId) ?? 0) + 1,
  );
}

function fetchRootFiles(
  environmentId: string,
  sessionId: string,
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  setWorkspaceRootFiles: (environmentId: string, files: WorkspaceRootFilesResponse) => void,
  force = false,
): Promise<void> {
  const pending = pendingByEnvironment.get(environmentId);
  if (!force && pending) return pending.promise;

  const generation = (latestGenerationByEnvironment.get(environmentId) ?? 0) + 1;
  latestGenerationByEnvironment.set(environmentId, generation);
  const promise = listWorkspaceRootFiles(client, sessionId)
    .then((response) => {
      if (latestGenerationByEnvironment.get(environmentId) === generation) {
        setWorkspaceRootFiles(environmentId, response);
      }
    })
    .catch(() => undefined)
    .finally(() => {
      if (pendingByEnvironment.get(environmentId)?.generation === generation) {
        pendingByEnvironment.delete(environmentId);
      }
    });
  pendingByEnvironment.set(environmentId, { generation, promise });
  return promise;
}

function scheduleRootFilesRefetch(
  environmentId: string,
  sessionId: string,
  client: NonNullable<ReturnType<typeof getWebSocketClient>>,
  setWorkspaceRootFiles: (environmentId: string, files: WorkspaceRootFilesResponse) => void,
) {
  const previousTimer = refetchTimersByEnvironment.get(environmentId);
  if (previousTimer) clearTimeout(previousTimer);
  const timer = setTimeout(() => {
    refetchTimersByEnvironment.delete(environmentId);
    void fetchRootFiles(environmentId, sessionId, client, setWorkspaceRootFiles, true);
  }, ROOT_FILES_CHANGE_DEBOUNCE_MS);
  refetchTimersByEnvironment.set(environmentId, timer);
}

export function useWorkspaceRootFiles(sessionId: string | null) {
  const environmentId = useAppStore((state) =>
    sessionId ? (state.environmentIdBySessionId[sessionId] ?? sessionId) : null,
  );
  const connectionStatus = useAppStore((state) => state.connection.status);
  const files = useAppStore((state) =>
    environmentId ? state.workspaceRootFiles.byEnvironmentId[environmentId] : undefined,
  );
  const setWorkspaceRootFiles = useAppStore((state) => state.setWorkspaceRootFiles);
  const inventoryFingerprint = useAppStore((state) => {
    if (!environmentId) return "";
    const inventory = state.gitStatus.inventoryByEnvironment?.[environmentId];
    if (!inventory) return "";
    return JSON.stringify([inventory.generation, [...(inventory.scopes ?? [])].sort()]);
  });
  const previousInventory = useRef({ environmentId, fingerprint: inventoryFingerprint });

  useEffect(() => {
    if (!sessionId || !environmentId || connectionStatus !== "connected") return;
    const client = getWebSocketClient();
    if (!client) return;

    void fetchRootFiles(environmentId, sessionId, client, setWorkspaceRootFiles);
    const unsubscribe = client.on("session.workspace.file.changes", (message) => {
      if (message.payload.session_id !== sessionId) return;
      if (!message.payload.changes?.some((change) => !change.repository_name)) return;
      scheduleRootFilesRefetch(environmentId, sessionId, client, setWorkspaceRootFiles);
    });
    observerCountByEnvironment.set(
      environmentId,
      (observerCountByEnvironment.get(environmentId) ?? 0) + 1,
    );
    return () => {
      unsubscribe();
      releaseEnvironmentObserver(environmentId);
    };
  }, [connectionStatus, environmentId, sessionId, setWorkspaceRootFiles]);

  useEffect(() => {
    const previous = previousInventory.current;
    previousInventory.current = { environmentId, fingerprint: inventoryFingerprint };
    if (
      !environmentId ||
      !sessionId ||
      connectionStatus !== "connected" ||
      previous.environmentId !== environmentId ||
      previous.fingerprint === inventoryFingerprint
    ) {
      return;
    }
    const client = getWebSocketClient();
    if (!client) return;
    scheduleRootFilesRefetch(environmentId, sessionId, client, setWorkspaceRootFiles);
  }, [connectionStatus, environmentId, inventoryFingerprint, sessionId, setWorkspaceRootFiles]);

  return files;
}
