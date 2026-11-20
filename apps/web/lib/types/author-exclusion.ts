// Wire shape of an exclude_author_provider setup warning, returned by the
// Office routing settings endpoints and by workflow import preview/result.
export type AuthorExclusionWarningCode =
  | "routing_disabled"
  | "single_provider"
  | "agent_single_provider"
  | "agent_bound_profile"
  | "check_failed";

export type AuthorExclusionWarning = {
  code: AuthorExclusionWarningCode;
  workflow_name?: string;
  step_name?: string;
  role?: string;
  provider_id?: string;
  agent_name?: string;
};

// One ensure_participant_seat action listed on the Office routing settings
// page, with whether Review on a different provider is on for it.
export type AuthorExclusionStep = {
  workflow_id: string;
  workflow_name: string;
  step_id: string;
  step_name: string;
  role: string;
  enabled: boolean;
};

export type AuthorExclusionStepChange = {
  step_id: string;
  role: string;
  enabled: boolean;
};
