import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchShellCommandOutput,
  type ShellCommandOutputSnapshot,
} from "@/lib/api/domains/session-api";
import { isTerminalToolCallStatus, normalizeToolCallStatus } from "@/lib/utils/tool-call-status";

const POLL_INTERVAL_MS = 1_000;
const MAX_RETRY_INTERVAL_MS = 5_000;

export type UseShellCommandOutputOptions = {
  sessionId: string;
  messageId: string;
  isOpen: boolean;
  messageStatus?: string;
};

export type UseShellCommandOutputResult = {
  snapshot: ShellCommandOutputSnapshot | null;
  isLoading: boolean;
  error: Error | null;
  retry: () => void;
};

type PollOperation = {
  generation: number;
  controller: AbortController | null;
  timer: ReturnType<typeof setTimeout> | null;
};

function isRunningStatus(status: string | undefined) {
  return normalizeToolCallStatus(status) === "running";
}

function retryDelay(failureCount: number) {
  return Math.min(POLL_INTERVAL_MS * 2 ** Math.max(0, failureCount - 1), MAX_RETRY_INTERVAL_MS);
}

function asError(error: unknown) {
  return error instanceof Error ? error : new Error("Command output unavailable");
}

export function useShellCommandOutput({
  sessionId,
  messageId,
  isOpen,
  messageStatus,
}: UseShellCommandOutputOptions): UseShellCommandOutputResult {
  const [snapshot, setSnapshot] = useState<ShellCommandOutputSnapshot | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<Error | null>(null);
  const [retryVersion, setRetryVersion] = useState(0);
  const snapshotRef = useRef<ShellCommandOutputSnapshot | null>(null);
  const outputKeyRef = useRef("");
  const messageStatusRef = useRef(messageStatus);
  const operationRef = useRef<PollOperation>({ generation: 0, controller: null, timer: null });
  messageStatusRef.current = messageStatus;

  const stop = useCallback(() => {
    const operation = operationRef.current;
    operation.generation += 1;
    if (operation.timer) clearTimeout(operation.timer);
    operation.timer = null;
    operation.controller?.abort();
    operation.controller = null;
  }, []);

  useEffect(() => {
    stop();
    if (!isOpen || !sessionId || !messageId) {
      setIsLoading(false);
      return;
    }

    const outputKey = `${sessionId}:${messageId}`;
    if (outputKeyRef.current !== outputKey) {
      outputKeyRef.current = outputKey;
      snapshotRef.current = null;
      setSnapshot(null);
      setError(null);
    }

    const operation = operationRef.current;
    const generation = operation.generation;
    let failureCount = 0;

    const requestSnapshot = async () => {
      const controller = new AbortController();
      operation.controller = controller;
      if (!snapshotRef.current) setIsLoading(true);
      try {
        const nextSnapshot = await fetchShellCommandOutput(sessionId, messageId, {
          init: { signal: controller.signal },
        });
        if (operation.generation !== generation || controller.signal.aborted) return;
        operation.controller = null;
        failureCount = 0;
        snapshotRef.current = nextSnapshot;
        setSnapshot(nextSnapshot);
        setError(null);
        setIsLoading(false);
        if (
          isRunningStatus(nextSnapshot.status) &&
          !isTerminalToolCallStatus(messageStatusRef.current)
        ) {
          operation.timer = setTimeout(requestSnapshot, POLL_INTERVAL_MS);
        }
      } catch (requestError) {
        if (operation.generation !== generation || controller.signal.aborted) return;
        operation.controller = null;
        failureCount += 1;
        setError(asError(requestError));
        setIsLoading(false);
        if (!isTerminalToolCallStatus(messageStatusRef.current)) {
          operation.timer = setTimeout(requestSnapshot, retryDelay(failureCount));
        }
      }
    };

    void requestSnapshot();
    return stop;
  }, [isOpen, messageId, messageStatus, retryVersion, sessionId, stop]);

  const retry = useCallback(() => setRetryVersion((version) => version + 1), []);
  return { snapshot, isLoading, error, retry };
}
