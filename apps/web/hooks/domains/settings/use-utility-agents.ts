"use client";

import { useEffect, useState } from "react";
import { listUtilityAgents, type UtilityAgent } from "@/lib/api/domains/utility-api";

type UtilityAgentsState = {
  agents: UtilityAgent[];
  loading: boolean;
  error: Error | null;
};

export function useUtilityAgents(): UtilityAgentsState {
  const [state, setState] = useState<UtilityAgentsState>({
    agents: [],
    loading: true,
    error: null,
  });

  useEffect(() => {
    let active = true;
    listUtilityAgents({ cache: "no-store" })
      .then((response) => {
        if (active) setState({ agents: response.agents, loading: false, error: null });
      })
      .catch((error: unknown) => {
        if (!active) return;
        setState({
          agents: [],
          loading: false,
          error: error instanceof Error ? error : new Error("Failed to load utility agents"),
        });
      });
    return () => {
      active = false;
    };
  }, []);

  return state;
}
