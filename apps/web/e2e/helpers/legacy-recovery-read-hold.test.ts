import { expect, it, vi } from "vitest";
import type { Page, WebSocketRoute } from "@playwright/test";
import { holdLegacyRecoveryReads } from "./legacy-recovery-read-hold";

type Message = string | Buffer;

async function setup() {
  let connect!: (socket: WebSocketRoute) => void;
  const page = {
    routeWebSocket: vi.fn(async (_url: RegExp, handler: typeof connect) => {
      connect = handler;
    }),
  } as unknown as Page;
  await holdLegacyRecoveryReads(page, "target");
  function connection() {
    let fromClient!: (message: Message) => void;
    let fromServer!: (message: Message) => void;
    const toServer = vi.fn();
    const toClient = vi.fn();
    connect({
      connectToServer: () => ({
        send: toServer,
        onMessage: (handler: typeof fromServer) => {
          fromServer = handler;
        },
      }),
      send: toClient,
      onMessage: (handler: typeof fromClient) => {
        fromClient = handler;
      },
    } as unknown as WebSocketRoute);
    return { fromClient, fromServer, toServer, toClient };
  }
  return connection;
}

function frame(id: string, action: string, sessionId = "target", intent?: string) {
  return JSON.stringify({
    id,
    type: "request",
    action,
    payload: { session_id: sessionId, intent },
  });
}

it.each(["response", "error"])(
  "releases real reads only after the correlated recovery %s",
  async (type) => {
    const connect = await setup();
    const socket = connect();
    const commits = frame("commits", "session.git.commits");
    const diff = frame("diff", "session.cumulative_diff");
    const status = frame("status", "task.session.status");
    const otherSession = frame("other", "session.git.commits", "other");
    const restore = frame("restore", "session.launch", "target", "restore_workspace");
    socket.fromClient([commits, status, diff, otherSession, restore].join("\n"));
    expect(socket.toServer.mock.calls.map(([message]) => message)).toEqual([
      status,
      otherSession,
      restore,
    ]);
    socket.fromServer(JSON.stringify({ id: "status", type: "response" }));
    socket.fromServer(JSON.stringify({ id: "restore", type: "notification" }));
    expect(socket.toServer).toHaveBeenCalledTimes(3);
    const response = JSON.stringify({
      id: "restore",
      type,
      payload: { details: { kind: "managed_clone_relocation_required" } },
    });
    socket.fromServer(response);
    expect(socket.toClient).toHaveBeenLastCalledWith(response);
    expect(socket.toServer.mock.calls.map(([message]) => message)).toEqual([
      status,
      otherSession,
      restore,
      commits,
      diff,
    ]);
    socket.fromServer(response);
    expect(socket.toServer).toHaveBeenCalledTimes(5);
    const reconnected = connect();
    reconnected.fromClient(diff);
    expect(reconnected.toServer).toHaveBeenCalledWith(diff);
  },
);

it("passes binary and unknown frames and keeps automatic resume from releasing the hold", async () => {
  const socket = (await setup())();
  const binary = Buffer.from([1, 2]);
  const resume = frame("automatic", "session.launch", "target", "resume");
  socket.fromClient(binary);
  socket.fromClient("not-json\nnull");
  socket.fromClient(resume);
  socket.fromClient(frame("diff", "session.cumulative_diff"));
  socket.fromServer(JSON.stringify({ id: "automatic", type: "response" }));
  expect(socket.toServer.mock.calls.map(([message]) => message)).toEqual([
    binary,
    "not-json",
    "null",
    resume,
  ]);
  const manual = frame("manual", "session.recover");
  socket.fromClient(manual);
  socket.fromServer(JSON.stringify({ id: "manual", type: "error" }));
  expect(socket.toServer).toHaveBeenLastCalledWith(frame("diff", "session.cumulative_diff"));
});
