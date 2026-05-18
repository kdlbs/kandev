"use client";

import { cn } from "@/lib/utils";
import { StatusIcon as TaskListStatusIcon } from "../status-icon";
import type { TaskStatus } from "./types";
import type { OfficeTaskStatus } from "@/lib/state/slices/office/types";

type StatusIconProps = {
  status: TaskStatus;
  className?: string;
};

/**
 * Re-uses the task-list StatusIcon so detail-page components render the
 * same outline-style icons. Normalises the local TaskStatus string into
 * the OfficeTaskStatus union expected by the underlying component.
 */
export function StatusIcon({ status, className }: StatusIconProps) {
  const normalised = (status?.toLowerCase().replace(/ /g, "_") as OfficeTaskStatus) ?? "todo";
  return <TaskListStatusIcon status={normalised} className={cn(className)} />;
}
