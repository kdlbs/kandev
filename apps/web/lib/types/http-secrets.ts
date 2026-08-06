export type SecretScope = "global" | "workspace";

export interface SecretListItem {
  id: string;
  name: string;
  has_value: boolean;
  scope?: SecretScope;
  workspace_id?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateSecretRequest {
  name: string;
  value: string;
  scope?: SecretScope;
  workspace_id?: string;
}

export interface UpdateSecretRequest {
  name?: string;
  value?: string;
}

export interface RevealSecretResponse {
  value: string;
}
