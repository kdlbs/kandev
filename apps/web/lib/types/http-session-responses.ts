import type { TaskSession } from "./http";
import type { WorkflowStepDTO } from "./http";

export type TaskSessionsResponse = {
  sessions: TaskSession[];
  total: number;
};

export type TaskSessionResponse = {
  session: TaskSession;
};

/**
 * Response for POST /task-sessions/:id/mark-read. Intentionally narrow —
 * only the advanced read cursor, never a full session snapshot — so
 * applying it can never clobber a newer field (e.g. state) written by a
 * concurrent WebSocket update while the request was in flight.
 */
export type MarkSessionReadResponse = {
  session_id: string;
  last_read_message_id: string;
};

export type ApproveSessionResponse = {
  success: boolean;
  session: TaskSession;
  workflow_step?: WorkflowStepDTO;
};
