import { expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import { routeGatewayNotifications } from "./ws-capture";

it("isolates a controlled session snapshot while forwarding unrelated gateway frames", async () => {
  let onServerMessage!: (message: string | Buffer) => void;
  const send = vi.fn();
  const socket = {
    send,
    onMessage: vi.fn(),
    connectToServer: () => ({
      send: vi.fn(),
      onMessage: (handler: typeof onServerMessage) => {
        onServerMessage = handler;
      },
    }),
  };
  const page = {
    routeWebSocket: async (_pattern: RegExp, handler: (socket: unknown) => void) => {
      handler(socket);
    },
  } as unknown as Page;
  const route = await routeGatewayNotifications(
    page,
    (frame) =>
      frame.type === "notification" &&
      frame.payload?.session_id === "controlled" &&
      frame.action === "session.available_commands",
  );
  const owned = JSON.stringify({
    type: "notification",
    action: "session.available_commands",
    payload: { session_id: "controlled", commands: ["late-real-provider"] },
  });
  const other = JSON.stringify({
    type: "notification",
    action: "session.available_commands",
    payload: { session_id: "other", commands: ["real-provider"] },
  });
  onServerMessage(`${owned}\n${other}`);
  expect(send.mock.calls).toEqual([[other]]);
  send.mockClear();
  const binary = Buffer.from([1, 2]);
  onServerMessage(binary);
  onServerMessage("non-json frame");
  expect(send.mock.calls).toEqual([[binary], ["non-json frame"]]);
  send.mockClear();
  route.send("session.available_commands", {
    session_id: "controlled",
    commands: ["synthetic-plan"],
  });
  expect(JSON.parse(send.mock.calls[0][0]).payload.commands).toEqual(["synthetic-plan"]);
});
