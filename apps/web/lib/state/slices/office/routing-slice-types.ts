import type { ProviderHealth, RouteAttempt, AgentRouteData } from "./types";

export type ProviderHealthSliceState = {
  byWorkspace: Record<string, ProviderHealth[]>;
};

export type RunAttemptsState = {
  byRunId: Record<string, RouteAttempt[]>;
};

export type AgentRoutingSliceState = {
  byAgentId: Record<string, AgentRouteData | undefined>;
};
