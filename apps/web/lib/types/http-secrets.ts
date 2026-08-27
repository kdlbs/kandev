/** Scope of a secret: either global or bound to a workspace. */
export type SecretScope = "global" | "workspace";

/** A secret as returned by the secrets API. */
export interface SecretListItem {
  id: string;
  name: string;
  has_value: boolean;
  scope?: SecretScope;
  workspace_id?: string;
  created_at: string;
  updated_at: string;
}

/** Request payload for creating a secret. */
export interface CreateSecretRequest {
  name: string;
  value: string;
  scope?: SecretScope;
  workspace_id?: string;
}

/** Request payload for updating a secret. */
export interface UpdateSecretRequest {
  name?: string;
  value?: string;
}

/**
 * Request body for copy/move operations. `name` is optional with presence
 * semantics matching the backend: omitted means "use the source secret's
 * name". It must never be emitted as `null` (the backend rejects it).
 */
export interface CopyMoveSecretRequest {
  target_scope: SecretScope;
  target_workspace_id?: string;
  name?: string;
}

/** Response payload for revealing a secret's value. */
export interface RevealSecretResponse {
  value: string;
}
