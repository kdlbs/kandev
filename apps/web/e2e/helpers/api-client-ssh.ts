// SSH-specific HTTP shapes for the e2e ApiClient. Kept in a dedicated module
// (apart from helpers/api-client.ts) so adding SSH fields doesn't keep growing
// the omnibus client file. ApiClient methods that exercise these shapes live
// on the ApiClient class itself, alongside the rest of the surface.

export type SSHIdentitySourceBody = "agent" | "file";

export type SSHTestRequestBody = {
  name: string;
  host_alias?: string;
  host?: string;
  port?: number;
  user?: string;
  identity_source?: SSHIdentitySourceBody;
  identity_file?: string;
  proxy_jump?: string;
};

export type SSHTestStepBody = {
  name: string;
  duration_ms: number;
  success: boolean;
  output?: string;
  error?: string;
};

export type SSHTestResultBody = {
  success: boolean;
  fingerprint?: string;
  uname_all?: string;
  arch?: string;
  git_version?: string;
  agentctl_action?: "cached" | "needs_upload" | "skipped";
  steps: SSHTestStepBody[];
  total_duration_ms: number;
  error?: string;
};

export type SSHSessionBody = {
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
};

export type SSHAgentReadinessRowBody = {
  agent_id: string;
  agent_name: string;
  binary: string;
  available: boolean;
  resolved_at?: string;
  install_hint?: string;
  error?: string;
};

export type SSHAgentReadinessResponseBody = {
  host: string;
  shell?: string;
  duration_ms: number;
  rows: SSHAgentReadinessRowBody[];
};

export type SSHProbeShellsResponseBody = {
  host: string;
  duration_ms: number;
  available: string[];
};
