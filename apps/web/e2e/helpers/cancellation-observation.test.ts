import { expect, it, vi } from "vitest";
import type { Page, WebSocketRoute } from "@playwright/test";
import { holdCancellationSettlement } from "./cancellation-observation";

it("holds settlement frames that arrive before the cancellation-pending notification", async () => {
  let receive!: (message: string) => void;
  const send = vi.fn();
  const client = {
    send,
    connectToServer: () => ({
      onMessage: (handler: typeof receive) => {
        receive = handler;
      },
    }),
  };
  const page = {
    routeWebSocket: async (_pattern: string, handler: (route: WebSocketRoute) => void) => {
      handler(client as unknown as WebSocketRoute);
    },
  };
  const cancellation = await holdCancellationSettlement(page as unknown as Page);
  const frame = (pending: boolean) =>
    JSON.stringify({
      action: "session.cancellation_changed",
      payload: {
        session_id: "session-1",
        cancellation_pending: pending,
        cancellation_revision: pending ? 1 : 2,
      },
    });
  const settled = frame(false);
  const pending = frame(true);

  cancellation.arm("session-1");
  receive(settled);
  receive(pending);
  expect(send.mock.calls).toEqual([[pending]]);

  cancellation.release();
  expect(send.mock.calls).toEqual([[pending], [settled]]);
  receive(settled);
  expect(send).toHaveBeenCalledTimes(3);
});
