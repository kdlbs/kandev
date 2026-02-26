import type { ComponentType } from "react";
import { IconAlertCircle, IconCheck, IconLoader2, IconX } from "@tabler/icons-react";
import type { TaskSessionState, TaskState } from "@/lib/types/http";
import { cn } from "@/lib/utils";

type IconConfig = {
  Icon: ComponentType<{ className?: string }>;
  className: string;
};

const STYLE_MUTED = "text-muted-foreground";
const STYLE_LOADING = "text-blue-500 animate-spin";
const STYLE_WARNING = "text-yellow-500";
const STYLE_ERROR = "text-red-500";

const TASK_STATE_ICONS: Record<TaskState, IconConfig> = {
  CREATED: { Icon: IconAlertCircle, className: STYLE_MUTED },
  SCHEDULING: { Icon: IconLoader2, className: STYLE_LOADING },
  IN_PROGRESS: { Icon: IconLoader2, className: STYLE_LOADING },
  REVIEW: { Icon: IconCheck, className: STYLE_WARNING },
  BLOCKED: { Icon: IconAlertCircle, className: STYLE_WARNING },
  WAITING_FOR_INPUT: { Icon: IconCheck, className: STYLE_WARNING },
  COMPLETED: { Icon: IconCheck, className: "text-green-500" },
  FAILED: { Icon: IconX, className: STYLE_ERROR },
  CANCELLED: { Icon: IconX, className: STYLE_ERROR },
  TODO: { Icon: IconAlertCircle, className: STYLE_MUTED },
};

const SESSION_STATE_ICONS: Record<TaskSessionState, IconConfig> = {
  CREATED: { Icon: IconAlertCircle, className: STYLE_MUTED },
  STARTING: { Icon: IconLoader2, className: STYLE_LOADING },
  RUNNING: { Icon: IconLoader2, className: STYLE_LOADING },
  WAITING_FOR_INPUT: { Icon: IconCheck, className: STYLE_WARNING },
  COMPLETED: { Icon: IconCheck, className: "text-green-500" },
  FAILED: { Icon: IconX, className: STYLE_ERROR },
  CANCELLED: { Icon: IconX, className: STYLE_ERROR },
};

const DEFAULT_TASK_ICON: IconConfig = {
  Icon: IconAlertCircle,
  className: STYLE_MUTED,
};

const DEFAULT_SESSION_ICON: IconConfig = {
  Icon: IconAlertCircle,
  className: STYLE_MUTED,
};

export function getTaskStateIcon(state?: TaskState, className?: string) {
  const config = state ? (TASK_STATE_ICONS[state] ?? DEFAULT_TASK_ICON) : DEFAULT_TASK_ICON;
  return <config.Icon className={cn("h-4 w-4", config.className, className)} />;
}

export function getSessionStateIcon(state?: TaskSessionState, className?: string) {
  const config = state
    ? (SESSION_STATE_ICONS[state] ?? DEFAULT_SESSION_ICON)
    : DEFAULT_SESSION_ICON;
  return <config.Icon className={cn("h-4 w-4", config.className, className)} />;
}
