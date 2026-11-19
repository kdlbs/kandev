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

export type PromptSyncSetConfigRequest = WorkflowSyncSetConfigRequest & {
  workspace_id: string;
};

export type PromptSyncResult = WorkflowSyncResult;

/** A completed sync returns 200 even when it carries a sanitized `error`. */
export interface PromptSyncRunResponse {
  config: PromptSyncConfig;
  result?: PromptSyncResult;
  error?: string;
}
