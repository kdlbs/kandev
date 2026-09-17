import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { PageShell } from "@/components/page-shell";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { listTaskSessions } from "@/lib/api/domains/session-api";
import {
  getConversation,
  getConversationComments,
} from "@/lib/api/domains/orchestration-conversation-api";
import { useSessionLiveSyncSubscriptions } from "@/hooks/domains/session/use-session-live-sync";
import type { TaskSession as APISession } from "@/lib/types/http";
import type { TaskSession } from "@/components/task/simple/types";
import { OrchestratorConversationPane } from "./conversation-pane";

export function mapConversationSession(session: APISession): TaskSession {
  return {
    id: session.id,
    agentProfileId: session.agent_profile_id,
    agentName: "",
    agentRole: "agent",
    state: session.state as TaskSession["state"],
    isPrimary: Boolean(session.is_primary),
    startedAt: session.started_at,
    completedAt: session.completed_at ?? undefined,
    updatedAt: session.updated_at,
    errorMessage: session.error_message ?? undefined,
    metadata: session.metadata,
    commandCount: session.command_count,
  };
}
async function loadConversation(id: string) {
  const [task, comments, sessions] = await Promise.all([
    getConversation(id),
    getConversationComments(id),
    listTaskSessions(id),
  ]);
  return { task, comments, sessions: sessions.sessions ?? [] };
}
function ConversationContent({ taskId }: { taskId: string }) {
  const store = useAppStoreApi();
  const [data, setData] = useState<Awaited<ReturnType<typeof loadConversation>>>();
  const [error, setError] = useState<string>();
  const [revision, setRevision] = useState(0);
  const refresh = useCallback(() => setRevision((value) => value + 1), []);
  const setSessions = useAppStore((s) => s.setTaskSessionsForTask);
  const connectionStatus = useAppStore((s) => s.connection.status);
  useEffect(() => {
    let active = true;
    let pending = false;
    const load = async () => {
      if (pending || document.hidden) return;
      pending = true;
      try {
        const activityEpochsAtRequestStart = {
          ...store.getState().taskSessions.activityEpochBySession,
        };
        const result = await loadConversation(taskId);
        if (active) {
          setData(result);
          setError(undefined);
          setSessions(taskId, result.sessions, activityEpochsAtRequestStart);
        }
      } catch (e) {
        if (active) setError(String(e));
      } finally {
        pending = false;
      }
    };
    void load();
    const timer = window.setInterval(() => void load(), 3000);
    document.addEventListener("visibilitychange", load);
    return () => {
      active = false;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", load);
    };
  }, [taskId, revision, setSessions, store]);
  useSessionLiveSyncSubscriptions({
    connectionStatus,
    taskId,
    sessionIds: data?.sessions.map((s) => s.id) ?? [],
  });
  if (!data) return error ? <p role="alert">{error}</p> : null;
  const owner = data.task.orchestrator_id;
  return (
    <>
      {error && <p role="alert">{error}</p>}
      <OrchestratorConversationPane
        task={{ id: data.task.id, title: data.task.title, workspaceId: data.task.workspace_id }}
        orchestratorId={typeof owner === "string" ? owner : ""}
        comments={data.comments}
        sessions={data.sessions.map(mapConversationSession)}
        timeline={[]}
        activity={[]}
        onCommentsChanged={refresh}
      />
    </>
  );
}
export function OrchestrationConversationRoute({ taskId }: { taskId: string }) {
  const { t } = useTranslation();
  const enabled = useAppStore((s) => s.features.orchestration);
  return (
    <PageShell title={t("orchestration:orchestration")} scroll="none">
      <div className="flex flex-col h-full min-h-0">
        {enabled ? (
          <ConversationContent key={taskId} taskId={taskId} />
        ) : (
          <p>{t("common:unavailable")}</p>
        )}
      </div>
    </PageShell>
  );
}
