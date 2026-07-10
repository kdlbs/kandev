"use client";

import { useCallback, useState } from "react";
import {
  listInferenceAgents,
  refreshInferenceAgent,
  type InferenceAgent,
} from "@/lib/api/domains/utility-api";

/**
 * Owns the list of inference agents fetched from the backend and the
 * refresh-one / refresh-all flows the settings page invokes from the
 * inline status note. Refresh is best-effort — failures are absorbed and
 * the next render shows the latest cached state (Manager.Refresh writes
 * the failure into the cache on its own).
 */
export function useInferenceAgents() {
  const [inferenceAgents, setInferenceAgents] = useState<InferenceAgent[]>([]);

  const refreshAgent = useCallback(
    async (agentId: string) => {
      if (!agentId) return;
      try {
        const known = inferenceAgents.some((a) => a.id === agentId);
        if (known) {
          const updated = await refreshInferenceAgent(agentId);
          setInferenceAgents((prev) => prev.map((a) => (a.id === updated.id ? updated : a)));
        } else {
          const { agents: refetched } = await listInferenceAgents({ cache: "no-store" });
          setInferenceAgents(refetched);
        }
      } catch {
        // best-effort
      }
    },
    [inferenceAgents],
  );

  return { inferenceAgents, setInferenceAgents, refreshAgent };
}
