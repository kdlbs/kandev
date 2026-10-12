import { act, cleanup, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setWebSocketClient } from "@/lib/ws/connection";
import type { WebSocketClient } from "@/lib/ws/client";
import type { RepositoryScript } from "@/lib/types/http";
import { useTerminals } from "./use-terminals";
import { deferred, renderSessionRead } from "./session-read-test-helpers";

afterEach(() => {
  cleanup();
  setWebSocketClient(null);
  sessionStorage.clear();
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.2
it.each(["ordinary", "script"])(
  "keeps a created %s sibling available while another terminal is being destroyed",
  async (kind) => {
    const destroy = deferred<unknown>();
    let nextId = 0;
    const request = vi.fn((action: string) => {
      if (action === "user_shell.list") return Promise.resolve({ shells: [] });
      if (action === "user_shell.destroy") return destroy.promise;
      if (action === "user_shell.create") {
        nextId++;
        return Promise.resolve({
          terminal_id: `${kind === "script" ? "script" : "shell"}-${nextId}`,
          kind,
          ...(kind === "ordinary" ? { seq: nextId, state: "open" } : {}),
          pty_status: "running",
          label: `Terminal ${nextId}`,
          closable: true,
        });
      }
      throw new Error(`Unexpected action: ${action}`);
    });
    setWebSocketClient({ request } as unknown as WebSocketClient);
    const { result } = renderSessionRead(
      () => useTerminals({ sessionId: "session", environmentId: "environment" }),
      undefined,
      (store) => store.setState((state) => ({ tasks: { ...state.tasks, activeTaskId: "task" } })),
    );
    await waitFor(() =>
      expect(result.current.store.getState().userShells.loaded.environment).toBe(true),
    );
    for (let index = 0; index < 2; index++) {
      await act(async () => {
        if (kind === "ordinary") await result.current.value.addTerminal();
        else
          await result.current.value.handleRunCommand({
            id: "script",
            name: "Build",
          } as RepositoryScript);
      });
    }
    const [sibling, target] = result.current.value.terminals;
    expect(result.current.value.terminals).toHaveLength(2);
    expect(
      result.current.store
        .getState()
        .userShells.byEnvironmentId.environment.map((s) => s.terminalId),
    ).toEqual([sibling.id, target.id]);
    let closing!: Promise<boolean>;
    act(() => {
      closing = result.current.value.destroyTerminal(target.id);
    });
    expect(result.current.value.terminals.map((terminal) => terminal.id)).toEqual([sibling.id]);
    expect(result.current.store.getState().userShells.byEnvironmentId.environment).toEqual([
      expect.objectContaining({ terminalId: sibling.id, kind, running: true }),
    ]);
    expect(request.mock.calls.filter(([action]) => action === "user_shell.list")).toHaveLength(1);
    await act(async () => {
      destroy.resolve({});
      await expect(closing).resolves.toBe(true);
    });
  },
);
