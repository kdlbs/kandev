/**
 * Connection status for the GitLab integration. Returned by
 * `GET /api/v1/gitlab/status` and shaped by `internal/gitlab.Status`.
 */
export type GitLabStatus = {
  authenticated: boolean;
  username: string;
  auth_method: "glab_cli" | "pat" | "none" | "mock";
  host: string;
  token_configured: boolean;
  token_secret_id?: string;
  glab_version?: string;
  glab_outdated?: boolean;
  required_scopes: string[];
  diagnostics?: GitLabAuthDiagnostics;
};

export type GitLabAuthDiagnostics = {
  command: string;
  output: string;
  exit_code: number;
};

export type GitLabConfigureTokenResponse = { configured: boolean };
export type GitLabClearTokenResponse = { cleared: boolean };
export type GitLabConfigureHostResponse = { configured: boolean; host: string };

export type GitLabMRNote = {
  id: number;
  author: string;
  author_avatar?: string;
  author_is_bot?: boolean;
  body: string;
  type?: string;
  system?: boolean;
  created_at: string;
  updated_at: string;
};

export type GitLabMRDiscussion = {
  id: string;
  resolvable: boolean;
  resolved: boolean;
  notes: GitLabMRNote[];
  path?: string;
  line?: number;
  old_line?: number;
  created_at: string;
  updated_at: string;
};
