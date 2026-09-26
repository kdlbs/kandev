export type SSHIdentitySource = "agent" | "file";

export interface SSHTestRequest {
  name: string;
  host_alias?: string;
  host?: string;
  port?: number;
  user?: string;
  identity_source?: SSHIdentitySource;
  identity_file?: string;
  proxy_jump?: string;
}

export interface SSHTestStep {
  name: string;
  duration_ms: number;
  success: boolean;
  output?: string;
  error?: string;
}

export interface SSHTestResult {
  success: boolean;
  fingerprint?: string;
  uname_all?: string;
  os?: string;
  arch?: string;
  platform?: string;
  git_version?: string;
  agentctl_action?: "cached" | "needs_upload" | "skipped";
  steps: SSHTestStep[];
  total_duration_ms: number;
  error?: string;
}

export interface SSHSession {
  session_id: string;
  task_id: string;
  task_title?: string;
  host: string;
  user?: string;
  remote_task_dir?: string;
  remote_agentctl_port?: number;
  local_forward_port?: number;
  status: string;
  uptime_seconds: number;
  created_at: string;
}

export interface SSHAgentReadinessRow {
  agent_id: string;
  agent_name: string;
  binary: string;
  available: boolean;
  resolved_at?: string;
  install_hint?: string;
  error?: string;
}

export interface SSHAgentReadinessResponse {
  host: string;
  shell?: string;
  duration_ms: number;
  rows: SSHAgentReadinessRow[];
}

export interface SSHProbeShellsResponse {
  host: string;
  default_shell: string;
  duration_ms: number;
  available: string[];
}

export type SSHReachabilityState = "unknown" | "reachable" | "unreachable";

export type SSHReachabilityReason =
  | ""
  | "config"
  | "timeout"
  | "host_key"
  | "auth"
  | "network"
  | "unknown";

/** Mirrors the backend's reachability.RecordDTO wire shape. */
export interface SSHReachabilityRecord {
  executor_id: string;
  state: SSHReachabilityState;
  reason: SSHReachabilityReason;
  message?: string;
  consecutive_failures: number;
  host?: string;
  checked_at: string | null;
  last_success_at: string | null;
  /** Null on the synthesized "never probed" placeholder. */
  updated_at: string | null;
  probing_enabled: boolean;
  /** The effective, clamped probe interval in seconds (0 when disabled). */
  probe_interval_seconds: number;
  persisted: boolean;
}
