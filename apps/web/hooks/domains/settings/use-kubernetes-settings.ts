"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStoreApi } from "@/components/state-provider";
import {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
  fetchExecutor,
  updateExecutor,
} from "@/lib/api/domains/settings-api";
import {
  getKubernetesSessionImpact,
  listKubernetesSessions,
  testKubernetesConnection,
} from "@/lib/api/domains/kubernetes-api";
import type { Executor } from "@/lib/types/http";
import type {
  KubernetesSession,
  KubernetesTestRequest,
  KubernetesTestResult,
} from "@/lib/types/http-kubernetes";
import { useIsAdmin } from "@/hooks/domains/auth/use-is-admin";

export { kubernetesExecutorSettingsPath } from "@/lib/settings/executor-settings-routes";

const SESSION_REFRESH_INTERVAL_MS = 90_000;

export function useKubernetesAdminAccess(): boolean {
  return useIsAdmin();
}

export function useKubernetesDiagnostics() {
  const [testing, setTesting] = useState(false);
  const [result, setResult] = useState<KubernetesTestResult | null>(null);
  const [error, setError] = useState<unknown>(null);
  const generation = useRef(0);

  const run = useCallback(async (request: KubernetesTestRequest) => {
    const current = ++generation.current;
    setTesting(true);
    setError(null);
    try {
      const response = await testKubernetesConnection(request);
      if (generation.current === current) setResult(response);
      return response;
    } catch (cause) {
      if (generation.current === current) {
        setResult(null);
        setError(cause);
      }
      throw cause;
    } finally {
      if (generation.current === current) setTesting(false);
    }
  }, []);

  const clear = useCallback(() => {
    generation.current += 1;
    setTesting(false);
    setResult(null);
    setError(null);
  }, []);

  return { testing, result, error, run, clear };
}

export function useKubernetesSessions(executorId: string, enabled = true) {
  const [sessions, setSessions] = useState<KubernetesSession[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const generation = useRef(0);

  const refresh = useCallback(async (): Promise<KubernetesSession[]> => {
    if (!enabled) return [];
    const current = ++generation.current;
    setLoading(true);
    setError(null);
    try {
      const rows = await listKubernetesSessions(executorId, { cache: "no-store" });
      if (generation.current === current) setSessions(rows);
      return rows;
    } catch (cause) {
      if (generation.current === current) setError(cause);
      throw cause;
    } finally {
      if (generation.current === current) setLoading(false);
    }
  }, [enabled, executorId]);

  useEffect(() => {
    setSessions([]);
    setError(null);
    if (!enabled) {
      setLoading(false);
      generation.current += 1;
      return;
    }
    void refresh().catch(() => undefined);
    const interval = window.setInterval(
      () => void refresh().catch(() => undefined),
      SESSION_REFRESH_INTERVAL_MS,
    );
    return () => {
      window.clearInterval(interval);
      generation.current += 1;
    };
  }, [enabled, refresh]);

  return { sessions, loading, error, refresh };
}

export function useKubernetesSessionImpact(executorId: string, enabled = true) {
  const loadActiveSessionCount = useCallback(async (): Promise<number> => {
    if (!enabled) return 0;
    const impact = await getKubernetesSessionImpact(executorId, { cache: "no-store" });
    return impact.active_session_count;
  }, [enabled, executorId]);

  return { loadActiveSessionCount };
}

type KubernetesExecutorRecord = Pick<Executor, "id" | "name" | "type" | "config">;

export type KubernetesExecutorCreateInput = {
  name: string;
  config: Record<string, string>;
  profileName: string;
  profileConfig: Record<string, string>;
};

type KubernetesCreateDependencies = {
  createExecutor: typeof createExecutor;
  createExecutorProfile: typeof createExecutorProfile;
  deleteExecutor: typeof deleteExecutor;
};

const KUBERNETES_CREATE_DEPENDENCIES: KubernetesCreateDependencies = {
  createExecutor,
  createExecutorProfile,
  deleteExecutor,
};

export class KubernetesExecutorRollbackRejectedError extends Error {
  constructor() {
    super();
    // i18n-exempt: diagnostic error identity; the UI localizes this error by type.
    this.name = "KubernetesExecutorRollbackRejectedError";
  }
}

export async function createKubernetesExecutorPair(
  input: KubernetesExecutorCreateInput,
  dependencies: KubernetesCreateDependencies = KUBERNETES_CREATE_DEPENDENCIES,
) {
  const executor = await dependencies.createExecutor({
    name: input.name,
    type: "k8s",
    config: input.config,
  });
  try {
    const profile = await dependencies.createExecutorProfile(executor.id, {
      name: input.profileName,
      config: input.profileConfig,
    });
    return { executor, profile };
  } catch (profileError) {
    try {
      const rollback = await dependencies.deleteExecutor(executor.id);
      if (!rollback.success) throw new KubernetesExecutorRollbackRejectedError();
    } catch (rollbackError) {
      throw new AggregateError([profileError, rollbackError]);
    }
    throw profileError;
  }
}

export function useKubernetesExecutorResource(executorId?: string) {
  const store = useAppStoreApi();
  const [executor, setExecutor] = useState<KubernetesExecutorRecord | null>(null);
  const [loading, setLoading] = useState(Boolean(executorId));
  const [error, setError] = useState<unknown>(null);
  const generation = useRef(0);

  const reload = useCallback(async () => {
    if (!executorId) return null;
    const current = ++generation.current;
    setLoading(true);
    setError(null);
    try {
      const response = (await fetchExecutor(executorId, {
        cache: "no-store",
      })) as KubernetesExecutorRecord;
      if (generation.current === current) setExecutor(response);
      return response;
    } catch (cause) {
      if (generation.current === current) setError(cause);
      throw cause;
    } finally {
      if (generation.current === current) setLoading(false);
    }
  }, [executorId]);

  useEffect(() => {
    if (!executorId) return;
    void reload().catch(() => undefined);
    return () => {
      generation.current += 1;
    };
  }, [executorId, reload]);

  const create = useCallback(
    async (input: KubernetesExecutorCreateInput) => {
      const created = await createKubernetesExecutorPair(input);
      const now = new Date().toISOString();
      const item: Executor = {
        id: created.executor.id,
        name: created.executor.name,
        type: "k8s",
        status: "active",
        is_system: false,
        config: created.executor.config,
        profiles: [created.profile],
        created_at: now,
        updated_at: now,
      };
      const current = store.getState().executors.items;
      store.getState().setExecutors(upsertExecutor(current, item));
      return { executor: item, profile: created.profile };
    },
    [store],
  );

  const update = useCallback(
    async (name: string, config: Record<string, string>) => {
      if (!executorId) return;
      await updateExecutor(executorId, { name, config });
      const current = store.getState().executors.items;
      store
        .getState()
        .setExecutors(
          current.map((item) => (item.id === executorId ? { ...item, name, config } : item)),
        );
      setExecutor((value) => (value ? { ...value, name, config } : value));
    },
    [executorId, store],
  );

  const remove = useCallback(async () => {
    if (!executorId) return;
    await deleteExecutor(executorId);
    const current = store.getState().executors.items;
    store.getState().setExecutors(current.filter((item) => item.id !== executorId));
  }, [executorId, store]);

  return { executor, loading, error, reload, create, update, remove };
}

export function upsertExecutor(items: Executor[], next: Executor): Executor[] {
  return items.some((item) => item.id === next.id)
    ? items.map((item) => (item.id === next.id ? next : item))
    : [...items, next];
}
