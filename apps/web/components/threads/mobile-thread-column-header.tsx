"use client";

import { IconArrowsMaximize, IconChevronDown } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { useTranslation } from "react-i18next";
import type { ActiveThread } from "@/lib/threads/active-threads";
import type { ThreadStatus } from "@/lib/threads/thread-session-status";
import type { TaskSession } from "@/lib/types/http";
import { ThreadSessionStatusIcon, ThreadSessionSwitcher } from "./thread-session-switcher";
import { ThreadTaskMenuButton } from "./thread-task-actions";

export type MobileThreadNavigation = {
  onChoose: () => void;
};

export function MobileThreadColumnHeader({
  thread,
  status,
  sessions,
  selectedSessionId,
  onSelectSession,
  onOpenTask,
  navigation,
}: {
  thread: ActiveThread;
  status: ThreadStatus;
  sessions: readonly TaskSession[];
  selectedSessionId: string | null;
  onSelectSession: (sessionId: string) => void;
  onOpenTask: (taskId: string) => void;
  navigation: MobileThreadNavigation;
}) {
  const { t } = useTranslation();
  return (
    <header className="min-w-0 shrink-0 border-b px-3 py-2">
      <div className="flex min-w-0 items-center gap-1">
        <Button
          variant="ghost"
          className="h-auto min-h-11 min-w-0 flex-1 cursor-pointer justify-start gap-2 py-2 pl-1 pr-3.5 text-left"
          onClick={navigation.onChoose}
          aria-label={t("threads:chooseThreadLabel", { title: thread.title })}
          aria-haspopup="dialog"
          data-testid="thread-picker-trigger"
        >
          <span
            className="line-clamp-2 min-w-0 flex-1 whitespace-normal break-words text-balance text-base font-semibold leading-snug"
            data-testid="thread-mobile-title"
          >
            {thread.title}
          </span>
          <IconChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          className="h-11 w-11 shrink-0 cursor-pointer"
          aria-label={t("threads:openTask")}
          onClick={() => onOpenTask(thread.taskId)}
        >
          <IconArrowsMaximize className="h-4 w-4" />
        </Button>
        <ThreadTaskMenuButton taskId={thread.taskId} />
      </div>
      <div className="flex min-h-8 min-w-0 items-center justify-between gap-2 px-1">
        <span className="flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground">
          <span aria-hidden="true">
            <ThreadSessionStatusIcon
              status={status}
              label={t(status.labelKey)}
              testId={`thread-status-${status.kind}`}
            />
          </span>
          <span className="truncate">{t(status.labelKey)}</span>
        </span>
        <ThreadSessionSwitcher
          sessions={sessions}
          selectedSessionId={selectedSessionId}
          onSelect={onSelectSession}
        />
      </div>
    </header>
  );
}
