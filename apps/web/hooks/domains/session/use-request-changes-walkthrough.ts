import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useAppStoreApi } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { listPrompts } from "@/lib/api";
import { queueMessage } from "@/lib/api/domains/queue-api";
import { checkWalkthroughPackage } from "./walkthrough-package-availability";
import { getWebSocketClient } from "@/lib/ws/connection";
import {
  buildChangesWalkthroughPrompt,
  CHANGES_WALKTHROUGH_PROMPT_NAME,
} from "@/lib/walkthrough-request";
import type { Message } from "@/lib/types/http";
import type { AppState } from "@/lib/state/store";
import { deriveSessionInputMode } from "./session-input-mode";
import { getBackendConfig } from "@/lib/config";
import { generateUUID } from "@/lib/utils";

const TERMINAL_SESSION_STATES = new Set(["FAILED", "CANCELLED", "COMPLETED"]);

type UseRequestChangesWalkthroughParams = {
  taskId: string | null | undefined;
  sessionId: string | null | undefined;
  ready?: boolean;
};

function planModePayload(enabled: boolean): { plan_mode?: true } {
  return enabled ? { plan_mode: true } : {};
}

async function loadChangesWalkthroughPromptTemplate(): Promise<string> {
  const { prompts } = await listPrompts({ cache: "no-store" });
  const prompt = prompts.find((p) => p.name === CHANGES_WALKTHROUGH_PROMPT_NAME);
  const content = prompt?.content?.trim();
  if (!content) {
    throw new Error(`${CHANGES_WALKTHROUGH_PROMPT_NAME} prompt is not available`);
  }
  return content;
}

async function queueWalkthroughRequest(params: {
  taskId: string;
  sessionId: string;
  sessionIncarnationId: string | undefined;
  content: string;
  planModeEnabled: boolean;
}) {
  if (!params.sessionIncarnationId) throw new Error("Session is not available for input");
  await queueMessage({
    builtin_intent: "walkthrough",
    session_id: params.sessionId,
    session_incarnation_id: params.sessionIncarnationId,
    task_id: params.taskId,
    content: params.content,
    ...planModePayload(params.planModeEnabled),
  });
}

async function sendWalkthroughRequest(params: {
  taskId: string;
  sessionId: string;
  content: string;
  planModeEnabled: boolean;
  state: AppState;
  current: () => boolean;
}) {
  const client = getWebSocketClient();
  if (!client) throw new Error("WebSocket client unavailable");
  const created = await client.request<Message | undefined>(
    "message.add",
    {
      task_id: params.taskId,
      builtin_intent: "walkthrough",
      session_id: params.sessionId,
      client_message_id: generateUUID(),
      content: params.content,
      ...planModePayload(params.planModeEnabled),
    },
    10000,
  );
  if (params.current() && created?.id && created.session_id) params.state.addMessage(created);
}

function requestAuthority(state: AppState) {
  return JSON.stringify([
    state.auth.mode,
    state.auth.authenticated,
    state.auth.user?.id,
    getBackendConfig().apiBaseUrl,
  ]);
}
function inputError(ready: boolean, session: AppState["taskSessions"]["items"][string] | null) {
  if (!ready) return "review:changesStillLoading";
  if (deriveSessionInputMode(session) !== "unavailable") return null;
  return session && TERMINAL_SESSION_STATES.has(session.state)
    ? "task:sessionEndedCreateNew"
    : "task:sessionUnavailableForInput";
}
function sameInputSession(
  latest: AppState["taskSessions"]["items"][string] | undefined,
  taskId: string,
  incarnation: string | undefined,
) {
  return (
    latest?.task_id === taskId &&
    latest.queue_incarnation_id === incarnation &&
    deriveSessionInputMode(latest) !== "unavailable"
  );
}

export function useRequestChangesWalkthrough({
  taskId,
  sessionId,
  ready = true,
}: UseRequestChangesWalkthroughParams) {
  const storeApi = useAppStoreApi();
  const { toast } = useToast();
  const { t } = useTranslation("review");
  const scope = `${taskId}:${sessionId}`;
  const owner = useRef({ scope, generation: 0, mounted: true });
  if (owner.current.scope !== scope)
    owner.current = { scope, generation: owner.current.generation + 1, mounted: true };
  useEffect(() => {
    owner.current.mounted = true;
    return () => {
      owner.current.mounted = false;
      owner.current.generation += 1;
    };
  }, []);

  return useCallback(async () => {
    if (!taskId || !sessionId) return;
    const authority = requestAuthority(storeApi.getState());
    const generation = owner.current.generation;
    const current = () =>
      requestAuthority(storeApi.getState()) === authority &&
      owner.current.mounted &&
      owner.current.scope === scope &&
      owner.current.generation === generation;
    const state = storeApi.getState();
    const activeSession = state.taskSessions.items[sessionId] ?? null;
    const planModeEnabled = Boolean(state.chatInput.planModeBySessionId[sessionId]);
    const errorKey = inputError(ready, activeSession);
    if (errorKey) {
      toast({ title: t(errorKey), variant: "error" });
      return;
    }
    try {
      if (!(await checkWalkthroughPackage(taskId, sessionId, current, toast, t))) return;
      const template = await loadChangesWalkthroughPromptTemplate();
      if (!current()) return;
      const latest = storeApi.getState().taskSessions.items[sessionId];
      if (!sameInputSession(latest, taskId, activeSession?.queue_incarnation_id)) return;
      const content = buildChangesWalkthroughPrompt(template);
      if (deriveSessionInputMode(latest) === "queue") {
        await queueWalkthroughRequest({
          taskId,
          sessionId,
          sessionIncarnationId: latest?.queue_incarnation_id,
          content,
          planModeEnabled,
        });
        if (current()) toast({ title: t("review:walkthroughRequestQueued"), variant: "success" });
        return;
      }

      await sendWalkthroughRequest({ taskId, sessionId, content, planModeEnabled, state, current });
      if (current()) toast({ title: t("review:walkthroughRequestSent"), variant: "success" });
    } catch (error) {
      if (!current()) return;
      console.error("Failed to request walkthrough:", error);
      toast({ title: t("review:failedToRequestWalkthrough"), variant: "error" });
    }
  }, [ready, sessionId, storeApi, t, taskId, toast, scope]);
}
