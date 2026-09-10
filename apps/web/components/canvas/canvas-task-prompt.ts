// i18n-exempt: exact saved-prompt reference that users can edit in task text.
export const CANVAS_CREATE_PROMPT_REFERENCE = "@create-canvas";

export function buildCanvasCreateTaskPrompt(goal: string): string {
  const normalizedGoal = goal.trim();
  return normalizedGoal
    ? `${normalizedGoal}\n\n${CANVAS_CREATE_PROMPT_REFERENCE}`
    : CANVAS_CREATE_PROMPT_REFERENCE;
}
