import { EventEmitter } from "node:events";
import type { Page } from "@playwright/test";
import { describe, expect, it } from "vitest";
import { attachGatewayTrafficCapture } from "./ws-traffic";

describe("gateway session activation traffic", () => {
  it("distinguishes recovery focus signals from explicit session actions", () => {
    const page = new EventEmitter();
    const socket = Object.assign(new EventEmitter(), {
      url: () => "ws://localhost:8080/ws",
    });
    const { frames } = attachGatewayTrafficCapture(page as unknown as Page);
    page.emit("websocket", socket);

    for (const activationSource of [undefined, "session_focus", "session_open"]) {
      socket.emit("framesent", {
        payload: JSON.stringify({
          type: "request",
          action: "session.launch",
          payload: {
            session_id: "session-1",
            intent: "resume",
            activation_source: activationSource,
          },
        }),
      });
    }

    expect(frames.map((frame) => frame.activationSource)).toEqual([
      undefined,
      "session_focus",
      "session_open",
    ]);
    expect(frames.map((frame) => frame.sessionId)).toEqual(["session-1", "session-1", "session-1"]);
  });
});
