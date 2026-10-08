"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  bindCursorCloudSubmissionCandidate,
  getCursorCloudSubmissionResolution,
  retryCursorCloudSubmission,
  type CursorCloudSubmissionResolution,
} from "@/lib/api/domains/cursor-cloud-api";
import type { CursorCloudSubmissionRecoveryState } from "./cursor-cloud-submission-recovery";

function submissionState(state: string): CursorCloudSubmissionRecoveryState {
  if (state === "unknown") return "unknown";
  if (
    state === "reserved" ||
    state === "submitting" ||
    state === "accepted" ||
    state === "cancelling"
  ) {
    return "pending";
  }
  return "ready";
}

export function useCursorCloudSubmissionResolution({
  taskId,
  sessionId,
  latestUserMessageId,
  retrySessionStatus,
}: {
  taskId: string;
  sessionId: string | null;
  latestUserMessageId: string | null;
  retrySessionStatus: () => Promise<void>;
}) {
  const { t } = useTranslation();
  const [resolution, setResolution] = useState<CursorCloudSubmissionResolution | null>(null);
  const [state, setState] = useState<CursorCloudSubmissionRecoveryState>(
    sessionId ? "loading" : "ready",
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const resolutionRef = useRef<CursorCloudSubmissionResolution | null>(null);
  const latestUserMessageIdRef = useRef<string | null>(null);

  const refresh = useCallback(
    async (showLoading = true) => {
      if (!sessionId) {
        setResolution(null);
        setState("ready");
        return;
      }
      if (showLoading) setState("loading");
      setError(null);
      try {
        const result = await getCursorCloudSubmissionResolution(taskId, sessionId);
        resolutionRef.current = result;
        setResolution(result);
        setState(submissionState(result.state));
        return result;
      } catch {
        setError(t("executors:cursorCloudSubmissionCheckFailed"));
        setState("error");
        return null;
      }
    },
    [sessionId, t, taskId],
  );

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (state !== "unknown" && state !== "pending") return;
    const timer = window.setInterval(() => void refresh(false), 2500);
    return () => window.clearInterval(timer);
  }, [refresh, state]);

  useEffect(() => {
    const previousMessageId = latestUserMessageIdRef.current;
    latestUserMessageIdRef.current = latestUserMessageId;
    if (
      !sessionId ||
      !latestUserMessageId ||
      !previousMessageId ||
      previousMessageId === latestUserMessageId
    ) {
      return;
    }

    const previousOperationId = resolutionRef.current?.operationId;
    if (!previousOperationId) return;

    let cancelled = false;
    const refreshForNewOperation = async () => {
      for (let attempt = 0; attempt < 20 && !cancelled; attempt += 1) {
        const result = await refresh(false);
        if (result && result.operationId !== previousOperationId) return;
        await new Promise<void>((resolve) => window.setTimeout(resolve, 500));
      }
    };
    void refreshForNewOperation();
    return () => {
      cancelled = true;
    };
  }, [latestUserMessageId, refresh, sessionId]);

  const actions = useCursorCloudResolutionActions({
    taskId,
    sessionId,
    refresh,
    retrySessionStatus,
    setBusy,
    setError,
    setState,
    t,
  });

  return { resolution, state, busy, error, refresh, ...actions };
}

function useCursorCloudResolutionActions({
  taskId,
  sessionId,
  refresh,
  retrySessionStatus,
  setBusy,
  setError,
  setState,
  t,
}: {
  taskId: string;
  sessionId: string | null;
  refresh: (showLoading?: boolean) => Promise<CursorCloudSubmissionResolution | null | undefined>;
  retrySessionStatus: () => Promise<void>;
  setBusy: (busy: boolean) => void;
  setError: (error: string | null) => void;
  setState: (state: CursorCloudSubmissionRecoveryState) => void;
  t: (key: string) => string;
}) {
  const bind = useCallback(
    async (runId: string) => {
      if (!sessionId) return;
      setBusy(true);
      try {
        await bindCursorCloudSubmissionCandidate(taskId, sessionId, runId);
        await refresh();
        await retrySessionStatus();
      } catch {
        setError(t("executors:cursorCloudSubmissionCheckFailed"));
        setState("error");
      } finally {
        setBusy(false);
      }
    },
    [refresh, retrySessionStatus, sessionId, t, taskId],
  );

  const retry = useCallback(
    async (operationId: string) => {
      if (!sessionId) return;
      setBusy(true);
      try {
        await retryCursorCloudSubmission(taskId, sessionId, operationId, true);
        await refresh();
      } catch {
        setError(t("executors:cursorCloudSubmissionCheckFailed"));
        setState("error");
      } finally {
        setBusy(false);
      }
    },
    [refresh, sessionId, t, taskId],
  );

  return { bind, retry };
}
