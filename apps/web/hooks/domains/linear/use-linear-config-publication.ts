import { useCallback, useEffect, useRef } from "react";
import { getLinearConfig } from "@/lib/api/domains/linear-api";
import type { LinearConfig } from "@/lib/types/linear";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";

// Reads share the mounted workspace's sequence; write ACKs fence pending reads.
export function useLinearConfigPublication(workspaceId: string) {
  const currentOwner = useRef<{ sequence: number } | null>(null);
  useEffect(() => {
    currentOwner.current = { sequence: 0 };
    return () => {
      currentOwner.current = null;
    };
  }, [workspaceId]);

  const beginRead = useCallback(() => {
    const owner = currentOwner.current;
    const sequence = owner ? ++owner.sequence : 0;
    const isActive = () => owner !== null && owner === currentOwner.current;
    return {
      isCurrent: () => isActive() && owner?.sequence === sequence,
    };
  }, []);

  const beginWrite = useCallback(() => {
    const owner = currentOwner.current;
    return () => {
      if (!owner || owner !== currentOwner.current) return false;
      ++owner.sequence;
      return true;
    };
  }, []);

  return { beginRead, beginWrite };
}

export function useLinearConfigRefresh(
  workspaceId: string,
  setConfig: (cfg: LinearConfig | null) => void,
  beginRead: ReturnType<typeof useLinearConfigPublication>["beginRead"],
  loading: boolean,
) {
  // Initial form seeding settles before background health reads begin.
  useEffect(() => {
    if (loading) return;
    const id = setInterval(() => {
      const request = beginRead();
      getLinearConfig({ workspaceId })
        .then((cfg) => {
          if (request.isCurrent()) setConfig(cfg);
        })
        .catch(() => {
          /* transient failures retain the accepted config */
        });
    }, INTEGRATION_STATUS_REFRESH_MS);
    return () => clearInterval(id);
  }, [workspaceId, setConfig, beginRead, loading]);
}
