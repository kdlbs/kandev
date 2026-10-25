import { expect, it, vi } from "vitest";

import { NoopWebSocket } from "./noop-websocket";

it("restores the inert WebSocket after global stubs are cleared", () => {
  expect(globalThis.WebSocket).toBe(NoopWebSocket);

  vi.stubGlobal("WebSocket", class {});
  vi.unstubAllGlobals();

  expect(globalThis.WebSocket).toBe(NoopWebSocket);
  expect(window.WebSocket).toBe(NoopWebSocket);
});
