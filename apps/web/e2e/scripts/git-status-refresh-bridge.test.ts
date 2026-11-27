import { expect, test, vi } from "vitest";
import type { Page } from "@playwright/test";
import { routeGitStatusRefresh } from "../tests/git/git-status-refresh-helpers";

vi.mock("@playwright/test", () => ({ expect: vi.fn() }));

function socketPair() {
  const server = {
    send: vi.fn(),
    handler: (_message: string | Buffer) => {},
    onMessage(handler: (message: string | Buffer) => void) {
      this.handler = handler;
    },
  };
  const client = {
    ...server,
    send: vi.fn(),
    connectToServer: () => server,
  };
  const page = {
    routeWebSocket: async (_url: RegExp, connect: (socket: typeof client) => void) =>
      connect(client),
  } as unknown as Page;
  return { server, client, page };
}

function statusFrame(detailState: string) {
  return JSON.stringify({
    type: "notification",
    action: "session.git.event",
    payload: { type: "status_update", status: { detail_state: detailState } },
  });
}

test("holds background ready snapshots until the controlled initial refresh is released", async () => {
  const { page, server, client } = socketPair();
  const bridge = await routeGitStatusRefresh(page, { holdReadyNotifications: true });
  const first = statusFrame("ready");
  const second = statusFrame("ready");
  server.handler(`${first}\n${second}`);
  expect(client.send).not.toHaveBeenCalled();
  bridge.releaseReadyGitStatusNotifications();
  expect(client.send.mock.calls.map(([frame]) => frame)).toEqual([first, second]);
  server.handler(first);
  expect(client.send).toHaveBeenLastCalledWith(first);
});

test("forwards ready snapshots by default and still drops deliberately lost pending events", async () => {
  const { page, server, client } = socketPair();
  await routeGitStatusRefresh(page);
  const ready = statusFrame("ready");
  server.handler(`${statusFrame("pending")}\n${ready}`);
  expect(client.send).toHaveBeenCalledExactlyOnceWith(ready);
});
