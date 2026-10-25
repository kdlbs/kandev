import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type {
  PluginManagedConversationController,
  PluginManagedConversationSnapshot,
} from "@kandev/plugin-sdk";
import { useManagedConversationChat } from "./use-managed-conversation-chat";

const ready: PluginManagedConversationSnapshot = {
  workspaceId: "workspace-1",
  instanceKey: "delivery-lead",
  taskId: "task-1",
  sessionId: "session-1",
  revision: 1,
  sessionResourceVersion: "session-v1",
  state: "ready",
  pendingInteractions: [],
};

const paused: PluginManagedConversationSnapshot = {
  ...ready,
  revision: 2,
  state: "paused",
};

describe("useManagedConversationChat", () => {
  it("ignores a status read that began before a pause action completed", async () => {
    let resolveOldRead: (value: PluginManagedConversationSnapshot) => void = () => undefined;
    const oldRead = new Promise<PluginManagedConversationSnapshot>((resolve) => {
      resolveOldRead = resolve;
    });
    const getStatus = vi
      .fn()
      .mockImplementationOnce(() => oldRead)
      .mockResolvedValue(paused);
    const controller = {
      getStatus,
      listInputs: vi.fn().mockResolvedValue({ inputs: [], nextSequenceCursor: 0, hasMore: false }),
      setPaused: vi.fn().mockResolvedValue(paused),
    } as unknown as PluginManagedConversationController;

    const { result } = renderHook(() => useManagedConversationChat(ready, controller));
    await waitFor(() => expect(getStatus).toHaveBeenCalledOnce());

    act(() => result.current.togglePaused());
    await waitFor(() => expect(result.current.status.state).toBe("paused"));
    await act(async () => resolveOldRead(ready));

    expect(result.current.status.state).toBe("paused");
    expect(controller.setPaused).toHaveBeenCalledWith(
      expect.objectContaining({
        expectedConversationRevision: 1,
        paused: true,
      }),
    );
  });
});
