"use client";

import { useEffect, useRef, useState } from "react";
import { ApiError, fetchJson } from "@/lib/api/client";
import { parseStartupSnapshot, type StartupSnapshot } from "@/lib/startup-progress/types";

const POLL_MS = 1000;
const TIMEOUT_MS = 5000;

type ReadyBody = {
  startup?: unknown;
};

export type StartupProgressState = {
  snapshot: StartupSnapshot | null;
  lastKnown: boolean;
};

function readyBodyOf(err: unknown): ReadyBody | null {
  if (!(err instanceof ApiError) || !err.body || typeof err.body !== "object") return null;
  return err.body as ReadyBody;
}

/**
 * Polls GET /ready for step-level startup detail while `enabled`. This is
 * additive to, and independent of, useKandevRestart's boot_id-based
 * completion polling of /api/v1/system/info — that loop is left untouched,
 * this one only feeds display detail into RestartProgressDialog.
 *
 * Mirrors the cadence and fallback rules of the Go startup page's embedded
 * poll script (AC-PLATFORM-STARTUP-PROGRESS-003): one request in flight,
 * abandoned after TIMEOUT_MS, any tick skipped while one is outstanding
 * (AC-002.2). A response this hook cannot parse a snapshot out of resolves
 * by the status code, not by assuming staleness (AC-002.6/003.13): a
 * successful status with no snapshot means ready, so it clears "last known"
 * without touching the last-rendered detail; a failing status falls back to
 * "last known" instead. The 503 "still starting" response carries a
 * parseable snapshot either way and is therefore always treated as fresh
 * data, not a failure.
 */
export function useStartupProgress(enabled: boolean): StartupProgressState {
  const [snapshot, setSnapshot] = useState<StartupSnapshot | null>(null);
  const [lastKnown, setLastKnown] = useState(false);
  const inFlightRef = useRef(false);
  const activeControllerRef = useRef<AbortController | null>(null);

  useEffect(() => {
    if (!enabled) {
      setSnapshot(null);
      setLastKnown(false);
      activeControllerRef.current?.abort();
      activeControllerRef.current = null;
      inFlightRef.current = false;
      return;
    }
    let cancelled = false;

    const applySnapshot = (ok: boolean, body: ReadyBody | null) => {
      const parsed = parseStartupSnapshot(body?.startup);
      if (!parsed) {
        // AC-PLATFORM-STARTUP-PROGRESS-002.6: a response with no parseable
        // snapshot resolves by the status code, not by assuming staleness -
        // a successful status means ready, only a failing one falls back to
        // last-known.
        setLastKnown(!ok);
        return;
      }
      setLastKnown(false);
      setSnapshot(parsed);
    };

    const tick = async () => {
      if (inFlightRef.current) return;
      inFlightRef.current = true;
      const controller = new AbortController();
      activeControllerRef.current = controller;
      const timeoutId = setTimeout(() => controller.abort(), TIMEOUT_MS);
      try {
        const body = await fetchJson<ReadyBody>("/ready", {
          cache: "no-store",
          init: { signal: controller.signal },
        });
        if (!cancelled) applySnapshot(true, body);
      } catch (err) {
        const ok = err instanceof ApiError && err.status >= 200 && err.status < 300;
        if (!cancelled) applySnapshot(ok, readyBodyOf(err));
      } finally {
        clearTimeout(timeoutId);
        if (activeControllerRef.current === controller) {
          activeControllerRef.current = null;
          inFlightRef.current = false;
        }
      }
    };

    void tick();
    const interval = setInterval(() => void tick(), POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
      activeControllerRef.current?.abort();
      activeControllerRef.current = null;
      inFlightRef.current = false;
    };
  }, [enabled]);

  return { snapshot, lastKnown };
}
