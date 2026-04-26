"use client";

import {
  IconCircle,
  IconCircleDot,
  IconCircleCheck,
  IconCircleX,
  IconCircleDotted,
  IconProgress,
} from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { IssueStatus } from "./types";

const statusConfig: Record<IssueStatus, { icon: typeof IconCircle; className: string }> = {
  backlog: { icon: IconCircleDotted, className: "text-muted-foreground" },
  todo: { icon: IconCircle, className: "text-blue-600 dark:text-blue-400" },
  in_progress: { icon: IconProgress, className: "text-yellow-600 dark:text-yellow-400" },
  in_review: { icon: IconCircleDot, className: "text-violet-600 dark:text-violet-400" },
  done: { icon: IconCircleCheck, className: "text-green-600 dark:text-green-400" },
  cancelled: { icon: IconCircleX, className: "text-neutral-500" },
  blocked: { icon: IconCircleDot, className: "text-red-600 dark:text-red-400" },
};

type StatusIconProps = {
  status: IssueStatus;
  className?: string;
};

export function StatusIcon({ status, className }: StatusIconProps) {
  const config = statusConfig[status];
  const Icon = config.icon;
  return <Icon className={cn("h-4 w-4", config.className, className)} />;
}
