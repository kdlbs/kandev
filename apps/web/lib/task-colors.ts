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

let cachedMap: Record<string, TaskColor> | null = null;

if (typeof window !== "undefined") {
  window.addEventListener("storage", (e) => {
    if (e.key === null || e.key === TASK_COLORS_STORAGE_KEY) cachedMap = null;
  });
}

function readAll(): Record<string, TaskColor> {
  if (cachedMap) return cachedMap;
  const raw = getLocalStorage<Record<string, string>>(TASK_COLORS_STORAGE_KEY, {});
  const out: Record<string, TaskColor> = {};
  if (raw && typeof raw === "object") {
    for (const [taskId, color] of Object.entries(raw)) {
      if (isTaskColor(color)) out[taskId] = color;
    }
  }
  cachedMap = out;
  return out;
}

function writeAll(map: Record<string, TaskColor>): void {
  cachedMap = map;
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
    const next = { ...all };
    delete next[taskId];
    writeAll(next);
  } else {
    if (all[taskId] === color) return;
    writeAll({ ...all, [taskId]: color });
  }
}

/** Test-only: clears the in-memory cache so localStorage mutations from tests are observed. */
export function __resetTaskColorsCacheForTests(): void {
  cachedMap = null;
}
