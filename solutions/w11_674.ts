// backend/src/events/taskEvents.ts

import { Task } from '../models/Task';

interface TaskEventPayload {
  id: string;
  title: string;
  status: string;
  assignee: string | null;
  repository_id: string | null;
  metadata: Record<string, any> | null;
  parent_id: string | null;
  isPRReview: boolean;
  updated_at: string;
}

export function buildTaskEventPayload(task: Task): TaskEventPayload {
  // Ensure metadata and parent_id are forwarded from the raw task
  const metadata = task.metadata || null;
  const parent_id = task.parent_id || null;

  // Derive isPRReview from metadata if available
  const isPRReview = metadata?.type === 'pr_review' || metadata?.isPRReview === true;

  return {
    id: task.id,
    title: task.title,
    status: task.status,
    assignee: task.assignee,
    repository_id: task.repository_id || null,
    metadata,
    parent_id,
    isPRReview,
    updated_at: task.updated_at.toISOString(),
  };
}
