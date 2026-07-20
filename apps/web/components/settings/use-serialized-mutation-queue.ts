"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { RequestStatus } from "@/lib/http/use-request";

type Mutation = {
  operation: () => Promise<void>;
  errorTitle: string;
};

type PendingMutation = Mutation & {
  resolve: (saved: boolean) => void;
};

type MutationErrorHandler = (errorTitle: string, error: unknown) => void;

export function useSerializedMutationQueue(onError: MutationErrorHandler) {
  const [status, setStatus] = useState<RequestStatus>("idle");
  const queueRef = useRef<PendingMutation[]>([]);
  const activeRef = useRef(false);
  const failedRef = useRef<Mutation | null>(null);
  const successTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const processNextRef = useRef<() => Promise<void>>(async () => {});
  const onErrorRef = useRef(onError);
  onErrorRef.current = onError;

  const clearSuccessTimeout = useCallback(() => {
    if (!successTimeoutRef.current) return;
    clearTimeout(successTimeoutRef.current);
    successTimeoutRef.current = null;
  }, []);

  const finishQueue = useCallback(() => {
    setStatus("success");
    clearSuccessTimeout();
    successTimeoutRef.current = setTimeout(() => setStatus("idle"), 1500);
  }, [clearSuccessTimeout]);

  const processNext = useCallback(async () => {
    if (activeRef.current || failedRef.current) return;
    const pending = queueRef.current.shift();
    if (!pending) return;

    activeRef.current = true;
    clearSuccessTimeout();
    setStatus("loading");
    try {
      await pending.operation();
      pending.resolve(true);
      activeRef.current = false;
      if (queueRef.current.length > 0) void processNextRef.current();
      else finishQueue();
    } catch (error) {
      activeRef.current = false;
      failedRef.current = pending;
      pending.resolve(false);
      setStatus("error");
      onErrorRef.current(pending.errorTitle, error);
    }
  }, [clearSuccessTimeout, finishQueue]);
  processNextRef.current = processNext;

  const run = useCallback((operation: () => Promise<void>, errorTitle: string) => {
    return new Promise<boolean>((resolve) => {
      queueRef.current.push({ operation, errorTitle, resolve });
      void processNextRef.current();
    });
  }, []);

  const retry = useCallback(async () => {
    const failed = failedRef.current;
    if (!failed || activeRef.current) return false;

    activeRef.current = true;
    clearSuccessTimeout();
    setStatus("loading");
    try {
      await failed.operation();
      failedRef.current = null;
      activeRef.current = false;
      if (queueRef.current.length > 0) void processNextRef.current();
      else finishQueue();
      return true;
    } catch (error) {
      activeRef.current = false;
      setStatus("error");
      onErrorRef.current(failed.errorTitle, error);
      return false;
    }
  }, [clearSuccessTimeout, finishQueue]);

  useEffect(() => {
    return () => {
      clearSuccessTimeout();
      for (const pending of queueRef.current) pending.resolve(false);
      queueRef.current = [];
    };
  }, [clearSuccessTimeout]);

  return { status, run, retry };
}
