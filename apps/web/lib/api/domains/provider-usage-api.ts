import { fetchJson, type ApiRequestOptions } from "../client";

export type ObservedUsageWindow = {
  label: string;
  model?: string;
  utilization_pct: number;
  reset_at: string;
  utilization_observed: boolean;
  reset_at_observed: boolean;
};
export type ObservedUsageSample = {
  provider: string;
  account_ref: string;
  organization_ref?: string;
  plan?: string;
  fetched_at: string;
  windows: ObservedUsageWindow[];
};
export type ExecutionUsageReport = {
  source_known: boolean;
  reason: string;
  model?: string;
  current?: ObservedUsageSample;
  previous?: ObservedUsageSample;
  limited: boolean;
  blocking_window?: string;
  reset_at?: string;
  binding_changed?: boolean;
};
export type ProfileExecutionUsage = {
  execution_id: string;
  session_id: string;
  report: ExecutionUsageReport;
};
export type CachedProfileUsage = {
  profile_id: string;
  cached_only: true;
  executions: ProfileExecutionUsage[];
};

/** A distinct suffix stays unavailable on older provider-refreshing servers. */
export const readCachedProfileExecutionUsage = (profileId: string, options?: ApiRequestOptions) =>
  fetchJson<CachedProfileUsage>(
    `/api/v1/agent-profiles/${encodeURIComponent(profileId)}/usage/cached`,
    options,
  );

export const readProfileExecutionUsage = (profileId: string, options?: ApiRequestOptions) =>
  fetchJson<{ profile_id: string; executions: ProfileExecutionUsage[] }>(
    `/api/v1/agent-profiles/${encodeURIComponent(profileId)}/usage`,
    options,
  );
