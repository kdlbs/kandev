import { getLocalStorage, removeLocalStorage } from "./local-storage";

export const TASK_COLORS = ["red", "orange", "yellow", "green", "blue", "purple", "pink"] as const;

export type TaskColor = (typeof TASK_COLORS)[number];

export const TASK_COLOR_BAR_CLASS: Record<TaskColor, string> = {
  red: "bg-red-500",
  orange: "bg-orange-500",
  yellow: "bg-yellow-500",
  green: "bg-green-500",
  blue: "bg-blue-500",
  purple: "bg-purple-500",
  pink: "bg-pink-500",
};

/**
 * Catalog keys, not resolved labels: a `t()` at module scope would resolve once
 * at import and freeze at the boot locale. The colour menu resolves at render.
 *
 * The record keys are the persisted `TaskColor` values (localStorage, and the
 * `TASK_COLOR_BAR_CLASS` lookup) and are never translated.
 */
export const TASK_COLOR_LABEL_KEYS: Record<TaskColor, string> = {
  red: "task:colorRed",
  orange: "task:colorOrange",
  yellow: "task:colorYellow",
  green: "task:colorGreen",
  blue: "task:colorBlue",
  purple: "task:colorPurple",
  pink: "task:colorPink",
};

export const TASK_COLORS_STORAGE_KEY = "kandev.taskColors";
export const MAX_TASK_COLOR_TASK_ID_BYTES = 128;
export const MAX_TASK_COLOR_MAP_ENTRIES = 10_000;

function isTaskColor(value: unknown): value is TaskColor {
  return typeof value === "string" && (TASK_COLORS as readonly string[]).includes(value);
}

/** Parses the backend manual-color map and drops malformed entries. */
export function parseSidebarTaskColors(value: unknown): Record<string, TaskColor | null> {
  if (!value || typeof value !== "object" || Array.isArray(value)) return {};
  const result: Record<string, TaskColor | null> = {};
  let entryCount = 0;
  for (const [taskId, color] of Object.entries(value)) {
    if (entryCount >= MAX_TASK_COLOR_MAP_ENTRIES) break;
    if (!isValidTaskColorTaskId(taskId)) continue;
    if (color === null || isTaskColor(color)) {
      result[taskId] = color;
      entryCount += 1;
    }
  }
  return result;
}

function isValidTaskColorTaskId(taskId: string): boolean {
  if (!taskId) return false;
  try {
    const encoded = encodeURIComponent(taskId).replace(/%[0-9a-f]{2}/gi, "_");
    return encoded.length <= MAX_TASK_COLOR_TASK_ID_BYTES;
  } catch {
    return false;
  }
}

/** Reads legacy browser colors as migration input only. */
export function readLegacyTaskColors(): Record<string, TaskColor> {
  const parsed = parseSidebarTaskColors(getLocalStorage(TASK_COLORS_STORAGE_KEY, {}));
  const result: Record<string, TaskColor> = {};
  for (const [taskId, color] of Object.entries(parsed)) {
    if (color !== null) result[taskId] = color;
  }
  return result;
}

/** Removes the legacy browser key after a complete migration. */
export function clearLegacyTaskColors(): void {
  removeLocalStorage(TASK_COLORS_STORAGE_KEY);
}
