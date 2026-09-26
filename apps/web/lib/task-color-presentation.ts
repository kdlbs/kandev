import type { FixedAutomaticTaskColor } from "@/lib/task-color-automation-settings";
import { TASK_COLOR_BAR_CLASS, type TaskColor } from "@/lib/task-colors";

export type TaskMarkerPresentation =
  | { token: FixedAutomaticTaskColor; className: string }
  | { token: "custom"; style: { backgroundColor: string } };

export type ManualTaskColorPresentation = { token: TaskColor; className: string };

export function manualTaskColorPresentation(
  taskColor: TaskColor | null,
): ManualTaskColorPresentation | null {
  if (!taskColor) return null;
  return { token: taskColor, className: TASK_COLOR_BAR_CLASS[taskColor] };
}

export function resolveTaskItemColor(
  automaticColor: TaskMarkerPresentation | undefined,
  manualColor: ManualTaskColorPresentation | null,
): TaskMarkerPresentation | ManualTaskColorPresentation | null {
  return automaticColor ?? manualColor;
}

const AUTOMATIC_TASK_COLOR_BAR_CLASS: Record<FixedAutomaticTaskColor, string> = {
  gray: "bg-slate-500",
  red: "bg-red-500",
  orange: "bg-orange-500",
  yellow: "bg-yellow-500",
  green: "bg-green-500",
  cyan: "bg-cyan-500",
  blue: "bg-blue-500",
  indigo: "bg-indigo-500",
  purple: "bg-purple-500",
  pink: "bg-pink-500",
};

const WORKFLOW_COLOR_TOKEN: Record<string, FixedAutomaticTaskColor> = {
  gray: "gray",
  grey: "gray",
  slate: "gray",
  zinc: "gray",
  neutral: "gray",
  stone: "gray",
  red: "red",
  orange: "orange",
  amber: "yellow",
  yellow: "yellow",
  lime: "green",
  green: "green",
  emerald: "green",
  teal: "cyan",
  cyan: "cyan",
  sky: "blue",
  blue: "blue",
  indigo: "indigo",
  violet: "purple",
  purple: "purple",
  fuchsia: "pink",
  pink: "pink",
};

const WORKFLOW_CLASS_TOKEN: Record<string, FixedAutomaticTaskColor> = {
  "bg-slate-500": "gray",
  "bg-gray-500": "gray",
  "bg-zinc-500": "gray",
  "bg-neutral-400": "gray",
  "bg-neutral-500": "gray",
  "bg-stone-500": "gray",
  "bg-red-500": "red",
  "bg-orange-500": "orange",
  "bg-amber-500": "yellow",
  "bg-yellow-500": "yellow",
  "bg-lime-500": "green",
  "bg-green-500": "green",
  "bg-emerald-500": "green",
  "bg-teal-500": "cyan",
  "bg-cyan-500": "cyan",
  "bg-sky-500": "blue",
  "bg-blue-500": "blue",
  "bg-indigo-500": "indigo",
  "bg-violet-500": "purple",
  "bg-purple-500": "purple",
  "bg-fuchsia-500": "pink",
  "bg-pink-500": "pink",
};

export function taskColorPresentation(token: FixedAutomaticTaskColor): TaskMarkerPresentation {
  return { token, className: AUTOMATIC_TASK_COLOR_BAR_CLASS[token] };
}

export function fixedAutomaticTaskColor(token: FixedAutomaticTaskColor): TaskMarkerPresentation {
  return taskColorPresentation(token);
}

/** Maps persisted workflow color data to a safe marker presentation. */
export function parseWorkflowStepColor(value: string | undefined): TaskMarkerPresentation {
  if (!value) return taskColorPresentation("gray");
  const hex = value.match(/^#(?:[0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$/);
  if (hex) return { token: "custom", style: { backgroundColor: hex[0] } };

  const token = WORKFLOW_CLASS_TOKEN[value] ?? WORKFLOW_COLOR_TOKEN[value.toLowerCase()];
  return token ? taskColorPresentation(token) : taskColorPresentation("gray");
}
