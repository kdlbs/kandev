import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { RefObject } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  getSSHExecutorReachability,
  probeSSHExecutorReachability,
} from "@/lib/api/domains/ssh-api";
import type { SSHReachabilityRecord } from "@/lib/types/http-ssh";

type ReachabilityScope = { executorId: string; storeApi: ReturnType<typeof useAppStoreApi> };
type ReachabilityLifetime = { scope: ReachabilityScope; generation: number; pendingProbes: number };

/**
 * Owns local request controls for the committed SSH executor visit. Accepted
 * records still reconcile with WebSocket evidence in the store.
 */
export function useSSHReachability(executorId: string) {
  const record = useAppStore((state) => state.sshReachability.byExecutorId[executorId]);
  const storeApi = useAppStoreApi();
  const scope = useMemo(() => ({ executorId, storeApi }), [executorId, storeApi]);
  const lifetimeRef = useRef<ReachabilityLifetime | null>(null);
  const generationRef = useRef(-1);
  const [generation, setGeneration] = useState(0);
  const [loadError, setLoadError] = useState(false);
  const [probing, setProbing] = useState(false);
  const seqRef = useRef(0);

  useLayoutEffect(() => {
    const lifetime = { scope, generation: ++generationRef.current, pendingProbes: 0 };
    lifetimeRef.current = lifetime;
    setGeneration(lifetime.generation);
    setLoadError(false);
    setProbing(false);
    return () => {
      lifetimeRef.current = null;
    };
  }, [scope]);

  const load = useCallback(async () => {
    const lifetime = lifetimeRef.current;
    if (!lifetime || lifetime.scope !== scope || lifetime.generation !== generation) return;
    const seq = ++seqRef.current;
    const isCurrent = () => lifetimeRef.current === lifetime && seq === seqRef.current;
    try {
      const response = await getSSHExecutorReachability(executorId);
      if (!isCurrent()) return;
      setLoadError(false);
      storeApi.getState().setSSHReachability(response);
    } catch {
      if (isCurrent()) setLoadError(true);
    }
  }, [executorId, generation, scope, storeApi]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!record || !record.probing_enabled || record.probe_interval_seconds <= 0) return;
    const interval = window.setInterval(() => void load(), record.probe_interval_seconds * 1000);
    return () => window.clearInterval(interval);
  }, [record, load]);

  const probeNow = useCallback(async () => {
    const lifetime = lifetimeRef.current;
    if (!lifetime || lifetime.scope !== scope || lifetime.generation !== generation) return;
    const isCurrent = () => lifetimeRef.current === lifetime;
    lifetime.pendingProbes++;
    setProbing(true);
    try {
      const response = await probeSSHExecutorReachability(executorId);
      if (!isCurrent()) return;
      setLoadError(false);
      storeApi.getState().setSSHReachability(response);
    } catch {
      if (isCurrent()) setLoadError(true);
    } finally {
      if (isCurrent()) {
        lifetime.pendingProbes--;
        setProbing(lifetime.pendingProbes > 0);
      }
    }
  }, [executorId, generation, scope, storeApi]);

  const now = useReachabilityClock(record, scope, lifetimeRef);
  return { record, loadError, probing, probeNow, now };
}

function useReachabilityClock(
  record: SSHReachabilityRecord | undefined,
  scope: ReachabilityScope,
  lifetimeRef: RefObject<ReachabilityLifetime | null>,
) {
  const [now, setNow] = useState(() => Date.now());
  useLayoutEffect(() => setNow(Date.now()), [scope]);

  // Keep the stale badge live even when a failed refresh leaves the record unchanged.
  useEffect(() => {
    if (!record?.probing_enabled || !record.checked_at || record.probe_interval_seconds <= 0) {
      return;
    }
    const lifetime = lifetimeRef.current;
    const checkedAt = Date.parse(record.checked_at);
    if (Number.isNaN(checkedAt)) return;
    const staleAt = checkedAt + record.probe_interval_seconds * 1000 * 3;
    const delay = Math.max(0, staleAt - Date.now() + 1);
    const timer = window.setTimeout(() => {
      if (lifetime && lifetimeRef.current === lifetime) setNow(Date.now());
    }, delay);
    return () => window.clearTimeout(timer);
  }, [record, scope, lifetimeRef]);
  return now;
}
