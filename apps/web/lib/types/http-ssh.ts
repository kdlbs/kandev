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
  /**
   * Stable identifier for the remediation this failure needs, not copy. The
   * backend picks it from the typed cause; the card maps it to translated
   * text and ignores an identifier it does not know.
   */
  hint?: string;
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

/** One private key file the backend host could use as a `file` identity. */
export interface SSHIdentity {
  path: string;
  display_path: string;
  key_type?: string;
  encrypted: boolean;
  source: "ssh_dir" | "ssh_config";
}

export interface SSHIdentitiesResponse {
  home_dir: string;
  identities: SSHIdentity[];
}
