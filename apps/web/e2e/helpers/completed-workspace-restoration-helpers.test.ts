import type { Page, WebSocketRoute } from "@playwright/test";
import { expect, it, vi } from "vitest";
import { failWorkspaceRestoresUntilReleased } from "../tests/session/completed-workspace-restoration-helpers";

vi.mock("@playwright/test", () => ({ expect: vi.fn() }));

const restoreRequest = (id: string) =>
  JSON.stringify({
    id,
    type: "request",
    action: "session.launch",
    payload: { intent: "restore_workspace", task_id: "task", session_id: "session" },
  });
const readiness = (sessionId = "session") =>
  JSON.stringify({
    type: "event",
    action: "session.agentctl_ready",
    payload: { session_id: sessionId, task_environment_id: "environment" },
  });
const response = (id: string, success: boolean) =>
  JSON.stringify({ id, type: "response", action: "session.launch", payload: { success } });

async function routedFailure() {
  let clientMessage!: (message: string) => void;
  let serverMessage!: (message: string) => void;
  const clientSend = vi.fn();
  const serverSend = vi.fn();
  const socket = {
    send: clientSend,
    onMessage: (handler: typeof clientMessage) => {
      clientMessage = handler;
    },
    connectToServer: () => ({
      send: serverSend,
      onMessage: (handler: typeof serverMessage) => {
        serverMessage = handler;
      },
    }),
  };
  const page = {
    routeWebSocket: async (_url: RegExp, handler: (socket: WebSocketRoute) => void) => {
      handler(socket as unknown as WebSocketRoute);
    },
  } as unknown as Page;
  const failure = await failWorkspaceRestoresUntilReleased(page, "task", "session");
  return { failure, clientSend, serverSend, clientMessage, serverMessage };
}

it("holds stale readiness until the admitted retry succeeds, including between release and click", async () => {
  const route = await routedFailure();
  route.clientMessage(restoreRequest("automatic"));
  expect(route.failure.wasConsumed()).toBe(true);
  expect(route.serverSend).not.toHaveBeenCalled();
  expect(JSON.parse(route.clientSend.mock.lastCall![0])).toMatchObject({
    id: "automatic",
    type: "error",
  });
  route.clientSend.mockClear();
  route.serverMessage(readiness());
  route.failure.allowNextRestores();
  route.serverMessage(`${readiness()}\n${readiness("other-session")}`);
  expect(route.clientSend.mock.calls).toEqual([[readiness("other-session")]]);

  route.clientMessage(restoreRequest("manual"));
  expect(route.serverSend).toHaveBeenCalledWith(restoreRequest("manual"));
  route.serverMessage(readiness());
  expect(route.clientSend).toHaveBeenCalledTimes(1);
  route.serverMessage(response("unrelated", true));
  expect(route.clientSend).toHaveBeenLastCalledWith(response("unrelated", true));
  route.serverMessage(response("manual", true));
  expect(route.clientSend).toHaveBeenLastCalledWith(
    [response("manual", true), readiness(), readiness(), readiness()].join("\n"),
  );
  route.serverMessage(readiness());
  expect(route.clientSend).toHaveBeenLastCalledWith(readiness());
});

it.each(["error", "unsuccessful"])("keeps readiness held after a %s retry", async (kind) => {
  const route = await routedFailure();
  route.failure.allowNextRestores();
  route.clientMessage(restoreRequest("first"));
  route.serverMessage(readiness());
  const failed =
    kind === "error"
      ? JSON.stringify({ id: "first", type: "error", action: "session.launch" })
      : response("first", false);
  route.serverMessage(failed);
  expect(route.clientSend.mock.calls).toEqual([[failed]]);
  route.serverMessage(readiness());
  expect(route.clientSend).toHaveBeenCalledTimes(1);
  route.clientMessage(restoreRequest("second"));
  route.serverMessage(response("second", true));
  expect(route.clientSend).toHaveBeenLastCalledWith(
    [response("second", true), readiness(), readiness()].join("\n"),
  );
});
