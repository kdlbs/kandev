"use client";

import { useEffect } from "react";
import { useAppStore } from "@/components/state-provider";
import type { OrchestrateIssue } from "@/lib/state/slices/orchestrate/types";
import { IssuesList } from "./issues-list";

type IssuesPageClientProps = {
  initialIssues: OrchestrateIssue[];
};

export function IssuesPageClient({ initialIssues }: IssuesPageClientProps) {
  const setIssues = useAppStore((s) => s.setIssues);

  useEffect(() => {
    if (initialIssues.length > 0) {
      setIssues(initialIssues);
    }
  }, [initialIssues, setIssues]);

  return <IssuesList />;
}
