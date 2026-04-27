"use client";

import { use, useMemo } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { TaskSimpleMode } from "./task-simple-mode";
import { TaskAdvancedMode } from "./task-advanced-mode";
import type { Issue, IssueComment, IssueActivityEntry, TaskSession } from "./types";

type IssueDetailPageProps = {
  params: Promise<{ id: string }>;
};

/**
 * Placeholder data until Wave 3A backend lands.
 * Returns a mock issue for rendering the UI skeleton.
 */
function useMockIssue(id: string): {
  task: Issue;
  comments: IssueComment[];
  activity: IssueActivityEntry[];
  sessions: TaskSession[];
} {
  return useMemo(
    () => ({
      task: {
        id,
        workspaceId: "ws-1",
        identifier: "KAN-1",
        title: "Example issue",
        description: "This is a placeholder issue for the task detail page.",
        status: "todo",
        priority: "medium",
        labels: [],
        blockedBy: [],
        blocking: [],
        children: [],
        reviewers: [],
        approvers: [],
        createdBy: "User",
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
      comments: [],
      activity: [],
      sessions: [],
    }),
    [id],
  );
}

export default function IssueDetailPage({ params }: IssueDetailPageProps) {
  const { id } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const mode = searchParams.get("mode") || "simple";

  // TODO: Replace with real data fetch once Wave 3A backend is ready
  const { task, comments, activity, sessions } = useMockIssue(id);

  const hasSession = Boolean(task.assigneeAgentInstanceId) || sessions.length > 0;

  const setMode = (newMode: string) => {
    const url =
      newMode === "advanced"
        ? `/orchestrate/issues/${id}?mode=advanced`
        : `/orchestrate/issues/${id}`;
    router.push(url);
  };

  if (mode === "advanced" && hasSession) {
    return <TaskAdvancedMode task={task} onToggleSimple={() => setMode("simple")} />;
  }

  return (
    <TaskSimpleMode
      task={task}
      comments={comments}
      activity={activity}
      sessions={sessions}
      onToggleAdvanced={hasSession ? () => setMode("advanced") : undefined}
    />
  );
}
