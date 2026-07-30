export type TaskNoteEventPayload = {
  id: string;
  task_id: string;
  content: string;
  updated_by: "agent" | "user";
  created_at: string;
  updated_at: string;
};

export type TaskNoteDeletedEventPayload = {
  task_id: string;
};
