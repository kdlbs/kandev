"use client";

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { ApiError } from "@/lib/api/client";
// The module-level `t`, not `useTranslation`: every string below is produced
// inside an imperative callback (a toast, a refresh error), so resolving at call
// time is both correct and keeps `t` out of the callbacks' dependency arrays —
// a `t` dep on `useTerminalJobRefresh` would re-issue its network reload on a
// locale switch. Never call it at module scope; that would freeze the boot locale.
import { t } from "@/lib/i18n";
import {
  adoptStorageGoCache,
  analyzeStorage,
  deleteStorageQuarantine,
  fetchStorageOverview,
  fetchStorageDisk,
  fetchStoragePolicy,
  fetchStorageQuarantine,
  fetchStorageRuns,
  purgeStorageQuarantine,
  restoreStorageQuarantine,
  runStorageMaintenance,
  saveStorageSettings,
} from "@/lib/api/domains/system-api";
import type {
  StorageBusyResource,
  StorageBusyResponse,
  StorageMaintenanceSettings,
  StoragePolicyResponse,
  StorageQuarantinePurgeScope,
  SystemJob,
} from "@/lib/types/system";
import { useSystemJob } from "./use-system-jobs";

export type StoragePendingAction =
  | "save"
  | "analyze"
  | "run"
  | "adopt"
  | "restore"
  | "delete"
  | "purge"
  | null;

export interface StorageBusyState {
  resources: StorageBusyResource[];
  forceAvailable: boolean;
  resourceSelection?: string[];
}

function messageFromError(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function isTerminal(state?: string): boolean {
  return state === "succeeded" || state === "failed";
}

function busyStateFromError(error: unknown, resourceSelection?: string[]): StorageBusyState | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  const body = error.body as Partial<StorageBusyResponse> | null;
  if (!body || !Array.isArray(body.busy_resources)) return null;
  const resources = body.busy_resources.filter(
    (resource): resource is StorageBusyResource =>
      Boolean(resource) && typeof resource.kind === "string" && typeof resource.label === "string",
  );
  if (resources.length === 0) return null;
  return {
    resources,
    forceAvailable: body.force_available === true,
    resourceSelection,
  };
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

export type StorageSection = "policy" | "overview" | "disk" | "runs" | "quarantine";
export type StorageSectionLoading = Record<StorageSection, boolean>;
export type StorageSectionErrors = Record<StorageSection, string | null>;
type Reload = (sections?: StorageSection[]) => Promise<void>;
type SetStorageError = Dispatch<SetStateAction<string | null>>;
const TERMINAL_REFRESH_RETRY_MS = 1000;
const TERMINAL_REFRESH_MAX_RETRY_MS = 8000;
const MAX_TERMINAL_REFRESH_ATTEMPTS = 6;

function useStorageActionRunner() {
  const { toast } = useToast();
  const [pendingActions, setPendingActions] = useState<
    Array<{ id: number; action: Exclude<StoragePendingAction, null> }>
  >([]);
  const nextPendingActionId = useRef(0);
  const [error, setError] = useState<string | null>(null);
  const pendingAction = useMemo<StoragePendingAction>(() => {
    const policyAction = pendingActions.findLast(
      ({ action }) => action === "save" || action === "adopt",
    );
    return policyAction?.action ?? pendingActions[0]?.action ?? null;
  }, [pendingActions]);
  const perform = useCallback(
    async (
      action: Exclude<StoragePendingAction, null>,
      work: () => Promise<void>,
      rethrow = false,
    ) => {
      const pendingActionId = nextPendingActionId.current++;
      setPendingActions((current) => [...current, { id: pendingActionId, action }]);
      setError(null);
      try {
        await work();
      } catch (requestError) {
        const message = messageFromError(requestError);
        setError(message);
        toast({ title: t("system:storageActionFailed"), description: message, variant: "error" });
        if (rethrow) throw requestError;
      } finally {
        setPendingActions((current) => current.filter(({ id }) => id !== pendingActionId));
      }
    },
    [toast],
  );
  return { pendingAction, error, setError, perform };
}

function useStoragePolicyActions(
  perform: ReturnType<typeof useStorageActionRunner>["perform"],
  reload: Reload,
  toast: ReturnType<typeof useToast>["toast"],
  clearBusy: () => void,
  setPolicy: (policy: StoragePolicyResponse) => void,
) {
  const save = useCallback(
    async (settings: StorageMaintenanceSettings, confirmation?: "DEDICATED") => {
      clearBusy();
      return perform(
        "save",
        async () => {
          await saveStorageSettings(settings, confirmation);
          await reload(["policy"]);
          toast({ title: t("system:storageToastPolicySaved"), variant: "success" });
        },
        true,
      );
    },
    [clearBusy, perform, reload, toast],
  );

  const adopt = useCallback(
    async (path: string) => {
      clearBusy();
      return perform("adopt", async () => {
        const response = await adoptStorageGoCache(path);
        setPolicy(response);
        toast({ title: t("system:storageToastCacheAdopted"), variant: "success" });
      });
    },
    [clearBusy, perform, setPolicy, toast],
  );

  return { save, adopt };
}

function useStorageDeleteAction(
  perform: ReturnType<typeof useStorageActionRunner>["perform"],
  toast: ReturnType<typeof useToast>["toast"],
  clearBusy: () => void,
  setDeleteJobId: Dispatch<SetStateAction<string | null>>,
) {
  return useCallback(
    async (id: string) => {
      clearBusy();
      return perform("delete", async () => {
        const accepted = await deleteStorageQuarantine(id);
        setDeleteJobId(accepted.job_id);
        toast({ title: t("system:storageToastDeletionStarted"), variant: "success" });
      });
    },
    [clearBusy, perform, setDeleteJobId, toast],
  );
}

function useStorageBulkDeleteAction(
  perform: ReturnType<typeof useStorageActionRunner>["perform"],
  toast: ReturnType<typeof useToast>["toast"],
  clearBusy: () => void,
  setDeleteJobId: Dispatch<SetStateAction<string | null>>,
) {
  return useCallback(
    async (scope: StorageQuarantinePurgeScope) => {
      clearBusy();
      return perform("purge", async () => {
        const accepted = await purgeStorageQuarantine(scope);
        setDeleteJobId(accepted.job_id);
        toast({
          title:
            scope === "eligible"
              ? t("system:storageToastEligiblePurgeStarted")
              : t("system:storageToastForcedPurgeStarted"),
          variant: "success",
        });
      });
    },
    [clearBusy, perform, setDeleteJobId, toast],
  );
}

function useStorageActions(reload: Reload, setPolicy: (policy: StoragePolicyResponse) => void) {
  const { toast } = useToast();
  const { pendingAction, error, setError, perform } = useStorageActionRunner();
  const [analysisJobId, setAnalysisJobId] = useState<string | null>(null);
  const [cleanupJobId, setCleanupJobId] = useState<string | null>(null);
  const [deleteJobId, setDeleteJobId] = useState<string | null>(null);
  const [busy, setBusy] = useState<StorageBusyState | null>(null);
  const analysisJob = useSystemJob(analysisJobId);
  const cleanupJob = useSystemJob(cleanupJobId);
  const deleteJob = useSystemJob(deleteJobId);
  const clearBusy = useCallback(() => setBusy(null), []);
  const { save, adopt } = useStoragePolicyActions(perform, reload, toast, clearBusy, setPolicy);

  const analyze = useCallback(async () => {
    clearBusy();
    return perform("analyze", async () => {
      const accepted = await analyzeStorage();
      setAnalysisJobId(accepted.job_id);
      toast({ title: t("system:storageToastAnalysisStarted"), variant: "success" });
    });
  }, [clearBusy, perform, toast]);

  const runNow = useCallback(
    async (resources?: string[]) => {
      setCleanupJobId(null);
      clearBusy();
      return perform("run", async () => {
        try {
          const accepted = await runStorageMaintenance(resources);
          setCleanupJobId(accepted.job_id);
          toast({ title: t("system:storageToastMaintenanceStarted"), variant: "success" });
        } catch (error) {
          const nextBusy = busyStateFromError(error, resources);
          if (nextBusy) {
            setBusy(nextBusy);
            return;
          }
          throw error;
        }
      });
    },
    [clearBusy, perform, toast],
  );
  const runAnyway = useCallback(async () => {
    if (!busy?.forceAvailable) return;
    const resources = busy.resourceSelection;
    clearBusy();
    setCleanupJobId(null);
    return perform("run", async () => {
      try {
        const accepted = await runStorageMaintenance(resources, true);
        setCleanupJobId(accepted.job_id);
        toast({ title: t("system:storageToastMaintenanceStarted"), variant: "success" });
      } catch (error) {
        const nextBusy = busyStateFromError(error, resources);
        if (nextBusy) {
          setBusy(nextBusy);
          return;
        }
        throw error;
      }
    });
  }, [busy, clearBusy, perform, toast]);

  const restore = useCallback(
    async (id: string) => {
      clearBusy();
      return perform("restore", async () => {
        await restoreStorageQuarantine(id);
        await reload(["quarantine"]);
        toast({ title: t("system:storageToastQuarantineRestored"), variant: "success" });
      });
    },
    [clearBusy, perform, reload, toast],
  );
  const permanentlyDelete = useStorageDeleteAction(perform, toast, clearBusy, setDeleteJobId);
  const purge = useStorageBulkDeleteAction(perform, toast, clearBusy, setDeleteJobId);

  return {
    pendingAction,
    error,
    setError,
    save,
    analyze,
    runNow,
    runAnyway,
    busy,
    adopt,
    restore,
    permanentlyDelete,
    clearEligible: useCallback(() => purge("eligible"), [purge]),
    forceClearAll: useCallback(() => purge("all"), [purge]),
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
        refreshError = t("system:storageRefreshFailed", {
          message: messageFromError(requestError),
        });
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
  const setPolicy = useAppStore((state) => state.setSystemStoragePolicy);
  const setOverview = useAppStore((state) => state.setSystemStorageOverview);
  const setDisk = useAppStore((state) => state.setSystemStorageDisk);
  const setRuns = useAppStore((state) => state.setSystemStorageRuns);
  const setQuarantine = useAppStore((state) => state.setSystemStorageQuarantine);
  const [loading, setLoading] = useState<StorageSectionLoading>({
    policy: true,
    overview: true,
    disk: true,
    runs: true,
    quarantine: true,
  });
  const [sectionErrors, setSectionErrors] = useState<StorageSectionErrors>({
    policy: null,
    overview: null,
    disk: null,
    runs: null,
    quarantine: null,
  });
  const sectionGenerations = useRef<Record<StorageSection, number>>({
    policy: 0,
    overview: 0,
    disk: 0,
    runs: 0,
    quarantine: 0,
  });
  const loadSection = useCallback(
    async <T>(section: StorageSection, request: () => Promise<T>, commit: (value: T) => void) => {
      const generation = ++sectionGenerations.current[section];
      setLoading((current) => ({ ...current, [section]: true }));
      setSectionErrors((current) => ({ ...current, [section]: null }));
      try {
        const value = await request();
        if (generation === sectionGenerations.current[section]) commit(value);
      } catch (requestError) {
        if (generation === sectionGenerations.current[section]) {
          setSectionErrors((current) => ({
            ...current,
            [section]: messageFromError(requestError),
          }));
          throw requestError;
        }
      } finally {
        if (generation === sectionGenerations.current[section]) {
          setLoading((current) => ({ ...current, [section]: false }));
        }
      }
    },
    [],
  );
  const reload = useCallback(
    async (sections: StorageSection[] = ["policy", "overview", "disk", "runs", "quarantine"]) => {
      const jobs = sections.map((section) => {
        switch (section) {
          case "policy":
            return loadSection("policy", fetchStoragePolicy, setPolicy);
          case "overview":
            return loadSection("overview", fetchStorageOverview, setOverview);
          case "disk":
            return loadSection("disk", fetchStorageDisk, setDisk);
          case "runs":
            return loadSection("runs", () => fetchStorageRuns(20), setRuns);
          case "quarantine":
            return loadSection("quarantine", fetchStorageQuarantine, setQuarantine);
        }
      });
      const results = await Promise.allSettled(jobs);
      const failure = results.find(
        (result): result is PromiseRejectedResult => result.status === "rejected",
      );
      if (failure) throw failure.reason;
    },
    [loadSection, setDisk, setOverview, setPolicy, setQuarantine, setRuns],
  );
  const commitAdoptedPolicy = useCallback(
    (policy: StoragePolicyResponse) => {
      sectionGenerations.current.policy += 1;
      setPolicy(policy);
      setSectionErrors((current) => ({ ...current, policy: null }));
      setLoading((current) => ({ ...current, policy: false }));
    },
    [setPolicy],
  );
  const actions = useStorageActions(reload, commitAdoptedPolicy);

  useEffect(() => {
    void reload().catch(() => undefined);
  }, [reload]);
  useReloadCompletedJobs(
    reload,
    actions.setError,
    actions.analysisJob,
    actions.cleanupJob,
    actions.deleteJob,
  );
  return { ...storage, ...actions, loading, sectionErrors, reload };
}
