// Mirrors the stored spec of an answer proposal (internal/coordinator/kind_answer.go).
export type AnswerDraftEntry = { question_id: string; option_ids?: string[]; custom_text?: string };
export type AnswerDraft = { answers?: AnswerDraftEntry[]; option_id?: string };
export type AnswerSource = { ref: string; label?: string };
export type AnswerSnapshotQuestion = {
  id: string;
  text: string;
  allow_custom_text?: boolean;
  options: { id: string; label: string }[];
};
export type AnswerSnapshotPermission = {
  title: string;
  summary: string;
  options: { id: string; name: string; kind: string }[];
};
export type AnswerSpec = {
  task_id: string;
  target: "clarification" | "permission";
  pending_id: string;
  request_id?: string;
  draft: AnswerDraft;
  snapshot: { questions?: AnswerSnapshotQuestion[]; permission?: AnswerSnapshotPermission };
  sources: AnswerSource[];
  confidence: string;
  needs_decision: boolean;
  rationale: string;
};
