"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { fetchSystemInfo, requestRestart } from "@/lib/api/domains/system-api";
// The module-level `t` (not the hook) so these resolve when the callback fires
// and stay out of the callbacks' dependency arrays. This file has no JSX, so
// `mode: "jsx-only"` can never see the strings below.
import { t } from "@/lib/i18n";

export type KandevRestartPhase = "idle" | "starting" | "restarting" | "done" | "error";

const POLL_INTERVAL_MS = 2000;
const MAX_DURATION_MS = 3 * 60 * 1000;

type UseKandevRestartArgs = {
  onComplete?: () => void;
};

export type KandevRestartController = {
  phase: KandevRestartPhase;
  errorMessage: string | null;
  isRestarting: boolean;
  start: () => Promise<void>;
  dismiss: () => void;
};

export function useKandevRestart({
  onComplete,
}: UseKandevRestartArgs = {}): KandevRestartController {
  const [phase, setPhase] = useState<KandevRestartPhase>("idle");
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [previousBootID, setPreviousBootID] = useState<string | null>(null);
  const activeRef = useRef(false);
  const startedAtRef = useRef<number | null>(null);

  const fail = useCallback((message: string) => {
    activeRef.current = false;
    setErrorMessage(message);
    setPhase("error");
  }, []);

  const start = useCallback(async () => {
    if (activeRef.current) return;
    activeRef.current = true;
    setErrorMessage(null);
    setPhase("starting");
    try {
      const before = await fetchSystemInfo({ cache: "no-store" });
      setPreviousBootID(before.boot_id);
      startedAtRef.current = Date.now();
      await requestRestart();
      setPhase("restarting");
    } catch (e) {
      fail(e instanceof Error ? e.message : t("system:restartStartFailed"));
    }
  }, [fail]);

  const dismiss = useCallback(() => {
    activeRef.current = false;
    setPhase("idle");
    setErrorMessage(null);
    setPreviousBootID(null);
    startedAtRef.current = null;
  }, []);

  useEffect(() => {
    if (phase !== "restarting") return;
    if (!previousBootID) return;
    let cancelled = false;

    const tick = async () => {
      const startedAt = startedAtRef.current ?? Date.now();
      if (Date.now() - startedAt > MAX_DURATION_MS) {
        if (!cancelled) {
          fail(t("system:restartTimedOut"));
        }
        return;
      }
      try {
        const info = await fetchSystemInfo({ cache: "no-store" });
        if (cancelled) return;
        if (info.boot_id && info.boot_id !== previousBootID) {
          activeRef.current = false;
          setPhase("done");
          onComplete?.();
        }
      } catch {
        // Backend unreachable is expected while the supervised process exits
        // and starts again. Keep polling until the boot ID changes.
      }
    };

    void tick();
    const interval = setInterval(() => void tick(), POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [phase, previousBootID, onComplete, fail]);

  return {
    phase,
    errorMessage,
    isRestarting: phase === "starting" || phase === "restarting",
    start,
    dismiss,
  };
}
