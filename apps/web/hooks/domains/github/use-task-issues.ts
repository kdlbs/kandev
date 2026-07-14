"use client";

import { useEffect, useRef } from "react";
import { useAppStore } from "@/components/state-provider";
import { listWorkspaceTaskIssues } from "@/lib/api/domains/github-api";

export function useWorkspaceTaskIssues(workspaceId: string | null) {
  const setTaskIssues = useAppStore((state) => state.setTaskIssues);
  const fetchedRef = useRef<string | null>(null);
  const requestRef = useRef(0);

  useEffect(() => {
    if (!workspaceId) {
      requestRef.current += 1;
      fetchedRef.current = null;
      return;
    }
    if (fetchedRef.current === workspaceId) return;

    const requestId = ++requestRef.current;
    fetchedRef.current = workspaceId;
    listWorkspaceTaskIssues(workspaceId, { cache: "no-store" })
      .then((response) => {
        if (requestRef.current !== requestId) return;
        setTaskIssues(workspaceId, response?.task_issues ?? {});
      })
      .catch(() => {
        if (requestRef.current === requestId) fetchedRef.current = null;
      });
  }, [setTaskIssues, workspaceId]);
}
