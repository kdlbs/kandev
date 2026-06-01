"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { applyUpdate, fetchSystemInfo, fetchSystemJob } from "@/lib/api/domains/system-api";

/**
 * Lifecycle of an in-UI self-update, from the user's perspective:
 *
 *   idle → starting → installing → restarting → done
 *                          └──────────────────→ error
 *
 * The backend's apply *job* finishes in ~1s — it only means "the out-of-process
 * helper was launched". The real work (npm/brew upgrade, service reinstall,
 * service restart) happens in that detached helper, and the backend serving
 * this page is itself restarted partway through. So the only trustworthy signal
 * that the update actually landed is `/system/info` reporting the target
 * version. This hook drives the whole flow off that: poll the version, treat an
 * unreachable backend as "restarting" (expected), and only call it done once the
 * version flips. State is mirrored to localStorage so a page reload during the
 * restart window resumes the progress view instead of re-offering the button.
 */
export type SelfUpdatePhase = "idle" | "starting" | "installing" | "restarting" | "done" | "error";

const STORAGE_KEY = "kandev.selfUpdate";
const POLL_INTERVAL_MS = 2000;
// Safety net: if the version never flips (helper died after launch, network
// wedged), stop polling and surface an error instead of spinning forever.
const MAX_DURATION_MS = 5 * 60 * 1000;

type PersistedUpdate = { target: string; startedAt: number };

function readPersisted(): PersistedUpdate | null {
  if (typeof window === "undefined") return null;
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as PersistedUpdate;
    if (typeof parsed.target !== "string" || typeof parsed.startedAt !== "number") return null;
    return parsed;
  } catch {
    return null;
  }
}

function writePersisted(value: PersistedUpdate): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
  } catch {
    // localStorage may be unavailable (private mode/quota); progress still
    // works in-memory, only the reload-resume affordance is lost.
  }
}

function clearPersisted(): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // ignore
  }
}

type UseSelfUpdateArgs = {
  latestVersion: string | undefined;
  /** Called once the version flip is confirmed, to refresh the updates view. */
  onComplete?: () => void;
};

export type SelfUpdateController = {
  phase: SelfUpdatePhase;
  targetVersion: string | null;
  errorMessage: string | null;
  /** True while an update is in flight (button should be hidden/disabled). */
  isUpdating: boolean;
  start: () => Promise<void>;
  dismiss: () => void;
};

export function useSelfUpdate({
  latestVersion,
  onComplete,
}: UseSelfUpdateArgs): SelfUpdateController {
  const [phase, setPhase] = useState<SelfUpdatePhase>("idle");
  // Lazy-init from localStorage (SSR-safe — null on the server). It isn't
  // rendered until `phase` leaves "idle", so the value differing between server
  // and client can't cause a hydration mismatch.
  const [targetVersion, setTargetVersion] = useState<string | null>(
    () => readPersisted()?.target ?? null,
  );
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [jobId, setJobId] = useState<string | null>(null);
  const hydrated = useRef(false);

  const fail = useCallback((message: string) => {
    setErrorMessage(message);
    setPhase("error");
    clearPersisted();
  }, []);

  const start = useCallback(async () => {
    if (!latestVersion) return;
    setErrorMessage(null);
    setTargetVersion(latestVersion);
    setPhase("starting");
    writePersisted({ target: latestVersion, startedAt: Date.now() });
    try {
      const res = await applyUpdate("UPDATE");
      setJobId(res.job_id);
      setPhase("installing");
    } catch (e) {
      fail(e instanceof Error ? e.message : "Failed to start the update");
    }
  }, [latestVersion, fail]);

  const dismiss = useCallback(() => {
    setPhase("idle");
    setErrorMessage(null);
    setJobId(null);
    clearPersisted();
  }, []);

  // Resume an in-progress update after a page reload (the restart window can
  // outlive a manual refresh). Runs once; the version poll below confirms done.
  useEffect(() => {
    if (hydrated.current) return;
    hydrated.current = true;
    const persisted = readPersisted();
    if (!persisted) return;
    if (Date.now() - persisted.startedAt > MAX_DURATION_MS) {
      clearPersisted();
      return;
    }
    // Resume an in-progress update after a page reload. Promoting out of "idle"
    // here (rather than in the initial state) keeps the first client render
    // matching the server's; the version poll below then confirms completion.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setPhase("installing");
  }, []);

  // Drive the flow off the live version. While installing/restarting, poll
  // /system/info: success at the target version → done; an unreachable backend
  // → restarting (expected); a failed launch job → error.
  useEffect(() => {
    if (phase !== "installing" && phase !== "restarting") return;
    if (!targetVersion) return;
    const startedAt = readPersisted()?.startedAt ?? Date.now();
    let cancelled = false;

    const tick = async () => {
      if (Date.now() - startedAt > MAX_DURATION_MS) {
        if (!cancelled) {
          fail("Update is taking longer than expected. Refresh to check the current version.");
        }
        return;
      }
      if (await launchFailed(jobId)) {
        if (!cancelled) fail("The update helper failed to start. Check the service logs.");
        return;
      }
      try {
        const info = await fetchSystemInfo({ cache: "no-store" });
        if (cancelled) return;
        if (info.version === targetVersion) {
          setPhase("done");
          clearPersisted();
          onComplete?.();
        }
      } catch {
        // Backend unreachable = it's restarting. Never downgrade out of this.
        if (!cancelled) setPhase((p) => (p === "installing" ? "restarting" : p));
      }
    };

    void tick();
    const interval = setInterval(() => void tick(), POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [phase, targetVersion, jobId, onComplete, fail]);

  return {
    phase,
    targetVersion,
    errorMessage,
    isUpdating: phase === "starting" || phase === "installing" || phase === "restarting",
    start,
    dismiss,
  };
}

/**
 * Returns true only when the launch job is known to have failed. A 404 (job
 * tracker is in-memory and wiped on restart) or network error is treated as
 * "not failed" so we keep waiting for the version flip.
 */
async function launchFailed(jobId: string | null): Promise<boolean> {
  if (!jobId) return false;
  try {
    const job = await fetchSystemJob(jobId);
    return job.state === "failed";
  } catch {
    return false;
  }
}
