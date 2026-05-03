// backend/src/events/taskEvents.ts
import { Task } from '../models/task';

interface TaskEventPayload {
  id: string;
  title: string;
  status: string;
  assignee: string | null;
  metadata: Record<string, any> | null;
  parent_id: string | null;
  repository_id: string | null;
  isPRReview?: boolean;
}

export function buildTaskEventPayload(task: Task): TaskEventPayload {
  return {
    id: task.id,
    title: task.title,
    status: task.status,
    assignee: task.assignee,
    metadata: task.metadata || null,
    parent_id: task.parent_id || null,
    repository_id: task.repository_id || null,
    isPRReview: task.metadata?.type === 'pr_review' || false,
  };
}
