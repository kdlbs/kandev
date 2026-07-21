"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import {
  adoptStorageGoCache,
  analyzeStorage,
  deleteStorageQuarantine,
  fetchStorageOverview,
  fetchStorageQuarantine,
  fetchStorageRuns,
  restoreStorageQuarantine,
  runStorageMaintenance,
  saveStorageSettings,
} from "@/lib/api/domains/system-api";
import type { StorageMaintenanceSettings, SystemJob } from "@/lib/types/system";
import { useSystemJob } from "./use-system-jobs";

export type StoragePendingAction =
  | "load"
  | "save"
  | "analyze"
  | "run"
  | "adopt"
  | "restore"
  | "delete"
  | null;

function messageFromError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function isTerminal(state?: string): boolean {
  return state === "succeeded" || state === "failed";
}

export function settingsWithDockerAcknowledgement(
  settings: StorageMaintenanceSettings,
  acknowledged: boolean,
): StorageMaintenanceSettings {
  return {
    ...settings,
    docker: {
      ...settings.docker,
      dedicated_daemon_acknowledged: acknowledged,
      build_cache_enabled: acknowledged && settings.docker.build_cache_enabled,
      unused_images_enabled: acknowledged && settings.docker.unused_images_enabled,
    },
  };
}

type Reload = () => Promise<void>;
type SetStorageError = Dispatch<SetStateAction<string | null>>;
const TERMINAL_REFRESH_RETRY_MS = 1000;
const TERMINAL_REFRESH_MAX_RETRY_MS = 8000;
const MAX_TERMINAL_REFRESH_ATTEMPTS = 6;

function useStorageActionRunner() {
  const { toast } = useToast();
  const [pendingAction, setPendingAction] = useState<StoragePendingAction>("load");
  const [error, setError] = useState<string | null>(null);
  const finishLoading = useCallback(() => setPendingAction(null), []);
  const perform = useCallback(
    async (
      action: Exclude<StoragePendingAction, "load" | null>,
      work: () => Promise<void>,
      rethrow = false,
    ) => {
      setPendingAction(action);
      setError(null);
      try {
        await work();
      } catch (requestError) {
        const message = messageFromError(requestError);
        setError(message);
        toast({ title: "Storage action failed", description: message, variant: "error" });
        if (rethrow) throw requestError;
      } finally {
        setPendingAction(null);
      }
    },
    [toast],
  );
  return { pendingAction, error, setError, finishLoading, perform };
}

function useStorageActions(reload: Reload) {
  const { toast } = useToast();
  const { pendingAction, error, setError, finishLoading, perform } = useStorageActionRunner();
  const [analysisJobId, setAnalysisJobId] = useState<string | null>(null);
  const [cleanupJobId, setCleanupJobId] = useState<string | null>(null);
  const [deleteJobId, setDeleteJobId] = useState<string | null>(null);
  const analysisJob = useSystemJob(analysisJobId);
  const cleanupJob = useSystemJob(cleanupJobId);
  const deleteJob = useSystemJob(deleteJobId);

  const save = useCallback(
    async (settings: StorageMaintenanceSettings, confirmation?: "DEDICATED") =>
      perform(
        "save",
        async () => {
          await saveStorageSettings(settings, confirmation);
          await reload();
          toast({ title: "Storage policy saved", variant: "success" });
        },
        true,
      ),
    [perform, reload, toast],
  );

  const adopt = useCallback(
    async (path: string) =>
      perform("adopt", async () => {
        await adoptStorageGoCache(path);
        await reload();
        toast({ title: "Go build cache adopted", variant: "success" });
      }),
    [perform, reload, toast],
  );

  const analyze = useCallback(
    async () =>
      perform("analyze", async () => {
        const accepted = await analyzeStorage();
        setAnalysisJobId(accepted.job_id);
        toast({ title: "Storage analysis started", variant: "success" });
      }),
    [perform, toast],
  );

  const runNow = useCallback(
    async (resources?: string[]) => {
      setCleanupJobId(null);
      return perform("run", async () => {
        const accepted = await runStorageMaintenance(resources);
        setCleanupJobId(accepted.job_id);
        toast({ title: "Storage maintenance started", variant: "success" });
      });
    },
    [perform, toast],
  );

  const restore = useCallback(
    async (id: string) =>
      perform("restore", async () => {
        await restoreStorageQuarantine(id);
        await reload();
        toast({ title: "Quarantined resource restored", variant: "success" });
      }),
    [perform, reload, toast],
  );

  const permanentlyDelete = useCallback(
    async (id: string) =>
      perform("delete", async () => {
        const accepted = await deleteStorageQuarantine(id);
        setDeleteJobId(accepted.job_id);
        toast({ title: "Permanent deletion started", variant: "success" });
      }),
    [perform, toast],
  );

  return {
    pendingAction,
    error,
    finishLoading,
    setError,
    save,
    analyze,
    runNow,
    adopt,
    restore,
    permanentlyDelete,
    analysisJob,
    cleanupJob,
    deleteJob,
  };
}

function useTerminalJobRefresh(reload: Reload, setError: SetStorageError, job?: SystemJob) {
  const terminalKey = job && isTerminal(job.state) ? `${job.id}:${job.state}` : "";
  useEffect(() => {
    if (!terminalKey) return;
    let cancelled = false;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let refreshError: string | null = null;
    let attempts = 0;
    const refresh = async () => {
      try {
        await reload();
        if (cancelled || !refreshError) return;
        const resolvedError = refreshError;
        setError((current) => (current === resolvedError ? null : current));
      } catch (requestError) {
        if (cancelled) return;
        refreshError = `Refresh storage data: ${messageFromError(requestError)}`;
        setError(refreshError);
        attempts += 1;
        if (attempts >= MAX_TERMINAL_REFRESH_ATTEMPTS) return;
        const retryDelay = Math.min(
          TERMINAL_REFRESH_RETRY_MS * 2 ** (attempts - 1),
          TERMINAL_REFRESH_MAX_RETRY_MS,
        );
        retryTimer = setTimeout(() => void refresh(), retryDelay);
      }
    };
    void refresh();
    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
    };
  }, [reload, setError, terminalKey]);
}

function useReloadCompletedJobs(
  reload: Reload,
  setError: SetStorageError,
  analysisJob?: SystemJob,
  cleanupJob?: SystemJob,
  deleteJob?: SystemJob,
) {
  useTerminalJobRefresh(reload, setError, analysisJob);
  useTerminalJobRefresh(reload, setError, cleanupJob);
  useTerminalJobRefresh(reload, setError, deleteJob);
}

export function useStorageMaintenance() {
  const storage = useAppStore((state) => state.system.storage);
  const setOverview = useAppStore((state) => state.setSystemStorageOverview);
  const setRuns = useAppStore((state) => state.setSystemStorageRuns);
  const setQuarantine = useAppStore((state) => state.setSystemStorageQuarantine);
  const reloadGeneration = useRef(0);
  const reload = useCallback(async () => {
    const generation = ++reloadGeneration.current;
    const [overview, runs, quarantine] = await Promise.all([
      fetchStorageOverview(),
      fetchStorageRuns(20),
      fetchStorageQuarantine(),
    ]);
    if (generation !== reloadGeneration.current) return;
    setOverview(overview);
    setRuns(runs);
    setQuarantine(quarantine);
  }, [setOverview, setQuarantine, setRuns]);
  const { finishLoading, setError, ...actions } = useStorageActions(reload);

  useEffect(() => {
    void reload()
      .catch((requestError) => setError(messageFromError(requestError)))
      .finally(finishLoading);
  }, [finishLoading, reload, setError]);
  useReloadCompletedJobs(
    reload,
    setError,
    actions.analysisJob,
    actions.cleanupJob,
    actions.deleteJob,
  );
  return { ...storage, ...actions, reload };
}
