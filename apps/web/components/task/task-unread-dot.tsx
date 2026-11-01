"use client";

import { useTranslation } from "react-i18next";

/** In-flow badge marking a task with unread messages; fixed size keeps row height. */
export function TaskUnreadDot() {
  const { t } = useTranslation();
  return (
    <span
      role="img"
      data-testid="task-unread-dot"
      aria-label={t("task:taskHasUnreadMessages")}
      className="inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center"
    >
      <span aria-hidden="true" className="h-2 w-2 rounded-full bg-primary" />
    </span>
  );
}
