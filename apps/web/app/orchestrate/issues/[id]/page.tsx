"use client";

import { use, useState, useEffect, useCallback } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useAppStore } from "@/components/state-provider";
import {
  getIssue,
  listComments,
  type TaskCommentResponse,
} from "@/lib/api/domains/orchestrate-api";
import { TaskSimpleMode } from "./task-simple-mode";
import { TaskAdvancedMode } from "./task-advanced-mode";
import { IssueDetailSkeleton } from "./issue-detail-skeleton";
import type { Issue, IssueComment, IssueActivityEntry, TaskSession } from "./types";
import type { OrchestrateIssue } from "@/lib/state/slices/orchestrate/types";

type IssueDetailPageProps = {
  params: Promise<{ id: string }>;
};

function mapOrchestrateIssueToIssue(raw: OrchestrateIssue): Issue {
  return {
    id: raw.id,
    workspaceId: raw.workspaceId,
    identifier: raw.identifier,
    title: raw.title,
    description: raw.description,
    status: raw.status as Issue["status"],
    priority: (raw.priority || "medium") as Issue["priority"],
    labels: raw.labels ?? [],
    assigneeAgentInstanceId: raw.assigneeAgentInstanceId,
    parentId: raw.parentId,
    projectId: raw.projectId,
    blockedBy: [],
    blocking: [],
    children: [],
    reviewers: [],
    approvers: [],
    createdBy: "",
    createdAt: raw.createdAt,
    updatedAt: raw.updatedAt,
  };
}

function mapCommentResponse(c: TaskCommentResponse): IssueComment {
  return {
    id: c.id,
    taskId: c.taskId,
    authorType: c.authorType as "user" | "agent",
    authorId: c.authorId,
    authorName: c.authorType === "agent" ? "Agent" : "You",
    content: c.body,
    createdAt: c.createdAt,
  };
}

function useIssueData(id: string) {
  const storeIssues = useAppStore((s) => s.orchestrate.issues.items);
  const [task, setTask] = useState<Issue | null>(null);
  const [comments, setComments] = useState<IssueComment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchComments = useCallback(async () => {
    try {
      const res = await listComments(id);
      setComments((res.comments ?? []).map(mapCommentResponse));
    } catch {
      // Comments endpoint may not be available yet; silently ignore
    }
  }, [id]);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError(null);

      // First try the store (already loaded from the issues list page)
      const fromStore = storeIssues.find((i) => i.id === id);
      if (fromStore) {
        if (!cancelled) setTask(mapOrchestrateIssueToIssue(fromStore));
      }

      // Also fetch from API for freshest data
      try {
        const res = await getIssue(id);
        if (!cancelled && res.issue) {
          setTask(mapOrchestrateIssueToIssue(res.issue));
        } else if (!cancelled && !fromStore) {
          setError("Issue not found");
        }
      } catch {
        // If API fetch fails but we have store data, keep using it
        if (!cancelled && !fromStore) {
          setError("Failed to load issue");
        }
      }

      if (!cancelled) {
        await fetchComments();
        setLoading(false);
      }
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [id, storeIssues, fetchComments]);

  return { task, comments, loading, error, fetchComments };
}

export default function IssueDetailPage({ params }: IssueDetailPageProps) {
  const { id } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const mode = searchParams.get("mode") || "simple";

  const { task, comments, loading, error, fetchComments } = useIssueData(id);

  // TODO: Wire activity and sessions when backend endpoints are available
  const activity: IssueActivityEntry[] = [];
  const sessions: TaskSession[] = [];

  const hasSession = Boolean(task?.assigneeAgentInstanceId) || sessions.length > 0;

  const setMode = (newMode: string) => {
    const url =
      newMode === "advanced"
        ? `/orchestrate/issues/${id}?mode=advanced`
        : `/orchestrate/issues/${id}`;
    router.push(url);
  };

  if (loading && !task) {
    return <IssueDetailSkeleton />;
  }

  if (error && !task) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-sm text-muted-foreground">{error}</p>
          <button
            className="mt-2 text-sm text-primary underline cursor-pointer"
            onClick={() => router.push("/orchestrate/issues")}
          >
            Back to issues
          </button>
        </div>
      </div>
    );
  }

  if (!task) return null;

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
      onCommentsChanged={fetchComments}
    />
  );
}
