export type TaskPlanEventPayload = {
  id: string;
  task_id: string;
  title: string;
  content: string;
  created_by: "agent" | "user";
  created_at: string;
  updated_at: string;
  implementation_started_at?: string | null;
  implementation_started_session_id?: string | null;
  implementation_started_by?: string | null;
};

export type TaskPlanRevisionEventPayload = {
  id: string;
  task_id: string;
  revision_number: number;
  title: string;
  author_kind: "agent" | "user";
  author_name: string;
  content_length?: number;
  revert_of_revision_id?: string | null;
  workflow_step_id?: string;
  workflow_step_name?: string;
  workflow_step_color?: string;
  coalesced?: boolean;
  created_at: string;
  updated_at: string;
};
