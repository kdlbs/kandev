import { type Page } from "@playwright/test";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { SessionPage } from "../../pages/session-page";

type GatewayFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

function parseFrame(raw: string): GatewayFrame | null {
  try {
    const value = JSON.parse(raw) as unknown;
    return typeof value === "object" && value !== null ? (value as GatewayFrame) : null;
  } catch {
    return null;
  }
}

export async function routeConversationForkResponse(page: Page) {
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }
      const forwarded: string[] = [];
      for (const part of message.split("\n")) {
        const trimmed = part.trim();
        if (!trimmed) continue;
        const frame = parseFrame(trimmed);
        if (
          frame?.type === "request" &&
          frame.action === "session.fork" &&
          typeof frame.id === "string"
        ) {
          socket.send(
            JSON.stringify({
              id: frame.id,
              type: "response",
              action: "session.fork",
              payload: {
                task_id: frame.payload && (frame.payload as { task_id?: string }).task_id,
                session_id: "e2e-fork-session",
                state: "CREATED",
              },
              timestamp: new Date().toISOString(),
            }),
          );
        } else {
          forwarded.push(part);
        }
      }
      if (forwarded.length > 0) server.send(forwarded.join("\n"));
    });
    server.onMessage((message) => socket.send(message));
  });
}

export async function openCompletedNativeForkTask(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
) {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  const sessionId = task.session_id ?? task.primary_session_id;
  if (!sessionId) throw new Error("The conversation fork task has no session");

  await routeConversationForkResponse(page);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  await markLatestTurnAsNative(page, task.id, sessionId);
  return { taskId: task.id, sessionId, session };
}

async function markLatestTurnAsNative(page: Page, taskId: string, sessionId: string) {
  await page.evaluate(
    ({ taskId, sessionId }) => {
      type E2EState = {
        features: Record<string, unknown>;
        messages: { bySession: Record<string, Array<Record<string, unknown>>> };
        turns: { bySession: Record<string, Array<Record<string, unknown>>> };
        addTurn: (turn: Record<string, unknown>) => void;
        setFeatures: (features: Record<string, unknown>) => void;
      };
      function getState(): E2EState {
        const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => E2EState } })
          .__KANDEV_E2E_STORE__;
        if (!store) throw new Error("E2E store bridge is unavailable");
        return store.getState();
      }
      function getAssistantMessage(state: E2EState): Record<string, unknown> {
        const message = [...(state.messages.bySession[sessionId] ?? [])]
          .reverse()
          .find(
            (candidate) =>
              candidate.author_type === "agent" && typeof candidate.turn_id === "string",
          );
        if (!message || typeof message.turn_id !== "string") {
          throw new Error("The completed session has no assistant message with a turn ID");
        }
        return message;
      }
      function markTurn(state: E2EState, message: Record<string, unknown>) {
        const turnId = message.turn_id as string;
        const storedTurn = (state.turns.bySession[sessionId] ?? []).find(
          (candidate) => candidate.id === turnId,
        );
        const timestamp = new Date().toISOString();
        state.addTurn({
          id: turnId,
          task_id: taskId,
          session_id: sessionId,
          started_at: storedTurn?.started_at ?? message.created_at ?? timestamp,
          completed_at: storedTurn?.completed_at ?? timestamp,
          created_at: storedTurn?.created_at ?? message.created_at ?? timestamp,
          updated_at: new Date(Date.now() + 60_000).toISOString(),
          metadata: {
            ...((storedTurn?.metadata as Record<string, unknown> | undefined) ?? {}),
            agent_type: "codex-app-server",
          },
        });
        state.setFeatures({ ...state.features, codexAppServer: true });
      }
      const state = getState();
      markTurn(state, getAssistantMessage(state));
    },
    { taskId, sessionId },
  );
}
