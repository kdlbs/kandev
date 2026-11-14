"use client";

import { useCallback, useEffect, useRef, useState, type MutableRefObject } from "react";
import { useTranslation } from "react-i18next";
import { ApiError } from "@/lib/api/client";
import {
  cancelAntigravityLogin,
  getAntigravityLogin,
  startAntigravityLogin,
  type AntigravityLoginSnapshot,
} from "@/lib/api/domains/antigravity-auth-api";
import { copyToClipboard } from "@/lib/utils/copy-to-clipboard";

const ACTIVE_STATES = new Set(["starting", "waiting_for_browser", "checking"]);

export function isAntigravityLoginActive(snapshot: AntigravityLoginSnapshot | null): boolean {
  return snapshot ? ACTIVE_STATES.has(snapshot.state) : false;
}

function isMissingAttempt(error: unknown): boolean {
  return error instanceof ApiError && error.status === 404;
}

type LoginActionContext = {
  applySnapshot: (next: AntigravityLoginSnapshot | null) => void;
  attemptIdRef: MutableRefObject<string | null>;
  snapshotRef: MutableRefObject<AntigravityLoginSnapshot | null>;
  generationRef: MutableRefObject<number>;
  completedRef: MutableRefObject<boolean>;
  startPromiseRef: MutableRefObject<Promise<boolean> | null>;
  refreshPromiseRef: MutableRefObject<Promise<void> | null>;
  setBusy: (busy: boolean) => void;
  setError: (error: string | null) => void;
  t: (key: string) => string;
};

function useAntigravityLoginActions(ctx: LoginActionContext) {
  const {
    applySnapshot,
    attemptIdRef,
    snapshotRef,
    generationRef,
    completedRef,
    startPromiseRef,
    refreshPromiseRef,
    setBusy,
    setError,
    t,
  } = ctx;

  const start = useCallback(async (): Promise<boolean> => {
    const generation = generationRef.current + 1;
    generationRef.current = generation;
    attemptIdRef.current = null;
    completedRef.current = false;
    setBusy(true);
    setError(null);
    const promise = (async () => {
      try {
        const next = await startAntigravityLogin();
        if (generation !== generationRef.current) return false;
        attemptIdRef.current = next.attempt_id;
        applySnapshot(next);
        return true;
      } catch {
        if (generation === generationRef.current) setError(t("agents:antigravityLoginStartError"));
        return false;
      } finally {
        if (generation === generationRef.current) setBusy(false);
      }
    })();
    startPromiseRef.current = promise;
    return promise;
  }, [
    applySnapshot,
    attemptIdRef,
    completedRef,
    generationRef,
    setBusy,
    setError,
    startPromiseRef,
    t,
  ]);

  const cancel = useCallback(async (): Promise<boolean> => {
    const attemptID = attemptIdRef.current;
    if (!attemptID) return true;
    const generation = generationRef.current;
    setBusy(true);
    try {
      const next = await cancelAntigravityLogin(attemptID);
      if (generation === generationRef.current) {
        attemptIdRef.current = next.attempt_id;
        applySnapshot(next);
      }
      return true;
    } catch {
      if (generation === generationRef.current) setError(t("agents:antigravityLoginCancelError"));
      return false;
    } finally {
      if (generation === generationRef.current) setBusy(false);
    }
  }, [applySnapshot, attemptIdRef, generationRef, setBusy, setError, t]);

  const dismiss = useCallback(async (): Promise<boolean> => {
    const generation = generationRef.current;
    if (startPromiseRef.current) await startPromiseRef.current;
    if (refreshPromiseRef.current) await refreshPromiseRef.current;
    if (generation !== generationRef.current) return true;
    if (!isAntigravityLoginActive(snapshotRef.current)) return true;
    return cancel();
  }, [cancel, generationRef, refreshPromiseRef, snapshotRef, startPromiseRef]);

  return { start, cancel, dismiss };
}

function useAntigravityClipboard(url: string | undefined) {
  const [copied, setCopied] = useState(false);
  const copy = useCallback(async () => {
    if (!url) return;
    if (await copyToClipboard(url)) {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    }
  }, [url]);
  const openURL = useCallback(() => {
    if (url) window.open(url, "_blank", "noopener,noreferrer");
  }, [url]);
  return { copied, copy, openURL };
}

export function useAntigravityLogin({
  open,
  onComplete,
}: {
  open: boolean;
  onComplete?: () => void;
}) {
  const { t } = useTranslation();
  const [snapshot, setSnapshot] = useState<AntigravityLoginSnapshot | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const attemptIdRef = useRef<string | null>(null);
  const snapshotRef = useRef<AntigravityLoginSnapshot | null>(null);
  const onCompleteRef = useRef(onComplete);
  const generationRef = useRef(0);
  const refreshSequenceRef = useRef(0);
  const completedRef = useRef(false);
  const startPromiseRef = useRef<Promise<boolean> | null>(null);
  const refreshPromiseRef = useRef<Promise<void> | null>(null);

  useEffect(() => {
    onCompleteRef.current = onComplete;
  }, [onComplete]);

  const applySnapshot = useCallback((next: AntigravityLoginSnapshot | null) => {
    snapshotRef.current = next;
    setSnapshot(next);
  }, []);

  const handleRefreshFailure = useCallback(
    (err: unknown, requestedAttempt: string | null, generation: number, sequence: number) => {
      if (generation !== generationRef.current || sequence !== refreshSequenceRef.current) return;
      if (isMissingAttempt(err)) {
        attemptIdRef.current = null;
        applySnapshot(null);
        setError(requestedAttempt ? t("agents:antigravityLoginExpired") : null);
        return;
      }
      setError(t("agents:antigravityLoginStatusError"));
    },
    [applySnapshot, t],
  );

  const doRefresh = useCallback(async () => {
    const generation = generationRef.current;
    const sequence = ++refreshSequenceRef.current;
    const requestedAttempt = attemptIdRef.current;
    try {
      const next = await getAntigravityLogin(requestedAttempt ?? undefined);
      if (generation !== generationRef.current || sequence !== refreshSequenceRef.current) return;
      if (requestedAttempt && attemptIdRef.current !== requestedAttempt) return;
      if (!requestedAttempt && attemptIdRef.current) return;
      attemptIdRef.current = next.attempt_id;
      applySnapshot(next);
      setError(null);
      if (next.state === "succeeded" && !completedRef.current) {
        completedRef.current = true;
        onCompleteRef.current?.();
      }
    } catch (err) {
      handleRefreshFailure(err, requestedAttempt, generation, sequence);
    }
  }, [applySnapshot, handleRefreshFailure]);

  const refresh = useCallback(() => {
    const promise = doRefresh();
    refreshPromiseRef.current = promise;
    return promise;
  }, [doRefresh]);

  useEffect(() => {
    if (!open) return;
    generationRef.current += 1;
    completedRef.current = false;
    setError(null);
    void refresh();
  }, [open, refresh]);

  useEffect(() => {
    if (!open || !isAntigravityLoginActive(snapshot)) return;
    const timer = window.setInterval(() => void refresh(), 1000);
    return () => window.clearInterval(timer);
  }, [open, snapshot, refresh]);

  const { start, cancel, dismiss } = useAntigravityLoginActions({
    applySnapshot,
    attemptIdRef,
    snapshotRef,
    generationRef,
    completedRef,
    startPromiseRef,
    refreshPromiseRef,
    setBusy,
    setError,
    t,
  });

  const { copied, copy, openURL } = useAntigravityClipboard(snapshot?.authorization_url);
  return { snapshot, busy, error, copied, start, cancel, dismiss, copy, openURL };
}
