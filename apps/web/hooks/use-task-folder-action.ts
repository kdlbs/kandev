"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { buildWorktreeOptions } from "@/components/task/editor-worktree-options";
import { useSessionWorktrees } from "@/hooks/domains/session/use-session-worktrees";
import { useOpenSessionFolder } from "@/hooks/use-open-session-folder";

export function useTaskFolderAction(sessionId: string | null) {
  const request = useOpenSessionFolder(sessionId);
  const worktrees = useSessionWorktrees(sessionId);
  const repositories = useAppStore((state) => state.repositories.itemsByWorkspaceId);
  const options = useMemo(
    () => buildWorktreeOptions(worktrees, Object.values(repositories).flat()),
    [worktrees, repositories],
  );
  const [pickerSessionId, setPickerSessionId] = useState<string | null>(null);
  const opener = useRef<HTMLElement | null>(null);
  useEffect(() => {
    setPickerSessionId(null);
  }, [sessionId]);
  const pickerOpen = Boolean(request.available && sessionId && pickerSessionId === sessionId);

  return {
    options,
    pickerOpen,
    isLoading: request.isLoading,
    disabled: !sessionId || !request.available || request.isLoading,
    open: (trigger?: HTMLElement) => {
      if (!sessionId || !request.available || request.isLoading) return;
      if (options.length > 1) {
        opener.current = trigger ?? (document.activeElement as HTMLElement | null);
        setPickerSessionId(sessionId);
      } else {
        void request.open(options[0]?.worktreeId);
      }
    },
    select: (worktreeId: string) => {
      if (!pickerOpen || !options.some((option) => option.worktreeId === worktreeId)) return;
      setPickerSessionId(null);
      void request.open(worktreeId);
    },
    onOpenChange: (open: boolean) => {
      if (!open) setPickerSessionId(null);
    },
    onCloseAutoFocus: (event: Event) => {
      event.preventDefault();
      if (opener.current?.isConnected) opener.current.focus();
    },
  };
}
