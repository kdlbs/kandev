import type { Page, WebSocketRoute } from "@playwright/test";

import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { waitForAgentMessage, waitForSessionState } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

type WireFrame = {
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

function framePayload(frame: WireFrame): Record<string, unknown> | undefined {
  return typeof frame.payload === "object" && frame.payload !== null
    ? (frame.payload as Record<string, unknown>)
    : undefined;
}

function readFrame(frameText: string): WireFrame | undefined {
  try {
    return JSON.parse(frameText) as WireFrame;
  } catch {
    return undefined;
  }
}

function isConfigurationCompletion(action: unknown, payload: Record<string, unknown>): boolean {
  if (action === "session.turn.completed") return !!payload.configuration_result;
  if (action !== "session.conversation.changed" || !Array.isArray(payload.operations)) {
    return false;
  }
  return payload.operations.some((value) => {
    if (!value || typeof value !== "object") return false;
    const operation = value as {
      entity?: unknown;
      configuration_result?: unknown;
      turn?: { completed_at?: unknown };
    };
    return (
      operation.entity === "turn" &&
      Boolean(operation.turn?.completed_at) &&
      !!operation.configuration_result
    );
  });
}

export type ConfigurationCompletionHold = {
  arm: (sessionId: string) => void;
  pendingCount: () => number;
  release: () => void;
};

/** Holds only the live receipt-bearing completion until the caller is ready to observe it. */
export async function holdConfigurationCompletions(
  page: Page,
): Promise<ConfigurationCompletionHold> {
  let sessionId: string | undefined;
  const held: string[] = [];
  let deliver: ((frames: readonly string[]) => void) | undefined;

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server: WebSocketRoute = socket.connectToServer();
    deliver = (frames) => {
      if (frames.length > 0) socket.send(frames.join("\n"));
    };
    socket.onMessage((message) => server.send(message));
    server.onMessage((message) => {
      if (typeof message !== "string" || !sessionId) {
        socket.send(message);
        return;
      }
      const passthrough: string[] = [];
      for (const part of message.split("\n")) {
        const value = part.trim();
        if (!value) continue;
        const frame = readFrame(value);
        const payload = frame && framePayload(frame);
        if (frame?.type !== "notification" || payload?.session_id !== sessionId) {
          passthrough.push(value);
          continue;
        }
        // A revision check can recover the held turn from history before its live receipt arrives.
        if (frame.action === "session.conversation.changed" && payload.check === true) continue;
        if (isConfigurationCompletion(frame.action, payload)) held.push(value);
        else passthrough.push(value);
      }
      if (passthrough.length > 0) socket.send(passthrough.join("\n"));
    });
  });

  return {
    arm: (nextSessionId) => {
      sessionId = nextSessionId;
      held.length = 0;
    },
    pendingCount: () => held.length,
    release: () => {
      const frames = held.splice(0);
      sessionId = undefined;
      deliver?.(frames);
    },
  };
}

export async function seedAndOpenChat(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  beforeNavigate?: (sessionId: string) => void,
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
  if (!task.session_id) throw new Error("expected an auto-started picker feedback session");
  await waitForAgentMessage(
    apiClient,
    task.session_id,
    "This is a simple mock response for e2e testing.",
    30_000,
  );
  await waitForSessionState(apiClient, {
    taskId: task.id,
    sessionId: task.session_id,
    expectedState: "WAITING_FOR_INPUT",
    timeout: 30_000,
    message: "initial picker feedback turn must settle before interaction",
  });
  beforeNavigate?.(task.session_id);
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return { session, sessionId: task.session_id };
}
