"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { applyUpdate, checkUpdates, fetchUpdates } from "@/lib/api/domains/system-api";

export function useUpdates() {
  const updates = useAppStore((s) => s.system.updates);
  const setSystemUpdates = useAppStore((s) => s.setSystemUpdates);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);
  const [isApplying, setIsApplying] = useState(false);

  const reload = useCallback(async () => {
    setIsLoading(true);
    setError(null);
    try {
      const res = await fetchUpdates({ cache: "no-store" });
      setSystemUpdates(res);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setIsLoading(false);
    }
  }, [setSystemUpdates]);

  /**
   * Triggers a server-side re-poll of the GitHub releases endpoint. The
   * backend rate-limits this per-process to one call per 30s and replies
   * with the fresh row (or 429 — surfaced via the returned promise).
   */
  const check = useCallback(async () => {
    setIsChecking(true);
    setError(null);
    try {
      const res = await checkUpdates();
      setSystemUpdates(res);
      return res;
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      setIsChecking(false);
    }
  }, [setSystemUpdates]);

  const apply = useCallback(async () => {
    setIsApplying(true);
    setError(null);
    try {
      return await applyUpdate("UPDATE");
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      throw e;
    } finally {
      setIsApplying(false);
    }
  }, []);

  useEffect(() => {
    if (updates) return;
    void reload();
  }, [updates, reload]);

  return { updates, isLoading, isChecking, isApplying, error, reload, check, apply };
}
