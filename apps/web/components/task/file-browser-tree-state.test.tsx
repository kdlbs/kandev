import { act, cleanup } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import { renderSessionRead } from "@/hooks/domains/session/session-read-test-helpers";
import { useFileTreeCacheBinding } from "./file-browser-tree-state";

afterEach(() => {
  cleanup();
  setWebSocketClient(null);
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5 AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
it("retires a phone session-keyed tree when its canonical environment begins restoration", () => {
  setWebSocketClient({ request: vi.fn() } as unknown as WebSocketClient);
  const { result } = renderSessionRead(() => useFileTreeCacheBinding("session", "session:1:0"), {});
  const original = result.current.value;
  expect(original.isCurrent()).toBe(true);
  act(() => {
    result.current.store.getState().beginWorkspaceRestoration("task", "session", "environment");
  });
  expect(original.isCurrent()).toBe(false);
  expect(result.current.value.isCurrent()).toBe(false);
  expect(result.current.value.key).not.toBe(original.key);
});
