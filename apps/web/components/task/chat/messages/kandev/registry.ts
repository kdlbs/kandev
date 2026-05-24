// Tool-stem → renderer registry. Keep this file pure (no JSX) so it can be
// imported from the matcher in `message-renderer.tsx` without dragging the
// React component tree along.

import {
  ListAgentsRenderer,
  ListExecutorProfilesRenderer,
  ListRelatedTasksRenderer,
  ListTaskDocumentsRenderer,
  ListTasksRenderer,
  ListWorkflowStepsRenderer,
  ListWorkflowsRenderer,
  ListWorkspacesRenderer,
} from "./list-renderers";
import {
  CreateTaskRenderer,
  MessageTaskRenderer,
  MoveTaskRenderer,
  UpdateTaskRenderer,
} from "./task-renderers";
import {
  CreateTaskPlanRenderer,
  DeleteTaskPlanRenderer,
  GetTaskConversationRenderer,
  GetTaskDocumentRenderer,
  GetTaskPlanRenderer,
  UpdateTaskPlanRenderer,
  WriteTaskDocumentRenderer,
} from "./document-renderers";
import { AskUserQuestionRenderer } from "./ask-user-question-renderer";
import type { KandevRenderer } from "./types";

export const KANDEV_RENDERERS: Record<string, KandevRenderer> = {
  list_workspaces: ListWorkspacesRenderer,
  list_workflows: ListWorkflowsRenderer,
  list_workflow_steps: ListWorkflowStepsRenderer,
  list_tasks: ListTasksRenderer,
  list_related_tasks: ListRelatedTasksRenderer,
  list_agents: ListAgentsRenderer,
  list_executor_profiles: ListExecutorProfilesRenderer,
  list_task_documents: ListTaskDocumentsRenderer,

  create_task: CreateTaskRenderer,
  update_task: UpdateTaskRenderer,
  move_task: MoveTaskRenderer,
  message_task: MessageTaskRenderer,

  get_task_plan: GetTaskPlanRenderer,
  create_task_plan: CreateTaskPlanRenderer,
  update_task_plan: UpdateTaskPlanRenderer,
  delete_task_plan: DeleteTaskPlanRenderer,
  get_task_document: GetTaskDocumentRenderer,
  write_task_document: WriteTaskDocumentRenderer,
  get_task_conversation: GetTaskConversationRenderer,

  ask_user_question: AskUserQuestionRenderer,
};

export function getKandevRenderer(stem: string | null): KandevRenderer | null {
  if (!stem) return null;
  return KANDEV_RENDERERS[stem] ?? null;
}
