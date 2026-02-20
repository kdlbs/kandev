export type SecretCategory = "api_key" | "service_token" | "ssh_key" | "custom";

export interface SecretListItem {
  id: string;
  name: string;
  env_key: string;
  category: SecretCategory;
  metadata?: Record<string, string>;
  has_value: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateSecretRequest {
  name: string;
  env_key: string;
  value: string;
  category?: SecretCategory;
  metadata?: Record<string, string>;
}

export interface UpdateSecretRequest {
  name?: string;
  value?: string;
  category?: SecretCategory;
  metadata?: Record<string, string>;
}

export interface RevealSecretResponse {
  value: string;
}
