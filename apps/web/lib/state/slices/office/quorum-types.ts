// QuorumGuardEntry mirrors the backend's GuardStateDTO
// (internal/office/dashboard/quorum_dto.go) — one guarded on_turn_complete
// transition's live evaluation state at the task's current step (AC-24b).
export type QuorumGuardEntry = {
  target_step_id: string;
  role: string;
  threshold: string;
  required_count: number;
  received_count: number;
  satisfied: boolean;
  reason?: string;
  error?: string;
};

export type QuorumResponseDTO = {
  guards: QuorumGuardEntry[];
  reevaluation_blocked: boolean;
};

export type TaskQuorumSliceState = {
  byTaskId: Record<string, QuorumResponseDTO | undefined>;
};
