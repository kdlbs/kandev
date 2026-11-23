import type {
  WorkflowSyncConfig,
  WorkflowSyncResult,
  WorkflowSyncSetConfigRequest,
} from "@/lib/types/workflow-sync";

// i18n-exempt: stable error code returned by the GitHub API layer.
export const PROMPT_SYNC_RATE_LIMIT_ERROR_CODE = "github_rate_limited";

/**
 * The install-wide Prompt Sync configuration. It has the same repository and
 * status fields as Workflow Sync; `workspace_id` names the credential
 * workspace whose GitHub or GitLab connection reads the repository.
 */
export type PromptSyncConfig = WorkflowSyncConfig & {
  last_error_class?: string;
  poll_suspended?: boolean;
  poll_suspension_reason?: string;
};

/** The user's side in a name conflict, by prompt name. */
export type PromptConflictChoice = "local" | "repository";
export type PromptConflictResolutions = Record<string, PromptConflictChoice>;

export type PromptSyncSetConfigRequest = WorkflowSyncSetConfigRequest & {
  workspace_id: string;
  resolutions?: PromptConflictResolutions;
};

export type PromptSyncResult = WorkflowSyncResult & {
  overridden?: string[];
  /** Prompts still waiting for the user to choose a version. */
  conflicts?: string[];
  restored?: string[];
};

/** A repository file whose name an existing prompt already uses. */
export interface PromptSyncConflict {
  name: string;
  path: string;
  local_content: string;
  repository_content: string;
  /** The version the prompt uses now, for prompts that keep both. */
  in_use?: PromptConflictChoice;
}

export interface PromptSyncCheckResponse {
  conflicts: PromptSyncConflict[];
  warnings?: string[];
}

/** A completed sync returns 200 even when it carries a sanitized `error`. */
export interface PromptSyncRunResponse {
  config: PromptSyncConfig;
  result?: PromptSyncResult;
  error?: string;
  error_code?: string;
}

/** What removing Prompt Sync asks about. */
export interface PromptSyncRemovalReview {
  /** Prompts with both a local and a repository version. */
  prompts: PromptSyncConflict[];
  /** Names of synced prompts that only came from the repository. */
  repository_only: string[];
}

/** The user's choices when Prompt Sync is removed. */
export interface PromptSyncRemoval {
  choices: PromptConflictResolutions;
  /** The reviewed prompts that only came from the repository to delete. */
  delete_repository_only: string[];
}

/** A saved Local override switch with an optional edited local version. */
export interface PromptVersionsSave {
  use_local: boolean;
  local_content?: string;
  expected_updated_at?: string;
}
