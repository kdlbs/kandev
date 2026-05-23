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
  arch?: string;
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
  duration_ms: number;
  available: string[];
}
