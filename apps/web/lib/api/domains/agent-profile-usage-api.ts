import { fetchJson } from "@/lib/api/client";

export type AgentProfileUsageWindow = {
  label: string;
  utilization_pct: number;
  reset_at?: string;
  scope?: string;
  limit_reached?: boolean;
};

export type AgentProfileUsage = {
  profile_id: string;
  model_id: string;
  state: "ok" | "unsupported" | "unavailable" | "no_usage_api";
  stale?: boolean;
  reason?: string;
  status?: number;
  source: "provider_api" | "proxy" | "none";
  binding_scope?: "host_default" | "local_proxy";
  provider?: string;
  account_id?: string;
  model_class?: "free" | "paid" | "subscription";
  plan?: string;
  fetched_at?: string;
  windows: AgentProfileUsageWindow[];
};

export type AgentProfileUsageResponse = { profiles: AgentProfileUsage[] };

export function listAgentProfileUsage(): Promise<AgentProfileUsageResponse> {
  return fetchJson<AgentProfileUsageResponse>("/api/v1/agent-profiles/usage", {
    cache: "no-store",
  });
}
