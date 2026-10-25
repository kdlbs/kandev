"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getAgentMcpDiscoveryAction } from "@/app/actions/agents";
import type { AgentMcpDiscoveryResponse } from "@/lib/types/agent-mcp-discovery";

export type AgentMcpDiscoveryState = {
  profileKey: string;
  status: "loading" | "ready" | "unavailable" | "unsupported";
  response?: AgentMcpDiscoveryResponse;
};

export function useAgentMcpDiscovery(agentId: string, profileId: string) {
  const profileKey = `${agentId}:${profileId}`;
  const requestGeneration = useRef(0);
  const [state, setState] = useState<AgentMcpDiscoveryState>({
    profileKey,
    status: "loading",
  });

  const refresh = useCallback(async () => {
    const generation = ++requestGeneration.current;
    setState((previous) => ({
      profileKey,
      status: "loading",
      response: previous.profileKey === profileKey ? previous.response : undefined,
    }));
    try {
      const response = await getAgentMcpDiscoveryAction(agentId);
      if (requestGeneration.current !== generation) return;
      if (response.agent_id !== agentId) {
        setState({ profileKey, status: "unavailable" });
        return;
      }
      setState({
        profileKey,
        status: response.status,
        response,
      });
    } catch {
      if (requestGeneration.current !== generation) return;
      setState((previous) => ({
        profileKey,
        status: "unavailable",
        response: previous.profileKey === profileKey ? previous.response : undefined,
      }));
    }
  }, [agentId, profileKey]);

  useEffect(() => {
    void refresh();
    return () => {
      requestGeneration.current += 1;
    };
  }, [refresh]);

  const currentState =
    state.profileKey === profileKey ? state : { profileKey, status: "loading" as const };
  return { ...currentState, refresh };
}
