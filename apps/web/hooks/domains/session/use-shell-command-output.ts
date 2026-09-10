import { useCallback, useEffect, useRef, useState } from "react";
import {
  fetchShellCommandOutput,
  type ShellCommandOutputSnapshot,
} from "@/lib/api/domains/session-api";
import { isTerminalToolCallStatus } from "@/lib/utils/tool-call-status";

const POLL_INTERVAL_MS = 1_000;
const MAX_RETRY_INTERVAL_MS = 5_000;
const MAX_CACHED_OUTPUTS = 200;

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
  outputKey: string;
  consumer: symbol | null;
  timer: ReturnType<typeof setTimeout> | null;
};

type SharedRequest = {
  controller: AbortController;
  consumers: Set<symbol>;
  terminalProjection: boolean;
  promise: Promise<ShellCommandOutputSnapshot>;
};

type SharedOutput = {
  snapshot: ShellCommandOutputSnapshot | null;
  request: SharedRequest | null;
};

const outputCache = new Map<string, SharedOutput>();

function retryDelay(failureCount: number) {
  return Math.min(POLL_INTERVAL_MS * 2 ** Math.max(0, failureCount - 1), MAX_RETRY_INTERVAL_MS);
}

function asError(error: unknown) {
  return error instanceof Error ? error : new Error("Command output unavailable");
}

function touchOutput(outputKey: string, output: SharedOutput) {
  outputCache.delete(outputKey);
  outputCache.set(outputKey, output);
}

function pruneOutputCache() {
  if (outputCache.size <= MAX_CACHED_OUTPUTS) return;
  for (const [outputKey, output] of outputCache) {
    if (output.request) continue;
    outputCache.delete(outputKey);
    if (outputCache.size <= MAX_CACHED_OUTPUTS) return;
  }
}

function readCachedOutput(outputKey: string) {
  const output = outputCache.get(outputKey);
  if (output) touchOutput(outputKey, output);
  return output?.snapshot ?? null;
}

function requestSharedOutput(
  outputKey: string,
  sessionId: string,
  messageId: string,
  consumer: symbol,
  terminalProjection: boolean,
) {
  const output = outputCache.get(outputKey) ?? { snapshot: null, request: null };
  touchOutput(outputKey, output);
  if (output.snapshot && isTerminalToolCallStatus(output.snapshot.status)) {
    return Promise.resolve(output.snapshot);
  }
  if (output.request) {
    if (!terminalProjection || output.request.terminalProjection) {
      output.request.consumers.add(consumer);
      return output.request.promise;
    }
    output.request.controller.abort();
    output.request = null;
  }

  const controller = new AbortController();
  const request: SharedRequest = {
    controller,
    consumers: new Set([consumer]),
    terminalProjection,
    promise: fetchShellCommandOutput(sessionId, messageId, {
      init: { signal: controller.signal },
    })
      .then((snapshot) => {
        if (output.request === request) {
          output.snapshot = snapshot;
          touchOutput(outputKey, output);
        }
        return snapshot;
      })
      .finally(() => {
        if (output.request === request) output.request = null;
        pruneOutputCache();
      }),
  };
  output.request = request;
  pruneOutputCache();
  return request.promise;
}

function releaseSharedOutput(outputKey: string, consumer: symbol) {
  const output = outputCache.get(outputKey);
  const request = output?.request;
  if (!output || !request) return;
  request.consumers.delete(consumer);
  if (request.consumers.size > 0) return;
  output.request = null;
  request.controller.abort();
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
  const operationRef = useRef<PollOperation>({
    generation: 0,
    outputKey: "",
    consumer: null,
    timer: null,
  });
  messageStatusRef.current = messageStatus;

  const stop = useCallback(() => {
    const operation = operationRef.current;
    operation.generation += 1;
    if (operation.timer) clearTimeout(operation.timer);
    operation.timer = null;
    if (operation.outputKey && operation.consumer) {
      releaseSharedOutput(operation.outputKey, operation.consumer);
    }
    operation.outputKey = "";
    operation.consumer = null;
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
      snapshotRef.current = readCachedOutput(outputKey);
      setSnapshot(snapshotRef.current);
      setError(null);
    }

    const operation = operationRef.current;
    const generation = operation.generation;
    const consumer = Symbol(outputKey);
    operation.outputKey = outputKey;
    operation.consumer = consumer;
    let failureCount = 0;

    if (snapshotRef.current && isTerminalToolCallStatus(snapshotRef.current.status)) {
      setIsLoading(false);
      return stop;
    }

    const requestSnapshot = async () => {
      if (!snapshotRef.current) setIsLoading(true);
      try {
        const nextSnapshot = await requestSharedOutput(
          outputKey,
          sessionId,
          messageId,
          consumer,
          isTerminalToolCallStatus(messageStatusRef.current),
        );
        if (operation.generation !== generation) return;
        failureCount = 0;
        snapshotRef.current = nextSnapshot;
        setSnapshot(nextSnapshot);
        setError(null);
        setIsLoading(false);
        if (
          !isTerminalToolCallStatus(nextSnapshot.status) &&
          !isTerminalToolCallStatus(messageStatusRef.current)
        ) {
          operation.timer = setTimeout(requestSnapshot, POLL_INTERVAL_MS);
        }
      } catch (requestError) {
        if (operation.generation !== generation) return;
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
