"use client";

import { IconCheck } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { MobilePickerSheet } from "@/components/task/mobile/mobile-picker-sheet";
import type { ActiveThread } from "@/lib/threads/active-threads";
import { resolveThreadColumnStatus } from "@/lib/threads/thread-session-status";
import { ThreadSessionStatusIcon } from "./thread-session-switcher";

export function MobileThreadPicker({
  threads,
  selectedTaskId,
  open,
  onOpenChange,
  onSelect,
  onCloseAutoFocus,
}: {
  threads: readonly ActiveThread[];
  selectedTaskId: string | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (taskId: string) => void;
  onCloseAutoFocus: (event: Event) => void;
}) {
  const { t } = useTranslation();
  return (
    <MobilePickerSheet
      open={open}
      onOpenChange={onOpenChange}
      title={t("threads:chooseThread")}
      description={t("threads:chooseThreadDescription")}
      contentTestId="thread-picker-sheet"
      onCloseAutoFocus={onCloseAutoFocus}
    >
      <div className="flex flex-col gap-1">
        {threads.map((thread) => {
          const selected = selectedTaskId === thread.taskId;
          const status = resolveThreadColumnStatus({
            taskState: thread.taskState,
            reviewStatus: thread.reviewStatus,
            taskPendingAction: thread.taskPendingAction,
            session: { state: thread.sessionState, pending_action: thread.pendingAction },
          });
          return (
            <button
              key={thread.taskId}
              type="button"
              className="flex min-h-11 w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-3 text-left hover:bg-muted active:bg-muted data-[selected=true]:bg-muted"
              data-testid={`thread-picker-row-${thread.taskId}`}
              data-selected={selected}
              aria-current={selected ? "true" : undefined}
              onClick={() => onSelect(thread.taskId)}
            >
              <ThreadSessionStatusIcon status={status} label={t(status.labelKey)} />
              <span className="flex min-w-0 flex-1 flex-col gap-1">
                <span className="line-clamp-2 break-words text-sm font-medium">{thread.title}</span>
                <span className="truncate text-xs text-muted-foreground">{t(status.labelKey)}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {thread.workflowName}
                  {thread.stepTitle && ` · ${thread.stepTitle}`}
                </span>
                {(thread.activeSubagentCount > 0 || thread.queuedPromptCount > 0) && (
                  <span className="flex flex-wrap gap-x-2 text-xs text-muted-foreground">
                    {thread.activeSubagentCount > 0 && (
                      <span>
                        {t("threads:subagentCount", { count: thread.activeSubagentCount })}
                      </span>
                    )}
                    {thread.queuedPromptCount > 0 && (
                      <span>
                        {t("threads:queuedPromptCount", { count: thread.queuedPromptCount })}
                      </span>
                    )}
                  </span>
                )}
              </span>
              {selected && <IconCheck aria-hidden="true" className="h-4 w-4 shrink-0" />}
            </button>
          );
        })}
      </div>
    </MobilePickerSheet>
  );
}
