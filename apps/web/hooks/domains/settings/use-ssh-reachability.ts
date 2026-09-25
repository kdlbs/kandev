import { useCallback, useEffect, useRef, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getSSHExecutorReachability,
  probeSSHExecutorReachability,
} from "@/lib/api/domains/ssh-api";

/**
 * Owns the settings-domain fetch, refresh, and immediate-probe lifecycle for
 * one SSH executor. The store remains the source of truth for the record, so
 * WebSocket updates and HTTP responses use the same reconciliation path.
 */
export function useSSHReachability(executorId: string) {
  const record = useAppStore((state) => state.sshReachability.byExecutorId[executorId]);
  const storeApi = useAppStoreApi();
  const [loadError, setLoadError] = useState(false);
  const [probing, setProbing] = useState(false);
  const [now, setNow] = useState(() => Date.now());
  const seqRef = useRef(0);

  const load = useCallback(async () => {
    const seq = ++seqRef.current;
    try {
      const response = await getSSHExecutorReachability(executorId);
      if (seq !== seqRef.current) return;
      setLoadError(false);
      storeApi.getState().setSSHReachability(response);
    } catch {
      if (seq !== seqRef.current) return;
      setLoadError(true);
    }
  }, [executorId, storeApi]);

  useEffect(() => {
    seqRef.current = 0;
    setLoadError(false);
    setNow(Date.now());
    void load();
    return () => {
      seqRef.current = -1;
    };
    // The executor identity is the lifecycle boundary. `load` is stable for
    // that identity and must not restart the request on every store update.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [executorId]);

  useEffect(() => {
    if (!record || !record.probing_enabled || record.probe_interval_seconds <= 0) return;
    const interval = window.setInterval(() => void load(), record.probe_interval_seconds * 1000);
    return () => window.clearInterval(interval);
  }, [record, load]);

  // A failed refresh can leave the record object unchanged. Keep the stale
  // badge live in that case instead of waiting for a successful fetch to
  // trigger another render.
  useEffect(() => {
    if (!record?.probing_enabled || !record.checked_at || record.probe_interval_seconds <= 0) {
      return;
    }
    const checkedAt = Date.parse(record.checked_at);
    if (Number.isNaN(checkedAt)) return;
    const staleAt = checkedAt + record.probe_interval_seconds * 1000 * 3;
    const delay = Math.max(0, staleAt - Date.now() + 1);
    const timer = window.setTimeout(() => setNow(Date.now()), delay);
    return () => window.clearTimeout(timer);
  }, [record]);

  const probeNow = useCallback(async () => {
    setProbing(true);
    try {
      const response = await probeSSHExecutorReachability(executorId);
      setLoadError(false);
      storeApi.getState().setSSHReachability(response);
    } catch {
      setLoadError(true);
    } finally {
      setProbing(false);
    }
  }, [executorId, storeApi]);

  return { record, loadError, probing, probeNow, now };
}
