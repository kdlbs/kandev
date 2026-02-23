"use client";

import { useEffect, useMemo, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { getAgentProfileMcpConfigAction } from "@/app/actions/agents";

const EMPTY_SERVERS: string[] = [];
const DEFAULT_KANDEV: string[] = ["kandev"];

/**
 * Resolves MCP support and configured MCP server names for the current session's agent.
 * Returns whether the agent supports MCP and the list of active MCP server names.
 */
export function useSessionMcp(agentProfileId: string | null | undefined) {
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  // Track which profileId the fetched servers belong to, so stale results are ignored
  const [fetchResult, setFetchResult] = useState<{
    profileId: string;
    servers: string[];
  } | null>(null);

  const agent = useMemo(() => {
    if (!agentProfileId) return null;
    for (const a of settingsAgents) {
      if (a.profiles.some((p) => p.id === agentProfileId)) return a;
    }
    return null;
  }, [agentProfileId, settingsAgents]);

  const supportsMcp = agent?.supports_mcp ?? false;

  useEffect(() => {
    if (!agentProfileId || !supportsMcp) return;
    let active = true;
    const currentProfileId = agentProfileId;
    getAgentProfileMcpConfigAction(currentProfileId)
      .then((config) => {
        if (!active) return;
        const userServers = config.enabled ? Object.keys(config.servers) : [];
        setFetchResult({ profileId: currentProfileId, servers: ["kandev", ...userServers] });
      })
      .catch(() => {
        if (!active) return;
        setFetchResult({ profileId: currentProfileId, servers: DEFAULT_KANDEV });
      });
    return () => {
      active = false;
    };
  }, [agentProfileId, supportsMcp]);

  const mcpServers = useMemo(() => {
    if (!supportsMcp) return EMPTY_SERVERS;
    if (fetchResult && fetchResult.profileId === agentProfileId) return fetchResult.servers;
    return DEFAULT_KANDEV;
  }, [supportsMcp, fetchResult, agentProfileId]);

  return { supportsMcp, mcpServers };
}
