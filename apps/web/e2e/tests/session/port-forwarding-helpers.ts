import type { Page } from "@playwright/test";

type PortAction = "port.list" | "port.tunnel.list" | "port.tunnel.start" | "port.tunnel.stop";
type Request = {
  id: string;
  type: string;
  action: PortAction;
  payload: { session_id: string; port: number };
};

export async function routePortForwarding(page: Page) {
  let sessionId = "";
  const tunnels = new Map([[9000, 49152]]);
  let failure: PortAction | undefined;
  let hold: PortAction | undefined;
  let release: (() => void) | undefined;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    server.onMessage((message) => socket.send(message));
    socket.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part) as Request;
        if (
          frame.type !== "request" ||
          frame.payload?.session_id !== sessionId ||
          !/^port\.(list|tunnel\.(list|start|stop))$/.test(frame.action)
        ) {
          server.send(part);
          continue;
        }
        if (failure === frame.action) {
          failure = undefined;
          socket.send(
            JSON.stringify({
              type: "error",
              id: frame.id,
              action: frame.action,
              payload: { code: "INTERNAL_ERROR", message: "Port operation failed" },
            }),
          );
          continue;
        }
        const payload = portResponse(frame, tunnels);
        const respond = () =>
          socket.send(
            JSON.stringify({ type: "response", id: frame.id, action: frame.action, payload }),
          );
        if (hold === frame.action) {
          hold = undefined;
          release = respond;
        } else respond();
      }
    });
  });
  return {
    setSession: (id: string) => {
      sessionId = id;
    },
    failNext: (action: PortAction) => {
      failure = action;
    },
    holdNext: (action: PortAction) => {
      hold = action;
      release = undefined;
    },
    held: () => Boolean(release),
    release: () => {
      release?.();
      release = undefined;
    },
  };
}

function portResponse(frame: Request, tunnels: Map<number, number>) {
  switch (frame.action) {
    case "port.list":
      return {
        ports: [
          {
            port: 3000,
            address: "*",
            process: "a-very-long-service-process-name-for-narrow-layouts",
          },
          { port: 9000, address: "127.0.0.1", process: "node" },
        ],
      };
    case "port.tunnel.list":
      return { tunnels: [...tunnels].map(([port, tunnel_port]) => ({ port, tunnel_port })) };
    case "port.tunnel.start":
      tunnels.set(frame.payload.port, 49153);
      return { tunnel_port: 49153 };
    case "port.tunnel.stop":
      tunnels.delete(frame.payload.port);
      return {};
  }
}
