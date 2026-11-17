// Wire shape of an exclude_author_provider setup warning, returned by the
// Office routing settings endpoints and by workflow import preview/result.
export type AuthorExclusionWarningCode =
  | "routing_disabled"
  | "single_provider"
  | "agent_single_provider"
  | "check_failed";

export type AuthorExclusionWarning = {
  code: AuthorExclusionWarningCode;
  workflow_name?: string;
  step_name?: string;
  role?: string;
  provider_id?: string;
  agent_name?: string;
};
