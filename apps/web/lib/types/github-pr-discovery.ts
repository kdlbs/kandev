export type GitHubPRDiscoveryHealthState = "unknown" | "healthy" | "degraded";
export type GitHubPRDiscoveryHealthCategory = "invalid_query" | "rate_limited" | "unavailable";

export type GitHubPRDiscoveryHealth = {
  state: GitHubPRDiscoveryHealthState;
  failed_target_count: number;
  last_failure_at?: string;
  category?: GitHubPRDiscoveryHealthCategory;
  retry_at?: string;
  revision: number;
  credential_generation: number;
  runtime_epoch?: number;
};

export type GitHubPRDiscoveryHealthUpdate = {
  workspace_id: string;
  health: GitHubPRDiscoveryHealth;
};
