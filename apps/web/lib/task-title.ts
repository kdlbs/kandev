export const TASK_TITLE_MAX_LENGTH = 60;

function takeTitleCharacters(value: string, length: number): string {
  return Array.from(value).slice(0, length).join("");
}

/** Clamp text entered directly into a task-title field. */
export function clampTaskTitleInput(value: string): string {
  return takeTitleCharacters(value, TASK_TITLE_MAX_LENGTH);
}

/** Shorten a title supplied by a remote issue/PR while preserving the limit. */
export function truncateRemoteTaskTitle(value: string): string {
  const characters = Array.from(value);
  if (characters.length <= TASK_TITLE_MAX_LENGTH) return value;
  return `${characters.slice(0, TASK_TITLE_MAX_LENGTH - 1).join("")}…`;
}
