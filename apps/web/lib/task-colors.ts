import { getLocalStorage, setLocalStorage } from "./local-storage";

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

export const TASK_COLOR_LABEL: Record<TaskColor, string> = {
  red: "Red",
  orange: "Orange",
  yellow: "Yellow",
  green: "Green",
  blue: "Blue",
  purple: "Purple",
  pink: "Pink",
};

export const TASK_COLORS_STORAGE_KEY = "kandev.taskColors";
export const TASK_COLORS_CHANGED_EVENT = "kandev:task-colors-changed";

function isTaskColor(value: unknown): value is TaskColor {
  return typeof value === "string" && (TASK_COLORS as readonly string[]).includes(value);
}

function readAll(): Record<string, TaskColor> {
  const raw = getLocalStorage<Record<string, string>>(TASK_COLORS_STORAGE_KEY, {});
  if (!raw || typeof raw !== "object") return {};
  const out: Record<string, TaskColor> = {};
  for (const [taskId, color] of Object.entries(raw)) {
    if (typeof taskId === "string" && isTaskColor(color)) out[taskId] = color;
  }
  return out;
}

function writeAll(map: Record<string, TaskColor>): void {
  setLocalStorage(TASK_COLORS_STORAGE_KEY, map);
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent(TASK_COLORS_CHANGED_EVENT));
  }
}

export function getTaskColor(taskId: string): TaskColor | null {
  if (!taskId) return null;
  return readAll()[taskId] ?? null;
}

export function setTaskColor(taskId: string, color: TaskColor | null): void {
  if (!taskId) return;
  const all = readAll();
  if (color === null) {
    if (!(taskId in all)) return;
    delete all[taskId];
  } else {
    if (all[taskId] === color) return;
    all[taskId] = color;
  }
  writeAll(all);
}
