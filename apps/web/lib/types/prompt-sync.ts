import type {
  WorkflowSyncConfig,
  WorkflowSyncResult,
  WorkflowSyncSetConfigRequest,
} from "@/lib/types/workflow-sync";

/**
 * The install-wide Prompt Sync configuration. It has the same repository and
 * status fields as Workflow Sync; `workspace_id` names the credential
 * workspace whose GitHub or GitLab connection reads the repository.
 */
export type PromptSyncConfig = WorkflowSyncConfig & {
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
}
