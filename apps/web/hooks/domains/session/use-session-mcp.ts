"use client";

import { useEffect, useMemo, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { readJourneyMcpConfig } from "@/hooks/journey-metadata-resources";

const EMPTY_SERVERS: string[] = [];
const DEFAULT_KANDEV: string[] = ["kandev"];

/**
 * Resolves MCP support and configured MCP server names for the current session's agent.
 * Returns whether the agent supports MCP and the list of active MCP server names.
 */
export function useSessionMcp(
  agentProfileId: string | null | undefined,
  sessionId?: string,
  detailActive = true,
) {
  const store = useAppStoreApi();
  const settingsAgents = useAppStore((state) => state.settingsAgents.items);
  const attachmentHistory = useAppStore((state) =>
    sessionId ? state.sessionMcpStatus.bySessionId[sessionId] : undefined,
  );
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
    if (!detailActive || !agentProfileId || !supportsMcp) return;
    let active = true;
    const controller = new AbortController();
    const currentProfileId = agentProfileId;
    readJourneyMcpConfig(store, currentProfileId, { signal: controller.signal })
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
      controller.abort();
    };
  }, [agentProfileId, detailActive, store, supportsMcp]);

  const mcpServers = useMemo(() => {
    const observedServers = attachmentHistory?.current.servers;
    if (observedServers?.length) return observedServers.map((server) => server.name);
    if (!supportsMcp) return EMPTY_SERVERS;
    if (fetchResult && fetchResult.profileId === agentProfileId) return fetchResult.servers;
    return DEFAULT_KANDEV;
  }, [attachmentHistory, supportsMcp, fetchResult, agentProfileId]);

  return { supportsMcp, mcpServers, attachmentHistory };
}
