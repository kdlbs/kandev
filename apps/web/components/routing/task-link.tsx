"use client";

import { forwardRef } from "react";
import Link, { type AppLinkProps } from "./app-link";
import { linkToTask, type TaskLinkOptions } from "@/lib/links";

export type TaskLinkProps = Omit<AppLinkProps, "href"> &
  TaskLinkOptions & {
    taskId: string;
  };

const TaskLink = forwardRef<HTMLAnchorElement, TaskLinkProps>(function TaskLink(
  { taskId, layout, sessionId, searchParams, ...props },
  ref,
) {
  return (
    <Link {...props} ref={ref} href={linkToTask(taskId, { layout, sessionId, searchParams })} />
  );
});

export default TaskLink;
