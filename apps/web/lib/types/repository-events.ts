/** A `repository.deleted` event identifies the repository. */
export type RepositoryDeletedPayload = { id: string; workspace_id?: string };

/**
 * A `repository_set.*` event. `repositories` is absent on the delete event, whose
 * payload only has to identify the set and its workspace.
 */
export type RepositorySetPayload = {
  id: string;
  workspace_id: string;
  name?: string;
  description?: string;
  repositories?: Array<{ repository_id: string; position: number; base_branch?: string }>;
  created_at?: string;
  updated_at?: string;
};

export type RepositoryBranchPolicyPayload = {
  id: string;
  repository_id: string;
  name?: string;
  description?: string;
  base_branch?: string;
  branch_template?: string;
  pull_request_target?: string;
  created_at?: string;
  updated_at?: string;
};
