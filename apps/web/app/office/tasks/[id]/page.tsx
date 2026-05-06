"use client";

import { use, useState, useEffect, useRef, useCallback, useMemo, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useAppStore } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import {
  getTask,
  listActivityForTarget,
  listComments,
  type TaskCommentResponse,
  type TaskDecisionDTO,
} from "@/lib/api/domains/office-api";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import { getWebSocketClient } from "@/lib/ws/connection";
import { OfficeSimplePane } from "@/components/task/simple/OfficeSimplePane";
import { TaskAdvancedMode } from "./task-advanced-mode";
import { IssueDetailSkeleton } from "./task-detail-skeleton";
import { TaskBody, resolveTaskBodyMode, type TaskBodyMode } from "@/components/task/TaskBody";
import type {
  Task,
  TaskComment,
  TaskActivityEntry,
  TaskDecision,
  TaskSession,
  TimelineEvent,
} from "./types";
import type { ActivityEntry, OfficeTask } from "@/lib/state/slices/office/types";
import type { TaskSession as ApiTaskSession } from "@/lib/types/http";

type IssueDetailPageProps = {
  params: Promise<{ id: string }>;
};

function mapDecisionDTO(d: TaskDecisionDTO): TaskDecision {
  return {
    id: d.id,
    taskId: d.task_id,
    deciderType: d.decider_type,
    deciderId: d.decider_id,
    deciderName: d.decider_name ?? "",
    role: d.role,
    decision: d.decision,
    comment: d.comment ?? "",
    createdAt: d.created_at,
  };
}

function mapOfficeTaskToTask(raw: OfficeTask): Task {
  // The server DTO includes reviewers/approvers/decisions even though the
  // strongly-typed OfficeTask only declares the cross-cutting fields. We
  // read those extra props off the raw object.
  const extra = raw as OfficeTask & {
    reviewers?: string[];
    approvers?: string[];
    decisions?: TaskDecisionDTO[];
    blockedBy?: string[];
  };
  return {
    id: raw.id,
    workspaceId: raw.workspaceId,
    identifier: raw.identifier,
    title: raw.title,
    description: raw.description,
    status: raw.status as Task["status"],
    priority: (raw.priority || "medium") as Task["priority"],
    labels: (raw.labels ?? []).map((l) =>
      typeof l === "string" ? { name: l, color: "#6b7280" } : l,
    ),
    assigneeAgentProfileId: raw.assigneeAgentProfileId,
    parentId: raw.parentId,
    projectId: raw.projectId,
    blockedBy: extra.blockedBy ?? [],
    blocking: [],
    children: (raw.children ?? []).map((child) => ({
      id: child.id,
      identifier: child.identifier,
      title: child.title,
      status: child.status as Task["status"],
      blockedBy: child.blockedBy ?? [],
      createdAt: child.createdAt,
    })),
    reviewers: extra.reviewers ?? [],
    approvers: extra.approvers ?? [],
    decisions: (extra.decisions ?? []).map(mapDecisionDTO),
    createdBy: "",
    createdAt: raw.createdAt,
    updatedAt: raw.updatedAt,
    executionPolicy: raw.executionPolicy,
    executionState: raw.executionState,
  };
}

function mapCommentResponse(c: TaskCommentResponse): TaskComment {
  return {
    id: c.id,
    taskId: c.taskId,
    authorType: c.authorType as "user" | "agent",
    authorId: c.authorId,
    // Agent name is resolved at render time against the office agents
    // store so it stays correct after renames. Backend doesn't send a
    // name for session-bridged comments, so leave it empty here.
    authorName: c.authorType === "user" ? "You" : "",
    content: c.body,
    source: c.source,
    createdAt: c.createdAt,
    runId: c.runId,
    runStatus: c.runStatus,
    runError: c.runError,
  };
}

function entryField(entry: ActivityEntry, camelKey: keyof ActivityEntry, snakeKey: string) {
  const raw = entry as ActivityEntry & Record<string, unknown>;
  return raw[camelKey] ?? raw[snakeKey];
}

function activityActionVerb(action: string) {
  return action
    .replace(/^task\./, "")
    .replaceAll("_", " ")
    .replaceAll(".", " ");
}

function mapActivityEntry(entry: ActivityEntry): TaskActivityEntry {
  const actorType = String(entryField(entry, "actorType", "actor_type") || "system");
  const action = String(entry.action || "");
  return {
    id: entry.id,
    actorType: actorType as TaskActivityEntry["actorType"],
    actorId: String(entryField(entry, "actorId", "actor_id") || ""),
    actionVerb: activityActionVerb(action),
    targetName: String(entryField(entry, "targetType", "target_type") || "task"),
    createdAt: String(entryField(entry, "createdAt", "created_at") || ""),
  };
}

function snapshotString(snapshot: Record<string, unknown> | null | undefined, key: string) {
  const value = snapshot?.[key];
  return typeof value === "string" ? value : "";
}

function mapTaskSession(session: ApiTaskSession): TaskSession {
  const profile = session.agent_profile_snapshot;
  return {
    id: session.id,
    agentProfileId: session.agent_profile_id,
    agentName: snapshotString(profile, "name") || session.agent_profile_id || "Agent",
    agentRole: snapshotString(profile, "role") || "agent",
    state: session.state as TaskSession["state"],
    isPrimary: Boolean(session.is_primary),
    startedAt: session.started_at,
    completedAt: session.completed_at ?? undefined,
    updatedAt: session.updated_at,
    errorMessage: session.error_message ?? undefined,
    commandCount: session.command_count,
  };
}

type IssueDetailData = {
  activity: TaskActivityEntry[] | null;
  sessions: TaskSession[] | null;
  rawSessions: ApiTaskSession[] | null;
  comments: TaskComment[] | null;
};

// ---------------------------------------------------------------------------
// Task fetchers — pure async functions, no React state
// ---------------------------------------------------------------------------

async function fetchIssueComments(id: string): Promise<TaskComment[]> {
  try {
    const res = await listComments(id);
    return (res.comments ?? []).map(mapCommentResponse);
  } catch {
    return [];
  }
}

async function fetchIssueDetailData(workspaceId: string, id: string): Promise<IssueDetailData> {
  const [activityResult, sessionsResult, commentsResult] = await Promise.allSettled([
    listActivityForTarget(workspaceId, id),
    listTaskSessions(id),
    fetchIssueComments(id),
  ]);
  const rawSessions =
    sessionsResult.status === "fulfilled" ? (sessionsResult.value.sessions ?? []) : null;
  return {
    activity:
      activityResult.status === "fulfilled"
        ? (activityResult.value.activity ?? []).map(mapActivityEntry)
        : null,
    sessions: rawSessions ? rawSessions.map(mapTaskSession) : null,
    rawSessions,
    comments: commentsResult.status === "fulfilled" ? commentsResult.value : null,
  };
}

// ---------------------------------------------------------------------------
// Live sync — WS subscriptions + re-fetch on session state changes
// ---------------------------------------------------------------------------

type LiveSyncParams = {
  task: Task | null;
  baseSessions: TaskSession[];
  onTaskRefetch: (task: Task, timeline: TimelineEvent[]) => void;
  onCommentsRefetch: () => Promise<void>;
};

function useSessionLiveSync({
  task,
  baseSessions,
  onTaskRefetch,
  onCommentsRefetch,
}: LiveSyncParams) {
  // Join to a stable string to avoid infinite re-renders from array reference changes.
  const sessionStatesKey = useAppStore((s) => {
    const items = s.taskSessions?.items ?? {};
    return baseSessions.map((sess) => items[sess.id]?.state ?? sess.state).join(",");
  });
  const sessionStoreStates = useMemo(
    () => (sessionStatesKey ? sessionStatesKey.split(",") : []),
    [sessionStatesKey],
  );

  const connectionStatus = useAppStore((s) => s.connection.status);
  useEffect(() => {
    if (connectionStatus !== "connected" || baseSessions.length === 0 || !task) return;
    const client = getWebSocketClient();
    if (!client) return;
    const unsubs = baseSessions.map((s) => client.subscribeSession(s.id));
    return () => unsubs.forEach((u) => u());
  }, [connectionStatus, baseSessions, task]);

  // Refetch the task + comments whenever session state actually changes.
  // The dep is intentionally the joined session-state key (and the
  // taskId), NOT the `task` object — calling onTaskRefetch inside this
  // effect triggers setTask, which produces a new `task` reference, and
  // including that in deps would self-perpetuate the effect into an
  // infinite render loop (the comment on sessionStatesKey above flagged
  // this concern but the deps still kept `task`).
  const taskId = task?.id;
  useEffect(() => {
    if (!taskId || !sessionStatesKey) return;
    getTask(taskId)
      .then((res) => {
        if (res.task) onTaskRefetch(mapOfficeTaskToTask(res.task), res.timeline ?? []);
      })
      .catch(() => {});
    void onCommentsRefetch();
  }, [sessionStatesKey, taskId, onTaskRefetch, onCommentsRefetch]);

  return sessionStoreStates;
}

// ---------------------------------------------------------------------------
// Optimistic update helpers
// ---------------------------------------------------------------------------

function useTaskOptimisticHelpers(
  id: string,
  setTask: React.Dispatch<React.SetStateAction<Task | null>>,
  setTimeline: React.Dispatch<React.SetStateAction<TimelineEvent[]>>,
) {
  // Refetch the canonical task DTO when the backend broadcasts an update
  // (priority / project / parent / blockers / participants / assignee).
  // The optimistic patch we applied locally gets reconciled with server
  // state.
  const refetchTask = useCallback(async () => {
    try {
      const res = await getTask(id);
      if (res.task) {
        setTask(mapOfficeTaskToTask(res.task));
        if (res.timeline) setTimeline(res.timeline);
      }
    } catch {
      /* swallow — next user action will retry */
    }
  }, [id, setTask, setTimeline]);
  useOfficeRefetch(`task:${id}`, () => {
    void refetchTask();
  });

  const applyTaskPatch = useCallback(
    (patch: Partial<Task>) => {
      setTask((prev) => (prev ? { ...prev, ...patch } : prev));
    },
    [setTask],
  );

  const restoreTask = useCallback(
    (snapshot: Task) => {
      setTask(snapshot);
    },
    [setTask],
  );

  return { applyTaskPatch, restoreTask };
}

// ---------------------------------------------------------------------------
// Primary data hook
// ---------------------------------------------------------------------------

function useIssueData(id: string) {
  const storeIssues = useAppStore((s) => s.office.tasks.items);
  const setTaskSessionsForTask = useAppStore((s) => s.setTaskSessionsForTask);
  // Snapshot storeIssues in a ref so the load effect can seed `task` from
  // the store without re-running on every store update. Re-running the GET
  // on store changes would race with in-flight optimistic mutations (the
  // WS-driven refetch in useTaskOptimisticHelpers handles canonical
  // refresh after a property mutation commits).
  const storeIssuesRef = useRef(storeIssues);
  useEffect(() => {
    storeIssuesRef.current = storeIssues;
  }, [storeIssues]);

  const [task, setTask] = useState<Task | null>(null);
  const [comments, setComments] = useState<TaskComment[]>([]);
  const [timeline, setTimeline] = useState<TimelineEvent[]>([]);
  const [activity, setActivity] = useState<TaskActivityEntry[]>([]);
  const [baseSessions, setBaseSessions] = useState<TaskSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const applyDetail = useCallback(
    (detail: IssueDetailData) => {
      if (detail.activity) setActivity(detail.activity);
      if (detail.sessions) setBaseSessions(detail.sessions);
      if (detail.rawSessions) setTaskSessionsForTask(id, detail.rawSessions);
      if (detail.comments) setComments(detail.comments);
    },
    [id, setTaskSessionsForTask],
  );

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setError(null);
      const fromStore = storeIssuesRef.current.find((i) => i.id === id);
      if (fromStore && !cancelled) setTask(mapOfficeTaskToTask(fromStore));

      try {
        const res = await getTask(id);
        if (cancelled) return;
        if (!res.task) {
          if (!fromStore) setError("Task not found");
        } else {
          const freshTask = mapOfficeTaskToTask(res.task);
          setTask(freshTask);
          if (res.timeline) setTimeline(res.timeline);
          const detail = await fetchIssueDetailData(freshTask.workspaceId, id);
          if (!cancelled) applyDetail(detail);
        }
      } catch {
        if (!cancelled && !fromStore) setError("Failed to load task");
      }

      if (!cancelled) setLoading(false);
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [id, applyDetail]);

  const fetchComments = useCallback(async () => {
    const result = await fetchIssueComments(id);
    setComments(result);
  }, [id]);

  const onTaskRefetch = useCallback((updated: Task, updatedTimeline: TimelineEvent[]) => {
    setTask(updated);
    setTimeline(updatedTimeline);
  }, []);

  const sessionStoreStates = useSessionLiveSync({
    task,
    baseSessions,
    onTaskRefetch,
    onCommentsRefetch: fetchComments,
  });
  const sessions = useMemo(
    () =>
      baseSessions.map((s, i) => ({
        ...s,
        state: (sessionStoreStates[i] ?? s.state) as TaskSession["state"],
      })),
    [baseSessions, sessionStoreStates],
  );

  // Refetch comments when a new comment is created via office WS event
  useOfficeRefetch("comments", fetchComments);

  const { applyTaskPatch, restoreTask } = useTaskOptimisticHelpers(id, setTask, setTimeline);

  return {
    task,
    comments,
    timeline,
    activity,
    sessions,
    loading,
    error,
    fetchComments,
    applyTaskPatch,
    restoreTask,
  };
}

export default function IssueDetailPage({ params }: IssueDetailPageProps) {
  return (
    <Suspense fallback={<IssueDetailSkeleton />}>
      <IssueDetailContent params={params} />
    </Suspense>
  );
}

function IssueDetailContent({ params }: IssueDetailPageProps) {
  const { id } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  // Office shell defaults to simple. Both `?advanced` (Phase 7) and the
  // legacy `?mode=advanced` flip to advanced.
  const mode: TaskBodyMode = resolveTaskBodyMode(
    {
      simple: searchParams.has("simple") ? "" : undefined,
      advanced: searchParams.has("advanced") ? "" : undefined,
      mode: searchParams.get("mode") ?? undefined,
    },
    "simple",
  );

  const {
    task,
    comments,
    timeline,
    activity,
    sessions,
    loading,
    error,
    fetchComments,
    applyTaskPatch,
    restoreTask,
  } = useIssueData(id);

  const hasSession = Boolean(task?.assigneeAgentProfileId) || sessions.length > 0;

  const setMode = (newMode: string) => {
    const url =
      newMode === "advanced" ? `/office/tasks/${id}?mode=advanced` : `/office/tasks/${id}`;
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
            onClick={() => router.push("/office/tasks")}
          >
            Back to tasks
          </button>
        </div>
      </div>
    );
  }

  if (!task) return null;

  const optimisticContext = {
    task,
    applyPatch: applyTaskPatch,
    restore: restoreTask,
  };

  const advancedSlot = hasSession ? (
    <TaskAdvancedMode task={task} onToggleSimple={() => setMode("simple")} />
  ) : (
    <OfficeSimplePane
      task={task}
      comments={comments}
      timeline={timeline}
      activity={activity}
      sessions={sessions}
      onCommentsChanged={fetchComments}
    />
  );

  const simpleSlot = (
    <OfficeSimplePane
      task={task}
      comments={comments}
      timeline={timeline}
      activity={activity}
      sessions={sessions}
      onToggleAdvanced={hasSession ? () => setMode("advanced") : undefined}
      onCommentsChanged={fetchComments}
    />
  );

  return (
    <TaskOptimisticContextProvider value={optimisticContext}>
      <TaskBody mode={mode} simpleSlot={simpleSlot} advancedSlot={advancedSlot} />
    </TaskOptimisticContextProvider>
  );
}
