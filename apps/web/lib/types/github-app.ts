export type GitHubAppRegistrationSource = "managed" | "imported";
export type GitHubAppOwnerType = "User" | "Organization";
export type GitHubAppManifestOwnerType = "user" | "organization";
export type GitHubAppVisibility = "private" | "public";
export type GitHubAppRegistrationState = "active" | "invalid";
export type GitHubAppWebhookStatus = "unverified" | "verified" | "failing";

export type GitHubAppRegistration = {
  id: string;
  source: GitHubAppRegistrationSource;
  display_name: string;
  github_host: string;
  app_id: number;
  client_id: string;
  slug: string;
  owner_login: string;
  owner_type: GitHubAppOwnerType;
  visibility: GitHubAppVisibility;
  public_base_url: string;
  created_for_workspace_id?: string;
  credential_generation: number;
  status: GitHubAppRegistrationState;
  webhook_status: GitHubAppWebhookStatus;
  last_webhook_at?: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type GitHubAppRegistrationCatalogItem = GitHubAppRegistration & {
  selected: boolean;
  binding_count: number;
  workspace_binding_count: number;
  shared: boolean;
  manifest_callback_url: string;
  install_callback_url: string;
  personal_callback_url: string;
  webhook_url: string;
};

export type GitHubAppRegistrationCatalog = {
  workspace_id: string;
  registrations: GitHubAppRegistrationCatalogItem[];
};

export type GitHubAppManifest = {
  name: string;
  description: string;
  url: string;
  hook_attributes: { url: string; active: boolean };
  redirect_url: string;
  callback_urls: string[];
  setup_url: string;
  public: boolean;
  default_permissions: Record<string, "read" | "write">;
  default_events: string[];
  request_oauth_on_install: boolean;
  setup_on_update: boolean;
};

export type StartGitHubAppManifestRequest = {
  workspace_id: string;
  display_name: string;
  owner_type: GitHubAppManifestOwnerType;
  owner_login: string;
  visibility: GitHubAppVisibility;
  public_base_url: string;
};

export type StartGitHubAppManifestResponse = {
  registration_id: string;
  workspace_id: string;
  state: string;
  expires_at: string;
  revision: number;
  registration_url: string;
  manifest: GitHubAppManifest;
};

export type ImportGitHubAppRegistrationRequest = {
  registration_id: string;
  workspace_id: string;
  display_name: string;
  github_host: string;
  app_id: number;
  client_id: string;
  client_secret: string;
  private_key: string;
  webhook_secret: string;
  slug: string;
  owner_login: string;
  owner_type: GitHubAppOwnerType;
  visibility: GitHubAppVisibility;
  public_base_url: string;
};

export type PrepareGitHubAppImportRequest = {
  workspace_id: string;
  public_base_url: string;
};

export type PrepareGitHubAppImportResponse = {
  registration_id: string;
  public_base_url: string;
  manifest_callback_url: string;
  install_callback_url: string;
  personal_callback_url: string;
  webhook_url: string;
  setup_url: string;
  permissions: Record<string, "read" | "write">;
  events: string[];
  expires_at: string;
};

export type GitHubAppRegistrationErrorBody = {
  code: string;
  error: string;
  existing_registration_id?: string;
  binding_count?: number;
  problems?: string[];
};

export type GitHubCallbackResultCode =
  | "app_registered"
  | "app_connected"
  | "personal_connected"
  | "github_invalid_callback"
  | "github_app_invalid_callback"
  | "github_app_registration_cancelled";

export type GitHubCallbackResult = {
  code: GitHubCallbackResultCode | string;
  workspace_id?: string;
};
