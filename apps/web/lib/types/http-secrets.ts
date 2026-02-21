export interface SecretListItem {
  id: string;
  name: string;
  has_value: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateSecretRequest {
  name: string;
  value: string;
}

export interface UpdateSecretRequest {
  name?: string;
  value?: string;
}

export interface RevealSecretResponse {
  value: string;
}
