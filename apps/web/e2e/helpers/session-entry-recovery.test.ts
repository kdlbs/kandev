import type { Page } from "@playwright/test";
import { describe, expect, it, vi } from "vitest";
import { routeSessionEntryRecovery } from "./session-entry-recovery";

vi.mock("./causal-waits", () => ({ injectLatency: vi.fn() }));

function createGateway() {
  let fromClient!: (message: string) => void;
  let fromServer!: (message: string) => void;
  const server = {
    send: vi.fn(),
    onMessage: (handler: typeof fromServer) => {
      fromServer = handler;
    },
  };
  const route = {
    connectToServer: () => server,
    send: vi.fn(),
    onMessage: (handler: typeof fromClient) => {
      fromClient = handler;
    },
  };
  const page = {
    routeWebSocket: async (_pattern: RegExp, handler: (socket: typeof route) => void) =>
      handler(route),
  } as unknown as Page;
  return {
    page,
    route,
    server,
    request: (id: string, sessionId: string) =>
      fromClient(
        JSON.stringify({
          type: "request",
          id,
          action: "session.ensure",
          payload: { session_id: sessionId },
        }),
      ),
    response: (id: string) =>
      fromServer(
        JSON.stringify({
          type: "response",
          id,
          action: "session.ensure",
          payload: { success: true },
        }),
      ),
  };
}

describe("session entry rejection lifetime", () => {
  it("keeps in-flight requests rejected after release and allows later requests", async () => {
    const gateway = createGateway();
    const proxy = await routeSessionEntryRecovery(gateway.page);
    proxy.rejectResponsesUntilReleased("session.ensure", "simulated failure", {
      sessionId: "target",
    });
    gateway.request("pending", "target");
    gateway.request("other", "sibling");
    proxy.releaseRejectedResponses("session.ensure");
    gateway.response("pending");
    gateway.response("other");
    gateway.request("later", "target");
    gateway.response("later");

    const frames = gateway.route.send.mock.calls.map(([frame]) => JSON.parse(frame));
    expect(frames).toEqual([
      expect.objectContaining({
        id: "pending",
        type: "error",
        payload: { code: "E2E_SIMULATED_ERROR", message: "simulated failure" },
      }),
      expect.objectContaining({ id: "other", type: "response" }),
      expect.objectContaining({ id: "later", type: "response" }),
    ]);
    expect(proxy.rejectedResponseCount("session.ensure")).toBe(1);
    expect(gateway.server.send).toHaveBeenCalledTimes(3);
  });
});
