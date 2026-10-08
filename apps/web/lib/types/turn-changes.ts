export type TurnChangeAvailability =
  | "pending"
  | "ready"
  | "unavailable"
  | "failed"
  | "expired"
  | "unsupported";

export type TurnChangeOverlap = {
  change_set_id: string;
  checkout_id: string;
  started_at: string;
  ended_at?: string;
};

export type TurnRepositoryChange = {
  id: string;
  checkout_id: string;
  repository_id?: string;
  worktree_id?: string;
  display_name?: string;
  repository_subpath?: string;
  availability: TurnChangeAvailability;
  reason?: string;
  enumeration_complete: boolean;
  comparison_complete: boolean;
  content_complete: boolean;
  overlap_intervals?: TurnChangeOverlap[];
};

export type TurnChangeSetSummary = {
  id: string;
  task_id: string;
  session_id: string;
  turn_id: string;
  revision: number;
  availability: TurnChangeAvailability;
  reason?: string;
  complete: boolean;
  summary_complete: boolean;
  content_complete: boolean;
  turn_ordinal: number;
  terminal_at?: string;
  terminal_outcome?: string;
  final_assistant_message_id?: string;
  fallback_anchor: string;
  file_count: number;
  added_lines?: number;
  deleted_lines?: number;
  binary_file_count: number;
  unknown_count_file_count: number;
  repository_count: number;
  retain_until?: string;
  expiry_reason?: string;
  overlap_intervals?: TurnChangeOverlap[];
  repositories: TurnRepositoryChange[];
};

export type TurnFileChange = {
  id: string;
  repository_change_id: string;
  checkout_id: string;
  path: string;
  old_path?: string;
  kind: string;
  old_mode?: string;
  new_mode?: string;
  submodule?: boolean;
  binary?: boolean;
  added_lines?: number;
  deleted_lines?: number;
  content_availability: TurnChangeAvailability;
  content_reason?: string;
  content_truncated?: boolean;
};

export type TurnChangeHistoryPage = {
  change_sets: TurnChangeSetSummary[];
  total: number;
  offset: number;
  limit: number;
  next_offset?: number;
};

export type TurnChangeFilesPage = {
  files: TurnFileChange[];
  total: number;
  offset: number;
  limit: number;
  next_offset?: number;
};

export type TurnChangeContentVariant =
  | "canonical_patch"
  | "filtered_patch"
  | "old_rendering"
  | "new_rendering";

export type TurnChangeContent = {
  file_change_id: string;
  variant: TurnChangeContentVariant;
  /** Go serializes the retained []byte payload as base64 in JSON. */
  content: string;
  digest: string;
  truncated?: boolean;
};
