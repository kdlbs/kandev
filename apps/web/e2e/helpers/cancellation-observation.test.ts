import { expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import { holdCancellationSettlement } from "./cancellation-observation";

async function fixture() {
  let receive!: (message: string | Buffer) => void;
  const send = vi.fn();
  const socket = {
    send,
    connectToServer: () => ({
      onMessage: (handler: typeof receive) => {
        receive = handler;
      },
    }),
  };
  const page = {
    routeWebSocket: async (_pattern: unknown, handler: (socket: unknown) => void) =>
      handler(socket),
  } as unknown as Page;
  const observation = await holdCancellationSettlement(page);
  return { send, observation, receive: (message: string | Buffer) => receive(message) };
}
const notification = (pending: boolean, sessionId = "target") =>
  JSON.stringify({
    action: "session.cancellation_changed",
    payload: { session_id: sessionId, cancellation_pending: pending },
  });

it("buffers settlement that overtakes the native pending notification", async () => {
  const { send, receive, observation } = await fixture();
  observation.arm("target");
  receive(notification(false));
  expect(send).not.toHaveBeenCalled();
  receive(notification(true));
  expect(send.mock.calls).toEqual([[notification(true)]]);
  observation.release();
  expect(send.mock.calls).toEqual([[notification(true)], [notification(false)]]);
});

it("observes pending inside a batched gateway frame and releases remaining frames once", async () => {
  const { send, receive, observation } = await fixture();
  observation.arm("target");
  const other = notification(false, "other");
  receive(`${other}\n${notification(true)}\n${notification(false)}`);
  expect(send.mock.calls).toEqual([[notification(true)]]);
  observation.release();
  observation.release();
  expect(send.mock.calls).toEqual([[notification(true)], [other], [notification(false)]]);
});

it("forwards original traffic outside the armed observation", async () => {
  const { send, receive, observation } = await fixture();
  const binary = Buffer.from([1, 2]);
  receive(binary);
  observation.arm("target");
  receive("control frame");
  expect(send.mock.calls).toEqual([[binary]]);
  observation.release();
  receive(notification(false));
  expect(send.mock.calls).toEqual([[binary], ["control frame"], [notification(false)]]);
});
