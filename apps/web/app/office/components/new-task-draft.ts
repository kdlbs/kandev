"use client";

import { useState, useEffect, useCallback, useRef } from "react";
import { clampTaskTitleInput } from "@/lib/task-title";
import type { TaskPriority } from "@/lib/types/http";
import type { StoredFileAttachment } from "@/lib/stored-chat-draft-attachments";

export type IssueDraft = {
  title: string;
  description: string;
  assigneeId: string;
  projectId: string;
  status: string;
  priority: TaskPriority;
  /** Ready staged descriptors only; file bytes never enter browser storage. */
  attachments?: StoredFileAttachment[];
};

const EMPTY_DRAFT: IssueDraft = {
  title: "",
  description: "",
  assigneeId: "",
  projectId: "",
  status: "todo",
  priority: "medium",
  attachments: [],
};

function draftKey(workspaceId: string | null, parentTaskId?: string) {
  const base = `kandev:issue-draft:${workspaceId ?? "none"}`;
  return parentTaskId ? `${base}:${parentTaskId}` : base;
}

export function useIssueDraft(
  workspaceId: string | null,
  parentTaskId?: string,
  defaults?: { projectId?: string; assigneeId?: string },
) {
  const key = draftKey(workspaceId, parentTaskId);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const draftRef = useRef<IssueDraft | null>(null);
  const skipFlushRef = useRef(false);

  const [draft, setDraft] = useState<IssueDraft>(() => {
    if (typeof window === "undefined") return { ...EMPTY_DRAFT, ...defaults };
    try {
      const stored = localStorage.getItem(key);
      if (stored) {
        const parsed = JSON.parse(stored) as IssueDraft;
        return {
          ...parsed,
          title: clampTaskTitleInput(parsed.title ?? ""),
          attachments: Array.isArray(parsed.attachments) ? parsed.attachments : [],
        };
      }
    } catch {
      // ignore
    }
    return {
      ...EMPTY_DRAFT,
      projectId: defaults?.projectId ?? "",
      assigneeId: defaults?.assigneeId ?? "",
    };
  });

  draftRef.current = draft;

  // Debounced save to localStorage
  useEffect(() => {
    if (skipFlushRef.current) return;
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => {
      try {
        localStorage.setItem(key, JSON.stringify(draft));
      } catch {
        // ignore quota errors
      }
    }, 800);
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [draft, key]);

  // A keyed session can unmount between debounced writes (workspace or parent
  // switch). Flush the outgoing owner once so recent edits are not lost.
  useEffect(
    () => () => {
      if (timerRef.current) clearTimeout(timerRef.current);
      if (skipFlushRef.current || draftRef.current === null) return;
      try {
        localStorage.setItem(key, JSON.stringify(draftRef.current));
      } catch {
        // ignore quota errors
      }
    },
    [key],
  );

  const updateDraft = useCallback((patch: Partial<IssueDraft>) => {
    skipFlushRef.current = false;
    setDraft((prev) => ({ ...prev, ...patch }));
  }, []);

  const clearDraft = useCallback(() => {
    skipFlushRef.current = true;
    if (timerRef.current) clearTimeout(timerRef.current);
    setDraft({ ...EMPTY_DRAFT });
    try {
      localStorage.removeItem(key);
    } catch {
      // ignore
    }
  }, [key]);

  return { draft, updateDraft, clearDraft };
}
