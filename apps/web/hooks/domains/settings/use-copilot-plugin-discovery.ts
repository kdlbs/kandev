"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { getCopilotPluginDiscoveryAction } from "@/app/actions/agents";
import type {
  CopilotPluginDiscoveryRequest,
  CopilotPluginDiscoveryResponse,
} from "@/lib/types/copilot-plugin-discovery";

export type CopilotPluginLaunchSettings = NonNullable<
  CopilotPluginDiscoveryRequest["launch_settings"]
>;

export type CopilotPluginDiscoveryState = {
  contextKey: string;
  status: "loading" | "ready" | "unavailable" | "unsupported";
  response?: CopilotPluginDiscoveryResponse;
  /** True when launch settings changed after the displayed inventory. */
  stale: boolean;
};

export function useCopilotPluginDiscovery({
  agentId,
  profileId,
  launchSettings,
  enabled = true,
}: {
  agentId: string;
  profileId: string;
  launchSettings?: CopilotPluginLaunchSettings;
  enabled?: boolean;
}) {
  const contextKey = discoveryContextKey(agentId, profileId, launchSettings);
  const contextKeyRef = useRef(contextKey);
  contextKeyRef.current = contextKey;
  const launchSettingsRef = useRef(launchSettings);
  launchSettingsRef.current = launchSettings;
  const requestGeneration = useRef(0);
  const requestAbort = useRef<AbortController | null>(null);
  const [state, setState] = useState<CopilotPluginDiscoveryState>({
    contextKey,
    status: "loading",
    stale: false,
  });

  const requestKey = `${agentId}:${profileId}`;
  const requestKeyRef = useRef(requestKey);
  requestKeyRef.current = requestKey;

  const runDiscovery = useCallback(
    async (forceRefresh: boolean) => {
      if (!enabled) return;
      requestAbort.current?.abort();
      const controller = new AbortController();
      requestAbort.current = controller;
      const timeout = setTimeout(() => controller.abort(), 30_000);
      const generation = ++requestGeneration.current;
      const requestedContext = contextKeyRef.current;
      const requestedKey = requestKeyRef.current;
      setState((previous) => ({
        contextKey: requestedContext,
        status: "loading",
        response: previous.contextKey === requestedContext ? previous.response : undefined,
        stale: false,
      }));
      try {
        const response = await getCopilotPluginDiscoveryAction(
          agentId,
          buildDiscoveryRequest(profileId, launchSettingsRef.current, forceRefresh),
          controller.signal,
        );
        if (requestGeneration.current !== generation) return;
        if (response.agent_id !== agentId) {
          setState({ contextKey: requestedContext, status: "unavailable", stale: false });
          return;
        }
        if (profileId && response.profile_id !== profileId) {
          setState({ contextKey: requestedContext, status: "unavailable", stale: false });
          return;
        }
        if (requestKeyRef.current !== requestedKey) return;
        setState({
          contextKey: requestedContext,
          status: response.status,
          response,
          stale: contextKeyRef.current !== requestedContext,
        });
      } catch {
        if (requestGeneration.current !== generation) return;
        if (requestKeyRef.current !== requestedKey) return;
        setState((previous) => ({
          contextKey: requestedContext,
          status: "unavailable",
          response: previous.contextKey === requestedContext ? previous.response : undefined,
          stale: contextKeyRef.current !== requestedContext,
        }));
      } finally {
        clearTimeout(timeout);
        if (requestAbort.current === controller) requestAbort.current = null;
      }
    },
    [agentId, enabled, profileId],
  );

  const refresh = useCallback(() => runDiscovery(true), [runDiscovery]);

  useEffect(() => {
    if (!enabled) return;
    void runDiscovery(false);
    return () => {
      requestGeneration.current += 1;
      requestAbort.current?.abort();
      requestAbort.current = null;
    };
  }, [enabled, runDiscovery]);

  const stale = state.contextKey !== contextKey && state.status !== "loading" ? true : state.stale;
  return { ...state, stale, refresh };
}

function discoveryContextKey(
  agentId: string,
  profileId: string,
  launchSettings?: CopilotPluginLaunchSettings,
): string {
  return launchSettings
    ? `${agentId}:${profileId}:${JSON.stringify(launchSettings)}`
    : `${agentId}:${profileId}`;
}

function buildDiscoveryRequest(
  profileId: string,
  launchSettings: CopilotPluginLaunchSettings | undefined,
  forceRefresh: boolean,
): CopilotPluginDiscoveryRequest {
  const request: CopilotPluginDiscoveryRequest = {
    launch_settings: launchSettings ?? { env_vars: [], cli_flags: [], command_prefix: "" },
  };
  if (profileId) request.profile_id = profileId;
  if (forceRefresh) request.refresh = true;
  return request;
}
