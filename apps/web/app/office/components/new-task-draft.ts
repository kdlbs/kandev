"use client";

import { useState, useEffect, useCallback, useRef } from "react";

export type IssueDraft = {
  title: string;
  description: string;
  assigneeId: string;
  projectId: string;
  status: string;
  priority: string;
  showReviewer: boolean;
  showApprover: boolean;
  reviewerIds: string[];
  approverIds: string[];
};

const EMPTY_DRAFT: IssueDraft = {
  title: "",
  description: "",
  assigneeId: "",
  projectId: "",
  status: "todo",
  priority: "medium",
  showReviewer: false,
  showApprover: false,
  reviewerIds: [],
  approverIds: [],
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

  const [draft, setDraft] = useState<IssueDraft>(() => {
    if (typeof window === "undefined") return { ...EMPTY_DRAFT, ...defaults };
    try {
      const stored = localStorage.getItem(key);
      if (stored) return JSON.parse(stored) as IssueDraft;
    } catch {
      // ignore
    }
    return {
      ...EMPTY_DRAFT,
      projectId: defaults?.projectId ?? "",
      assigneeId: defaults?.assigneeId ?? "",
    };
  });

  // Debounced save to localStorage
  useEffect(() => {
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

  const updateDraft = useCallback(
    (patch: Partial<IssueDraft>) => setDraft((prev) => ({ ...prev, ...patch })),
    [],
  );

  const clearDraft = useCallback(() => {
    setDraft({ ...EMPTY_DRAFT });
    try {
      localStorage.removeItem(key);
    } catch {
      // ignore
    }
  }, [key]);

  return { draft, updateDraft, clearDraft };
}
