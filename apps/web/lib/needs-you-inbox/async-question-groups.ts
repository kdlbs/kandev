import type { ClarificationInboxAsyncQuestion } from "@/lib/types/clarification-inbox";

// i18n-exempt: namespaced per-user sidecar identity, not user-facing copy
export const ASYNC_SIDECAR_PREFIX = "async:";

export type AsyncQuestionTaskGroup = {
  taskId: string;
  taskTitle: string;
  questions: ClarificationInboxAsyncQuestion[];
};

/**
 * Groups async requests by their originating task while preserving the
 * server order. The count stays a request count, not a group count.
 */
export function groupAsyncQuestionsByTask(
  questions: readonly ClarificationInboxAsyncQuestion[],
): AsyncQuestionTaskGroup[] {
  const groups = new Map<string, AsyncQuestionTaskGroup>();
  for (const question of questions) {
    const existing = groups.get(question.task_id);
    if (existing) {
      existing.questions.push(question);
      continue;
    }
    groups.set(question.task_id, {
      taskId: question.task_id,
      taskTitle: question.task_title,
      questions: [question],
    });
  }
  return [...groups.values()];
}

export function asyncQuestionPrimaryText(question: ClarificationInboxAsyncQuestion): string {
  const first = question.questions[0];
  return first?.title || first?.prompt || question.context || question.task_title;
}
