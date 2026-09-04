"use client";

import {
  use,
  useState,
  useEffect,
  useRef,
  useCallback,
  useMemo,
  Suspense,
  type Dispatch,
  type SetStateAction,
} from "react";
import { useRouter, useSearchParams } from "@/lib/routing/client-router";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useOfficeRefetch } from "@/hooks/use-office-refetch";
import { useLatestOnly } from "@/hooks/use-latest-only";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import {
  getTask,
  listActivityForTarget,
  listComments,
  type TaskCommentResponse,
} from "@/lib/api/domains/office-api";
import { fetchTask } from "@/lib/api/domains/kanban-api";
import {
  endAllEditorsForTask,
  getCanonicalValue,
  isFieldGuarded,
  nextTaskSequence,
  recordRefetchCandidate,
  seedInitialCanonical,
  subscribeField,
} from "@/lib/state/office-task-content-sync";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import {
  liveSessionMetadataFromStore,
  mergeLiveSessionMetadata,
} from "@/components/task/simple/chat-entries";
import { OfficeSimplePane } from "@/components/task/simple/OfficeSimplePane";
import { TaskAdvancedMode } from "./task-advanced-mode";
import { IssueDetailSkeleton } from "./task-detail-skeleton";
import { TaskBody, resolveTaskBodyMode, type TaskBodyMode } from "@/components/task/TaskBody";
import type { Task, TaskComment, TaskActivityEntry, TaskSession, TimelineEvent } from "./types";
import type { ActivityEntry } from "@/lib/state/slices/office/types";
import type { TaskSession as ApiTaskSession } from "@/lib/types/http";
import { useSessionLiveSyncSubscriptions } from "./use-session-live-sync";
import { useTranslation } from "react-i18next";
import { t } from "@/lib/i18n";
import { mapOfficeTaskToTask } from "./map-office-task";
import { captureTaskSessionActivityEpochs } from "@/lib/state/slices/session/activity-epochs";

type IssueDetailPageProps = {
  params: Promise<{ id: string }>;
};

// Sentinel for the live-session metadata key: distinguishes "no store entry"
// (keep the initial fetch) from an explicit server-side null (metadata was
// cleared and must not be resurrected).
const SESSION_METADATA_ABSENT = "\u0000absent\u0000";

function mapCommentResponse(c: TaskCommentResponse): TaskComment {
  return {
    id: c.id,
    taskId: c.taskId,
    authorType: c.authorType as "user" | "agent",
    authorId: c.authorId,
    // Agent name is resolved at render time against the office agents
    // store so it stays correct after renames. Backend doesn't send a
    // name for session-bridged comments, so leave it empty here.
    // Module-level `t`, resolved when the response is mapped: this runs in a
    // fetch, not a render. `task:you` is the same word the shared task chat
    // already uses, so it is reused rather than duplicated into `office`.
    authorName: c.authorType === "user" ? t("task:you") : "",
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

/**
 * NOT localized, deliberately. `action` is an open-ended backend activity
 * identifier (`task.status_changed`, `task.plan.revision.created`, …) with no
 * closed union on the wire, so a key map would silently fall through for any
 * action the backend adds. The verb is rendered by
 * `components/task/simple/task-activity.tsx`, which is outside this migration.
 */
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
    agentName: snapshotString(profile, "name") || session.agent_profile_id || t("task:agent"),
    agentRole: snapshotString(profile, "role") || "agent",
    state: session.state as TaskSession["state"],
    isPrimary: Boolean(session.is_primary),
    startedAt: session.started_at,
    completedAt: session.completed_at ?? undefined,
    updatedAt: session.updated_at,
    errorMessage: session.error_message ?? undefined,
    metadata: session.metadata ?? undefined,
    commandCount: session.command_count,
  };
}

type IssueDetailData = {
  activity: TaskActivityEntry[] | null;
  sessions: TaskSession[] | null;
  rawSessions: ApiTaskSession[] | null;
  comments: TaskComment[] | null;
};

type OrderedIssueDetailData = IssueDetailData & {
  activityEpochsAtRequestStart: Readonly<Record<string, number>>;
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

async function fetchOrderedIssueDetailData(
  state: Parameters<typeof captureTaskSessionActivityEpochs>[0],
  workspaceId: string,
  id: string,
): Promise<OrderedIssueDetailData> {
  const activityEpochsAtRequestStart = captureTaskSessionActivityEpochs(state, id);
  return {
    ...(await fetchIssueDetailData(workspaceId, id)),
    activityEpochsAtRequestStart,
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

  // Same stable-key trick for the live metadata (last_agent_error etc.)
  // carried by session.state_changed, so the office chat can render the
  // remediation link without a refetch. Tri-state per session: the sentinel
  // marks "no metadata update" (no store row, or a partial row without a
  // metadata field), explicit null means the server cleared metadata, and an
  // object is the live metadata.
  const sessionMetadataKey = useAppStore((s) => {
    const items = s.taskSessions?.items ?? {};
    return baseSessions
      .map((sess) =>
        JSON.stringify(liveSessionMetadataFromStore(items, sess.id) ?? SESSION_METADATA_ABSENT),
      )
      .join("\u0001");
  });
  const sessionStoreMetadata = useMemo(
    () =>
      sessionMetadataKey.split("\u0001").map((chunk) => {
        try {
          const parsed = JSON.parse(chunk);
          if (parsed === SESSION_METADATA_ABSENT) return undefined;
          return parsed && typeof parsed === "object" ? (parsed as Record<string, unknown>) : null;
        } catch {
          return null;
        }
      }),
    [sessionMetadataKey],
  );

  const connectionStatus = useAppStore((s) => s.connection.status);
  useSessionLiveSyncSubscriptions({
    connectionStatus,
    taskId: task?.id ?? null,
    sessionIds: baseSessions.map((session) => session.id),
  });

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
    // AC-52: the sequence marks when this refetch was *issued*, so a slow
    // response can still be recognized as stale against a write issued
    // after it but resolved before it.
    const sequence = nextTaskSequence(taskId);
    getTask(taskId)
      .then((res) => {
        if (res.task) {
          const mapped = mapOfficeTaskToTask(res.task);
          onTaskRefetch(mapped, res.timeline ?? []);
          recordRefetchCandidate(taskId, "title", mapped.title, mapped.updatedAt, sequence);
          recordRefetchCandidate(
            taskId,
            "description",
            mapped.description,
            mapped.updatedAt,
            sequence,
          );
        }
      })
      .catch(() => {});
    void onCommentsRefetch();
  }, [sessionStatesKey, taskId, onTaskRefetch, onCommentsRefetch]);

  return { sessionStoreStates, sessionStoreMetadata };
}

// ---------------------------------------------------------------------------
// Optimistic update helpers
// ---------------------------------------------------------------------------

// Exported (in addition to being used internally by `useIssueData`) so unit
// tests can drive the real guard-cleanup and restore behavior directly,
// rather than only through a fully-rendered page.
export function useTaskOptimisticHelpers(
  id: string,
  setTask: React.Dispatch<React.SetStateAction<Task | null>>,
  setTimeline: React.Dispatch<React.SetStateAction<TimelineEvent[]>>,
) {
  const storeApi = useAppStoreApi();

  const applyTaskPatch = useCallback(
    (patch: Partial<Task>) => {
      setTask((prev) => (prev && prev.id === id ? { ...prev, ...patch } : prev));
    },
    [id, setTask],
  );

  // AC-27/49/50/71: apply a field's canonical value to the displayed task
  // (and the Office task store entry) the instant it becomes unguarded —
  // either because a candidate was recorded while already unguarded, or
  // because the last open editor / in-flight write for that field just
  // cleared. `notifyField` fires unconditionally on every guard release
  // (office-task-content-sync.ts), so these listeners own the "if the two
  // differ" comparison against what's currently displayed/stored (AC-71):
  // without it, a no-op editor open/close on a task with no description
  // would write "" over an `undefined` description, since the canonical
  // value is always normalised to "" but the store never held one.
  useEffect(() => {
    let unmounted = false;
    const unsubTitle = subscribeField(id, "title", (value) => {
      setTask((prev) =>
        prev && prev.id === id && prev.title !== value ? { ...prev, title: value } : prev,
      );
      const current = storeApi.getState().office.tasks.items.find((t) => t.id === id);
      if (current && current.title !== value) {
        storeApi.getState().patchTaskInStore(id, { title: value });
      }
      if (unmounted && !isFieldGuarded(id, "title")) unsubTitle();
    });
    const unsubDescription = subscribeField(id, "description", (value) => {
      setTask((prev) =>
        prev && prev.id === id && (prev.description ?? "") !== value
          ? { ...prev, description: value }
          : prev,
      );
      const current = storeApi.getState().office.tasks.items.find((t) => t.id === id);
      if (current && (current.description ?? "") !== value) {
        storeApi.getState().patchTaskInStore(id, { description: value });
      }
      if (unmounted && !isFieldGuarded(id, "description")) unsubDescription();
    });
    return () => {
      // AC-68/69: ending the open-editor contribution can synchronously flush
      // a canonical value that was recorded while guarded (deferred-apply),
      // and that flush needs the listeners above still registered to reach
      // the Office task store. A write issued before unmount is deliberately
      // left pending by endAllEditorsForTask and must still be able to reach
      // the store once it resolves, so a field with a write still in flight
      // keeps its listener alive past this cleanup; each listener retires
      // itself once that field's guard finally clears.
      unmounted = true;
      endAllEditorsForTask(id);
      if (!isFieldGuarded(id, "title")) unsubTitle();
      if (!isFieldGuarded(id, "description")) unsubDescription();
    };
  }, [id, setTask, storeApi]);

  // Refetch the canonical task DTO when the backend broadcasts an update
  // (priority / project / parent / blockers / participants / assignee).
  // The optimistic patch we applied locally gets reconciled with server
  // state. Title/description are excluded from this merge — the
  // subscribeField effect above governs them so a guarded field's
  // optimistic/in-flight value survives an unrelated refetch (AC-38/39).
  //
  // A WS-driven trigger can fire again before a prior GET resolves (two
  // status changes in quick succession, ordinary network jitter). Without
  // a guard, a slower earlier response can land after a faster later one
  // and overwrite fresher task state with stale data. useLatestOnly
  // discards a response once a newer refetchTask call has started,
  // regardless of arrival order.
  const { begin, isCurrent } = useLatestOnly();
  const refetchTask = useCallback(async () => {
    const token = begin();
    // AC-52: the sequence marks when this refetch was *issued*.
    const sequence = nextTaskSequence(id);
    try {
      const [res, genericTask] = await Promise.all([getTask(id), fetchTask(id).catch(() => null)]);
      if (!isCurrent(token)) return;
      if (res.task) {
        const mapped = mapOfficeTaskToTask(res.task, genericTask ?? undefined);
        setTask((prev) =>
          prev ? { ...mapped, title: prev.title, description: prev.description } : mapped,
        );
        if (res.timeline) setTimeline(res.timeline);
        recordRefetchCandidate(id, "title", mapped.title, mapped.updatedAt, sequence);
        recordRefetchCandidate(id, "description", mapped.description, mapped.updatedAt, sequence);
      }
    } catch {
      /* swallow — next user action will retry */
    }
  }, [id, setTask, setTimeline, begin, isCurrent]);
  useOfficeRefetch(`task:${id}`, () => {
    void refetchTask();
  });

  // `restoreTask` backs `TaskOptimisticContextValue.restore`, the failure
  // rollback for the generic picker mutation hook (status/priority/project/
  // parent/labels/blockedBy/assignee). Title/description are excluded from
  // the restored snapshot and always kept at their live value — those two
  // fields are governed exclusively by the guard above, and this generic
  // rollback never patches them optimistically in the first place (AC-61:
  // only two writers may touch a guarded field, and this isn't one of them).
  const restoreTask = useCallback(
    (snapshot: Task) => {
      setTask((prev) =>
        prev && prev.id === id
          ? { ...snapshot, title: prev.title, description: prev.description }
          : prev,
      );
    },
    [id, setTask],
  );

  return { applyTaskPatch, restoreTask };
}

function useTaskDetailRefetch(
  setTask: Dispatch<SetStateAction<Task | null>>,
  setTimeline: Dispatch<SetStateAction<TimelineEvent[]>>,
) {
  return useCallback(
    (updated: Task, updatedTimeline: TimelineEvent[]) => {
      setTask((previous) =>
        previous
          ? {
              ...updated,
              // Office's live refetch endpoint does not carry the generic
              // status summary or repository rows. Preserve the supplement
              // loaded by the initial detail request until the next full load.
              statusSummary: updated.statusSummary ?? previous.statusSummary,
              repositories: updated.repositories ?? previous.repositories,
              // AC-27/28/38/39/49/50: title/description must not be blindly
              // overwritten by this whole-task refetch merge — they're
              // governed exclusively by the subscribeField-driven
              // deferred-apply effect in useTaskOptimisticHelpers, which
              // respects the open-editor/in-flight-write guard. Every other
              // field applies immediately.
              title: previous.title,
              description: previous.description,
            }
          : updated,
      );
      setTimeline(updatedTimeline);
    },
    [setTask, setTimeline],
  );
}

// ---------------------------------------------------------------------------
// Primary data hook
// ---------------------------------------------------------------------------

// Snapshot storeIssues in a ref so the load effect can seed `task` from the
// store without re-running on every store update. Re-running the GET
// on store changes would race with in-flight optimistic mutations (the WS-
// driven refetch in useTaskOptimisticHelpers handles canonical refresh after
// a property mutation commits).
// `errorKey` remains a catalog key so locale changes do not re-issue the load.

// AC-59: seeds each field's canonical value from a loaded task.
function seedCanonicalTitleAndDescription(taskId: string, task: Task): void {
  seedInitialCanonical(taskId, "title", task.title, task.updatedAt);
  seedInitialCanonical(taskId, "description", task.description ?? "", task.updatedAt);
}

// P1 review fix: the store-cache fast path can observe an unpersisted
// optimistic value (a title/description write still in flight when the user
// last navigated away). Seeding from it unconditionally would let that value
// win an equal-timestamp tiebreak against the already-recorded canonical
// value on a revisit before the authoritative GET resolves. Only seed a
// field from the cache when nothing is recorded for it yet; once a canonical
// value exists, only the authoritative GET response may supersede it.
function seedCanonicalTitleAndDescriptionIfAbsent(taskId: string, task: Task): void {
  if (getCanonicalValue(taskId, "title") === undefined) {
    seedInitialCanonical(taskId, "title", task.title, task.updatedAt);
  }
  if (getCanonicalValue(taskId, "description") === undefined) {
    seedInitialCanonical(taskId, "description", task.description ?? "", task.updatedAt);
  }
}

// AC-59/AC-72: if the authoritative GET fails but the page is already
// rendering this task interactively from the store cache, seed canonical
// from it — otherwise a commit issued before the load ever succeeds (or if
// it never does) would have no restore target on failure (AC-9/AC-64). In
// practice this is now redundant with the synchronous seed in `load()`
// below, but is kept as a defensive fallback for the store-miss case.
function handleInitialLoadFailure(
  taskId: string,
  fromStoreTask: Task | null,
  setErrorKey: (key: string | null) => void,
): void {
  if (fromStoreTask) {
    seedCanonicalTitleAndDescription(taskId, fromStoreTask);
  } else {
    setErrorKey("office:failedToLoadTask");
  }
}

function useIssueDetailState(id: string) {
  const store = useAppStoreApi();
  const [comments, setComments] = useState<TaskComment[]>([]);
  const [timeline, setTimeline] = useState<TimelineEvent[]>([]);
  const [activity, setActivity] = useState<TaskActivityEntry[]>([]);
  const [baseSessions, setBaseSessions] = useState<TaskSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [errorKey, setErrorKey] = useState<string | null>(null);
  const fetchComments = useCallback(async () => {
    setComments(await fetchIssueComments(id));
  }, [id]);
  const applyDetail = useCallback(
    (detail: OrderedIssueDetailData) => {
      if (detail.activity) setActivity(detail.activity);
      if (detail.sessions) setBaseSessions(detail.sessions);
      if (detail.rawSessions) {
        store
          .getState()
          .setTaskSessionsForTask(id, detail.rawSessions, detail.activityEpochsAtRequestStart);
      }
      if (detail.comments) setComments(detail.comments);
    },
    [id, store],
  );
  return {
    comments,
    timeline,
    setTimeline,
    activity,
    baseSessions,
    loading,
    setLoading,
    errorKey,
    setErrorKey,
    fetchComments,
    applyDetail,
    getStoreState: store.getState,
  };
}

export function useIssueData(id: string) {
  const storeIssues = useAppStore((s) => s.office.tasks.items);
  const storeIssuesRef = useRef(storeIssues);
  useEffect(() => {
    storeIssuesRef.current = storeIssues;
  }, [storeIssues]);

  const [task, setTask] = useState<Task | null>(null);
  const {
    comments,
    timeline,
    setTimeline,
    activity,
    baseSessions,
    loading,
    setLoading,
    errorKey,
    setErrorKey,
    fetchComments,
    applyDetail,
    getStoreState,
  } = useIssueDetailState(id);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      setLoading(true);
      setErrorKey(null);
      const fromStore = storeIssuesRef.current.find((i) => i.id === id);
      const fromStoreTask = fromStore ? mapOfficeTaskToTask(fromStore) : null;
      if (fromStoreTask && !cancelled) {
        setTask(fromStoreTask);
        // AC-59/AC-9/AC-64: the page is interactive from here, so a commit
        // issued before the GET below resolves needs a canonical value to
        // restore to on failure. Seeded only if absent (see the function's
        // doc comment): the later GET-success seed below is what's allowed
        // to supersede an existing canonical value.
        seedCanonicalTitleAndDescriptionIfAbsent(id, fromStoreTask);
      }

      try {
        const [res, genericTask] = await Promise.all([
          getTask(id),
          fetchTask(id).catch(() => null),
        ]);
        if (cancelled) return;
        if (!res.task) {
          if (!fromStore) setErrorKey("office:taskNotFound");
        } else {
          const freshTask = mapOfficeTaskToTask(res.task, genericTask ?? undefined);
          // AC-27/28/38/39: this GET can resolve after a title/description
          // commit has already landed (the page renders interactively from
          // `fromStore` before this request settles), so it must not blindly
          // overwrite those two fields — same merge pattern as `refetchTask`
          // and `onTaskRefetch` below.
          setTask((prev) =>
            prev && prev.id === id
              ? { ...freshTask, title: prev.title, description: prev.description }
              : freshTask,
          );
          if (res.timeline) setTimeline(res.timeline);
          // AC-59: the initial load seeds each field's canonical value.
          seedCanonicalTitleAndDescription(id, freshTask);
          const detail = await fetchOrderedIssueDetailData(
            getStoreState(),
            freshTask.workspaceId,
            id,
          );
          if (!cancelled) applyDetail(detail);
        }
      } catch {
        if (!cancelled) handleInitialLoadFailure(id, fromStoreTask, setErrorKey);
      }

      if (!cancelled) setLoading(false);
    }

    void load();
    return () => {
      cancelled = true;
    };
  }, [id, applyDetail, getStoreState]);

  const onTaskRefetch = useTaskDetailRefetch(setTask, setTimeline);

  const { sessionStoreStates, sessionStoreMetadata } = useSessionLiveSync({
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
        // Live session.state_changed metadata wins over the initial fetch;
        // explicit null (server cleared metadata) is preserved.
        metadata: mergeLiveSessionMetadata(s.metadata, sessionStoreMetadata[i]),
      })),
    [baseSessions, sessionStoreStates, sessionStoreMetadata],
  );

  useOfficeRefetch("comments", fetchComments);

  const { applyTaskPatch, restoreTask } = useTaskOptimisticHelpers(id, setTask, setTimeline);

  return {
    task,
    comments,
    timeline,
    activity,
    sessions,
    loading,
    errorKey,
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
  const { t } = useTranslation();
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
    errorKey,
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

  if (errorKey && !task) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="text-center">
          <p className="text-sm text-muted-foreground">{t(errorKey)}</p>
          <button
            className="mt-2 text-sm text-primary underline cursor-pointer"
            onClick={() => router.push("/office/tasks")}
          >
            {t("office:backToTasks")}
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
