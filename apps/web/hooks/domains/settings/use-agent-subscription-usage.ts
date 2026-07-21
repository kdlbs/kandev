import { useCallback, useEffect, useRef, useState } from "react";
import { listAgentSubscriptionUsage } from "@/lib/api";
import type { AgentSubscriptionUsage } from "@/lib/types/http";

/**
 * Fetches subscription utilization for host-installed agents (Claude Code,
 * Codex). The initial load accepts the backend's 5-minute cache; manual
 * refresh() requests fresh provider data (server-clamped to 15 s).
 */
export function useAgentSubscriptionUsage() {
  const [items, setItems] = useState<AgentSubscriptionUsage[]>([]);
  const [loading, setLoading] = useState(true);
  const fetchingRef = useRef(false);

  const fetchUsage = useCallback(async (fresh: boolean) => {
    if (fetchingRef.current) return;
    fetchingRef.current = true;
    setLoading(true);
    try {
      const response = await listAgentSubscriptionUsage({ cache: "no-store", fresh });
      setItems(response.agents ?? []);
    } catch {
      // Keep the previous items: clearing them would hide the whole section
      // (including the Refresh button) on a transient refresh failure.
    } finally {
      fetchingRef.current = false;
      setLoading(false);
    }
  }, []);

  const refresh = useCallback(() => fetchUsage(true), [fetchUsage]);

  useEffect(() => {
    void fetchUsage(false);
  }, [fetchUsage]);

  return { items, loading, refresh };
}
