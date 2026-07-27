import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { AgentUpdatePreview } from "@/lib/api";
import { useAgentUpdateDialogState } from "./use-agent-update-dialog-state";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((complete) => {
    resolve = complete;
  });
  return { promise, resolve };
}

const FIRST_PREVIEW: AgentUpdatePreview = {
  agent_name: "claude-acp",
  package: "@agentclientprotocol/claude-agent-acp",
  current_version: "0.62.0",
  target_version: "0.63.0",
  command: ["npm", "exec"],
  command_string: "npm exec",
};

describe("useAgentUpdateDialogState", () => {
  it("ignores a preview that resolves after close and reopen", async () => {
    const firstRequest = deferred<AgentUpdatePreview>();
    const secondRequest = deferred<AgentUpdatePreview>();
    const onPreview = vi
      .fn<(agentName: string) => Promise<AgentUpdatePreview>>()
      .mockReturnValueOnce(firstRequest.promise)
      .mockReturnValueOnce(secondRequest.promise);
    const { result } = renderHook(() =>
      useAgentUpdateDialogState({
        agentName: "claude-acp",
        onPreview,
        onUpdate: vi.fn(),
      }),
    );

    act(() => result.current.handleOpenChange(true));
    await waitFor(() => expect(onPreview).toHaveBeenCalledTimes(1));
    act(() => result.current.handleOpenChange(false));
    act(() => result.current.handleOpenChange(true));
    await waitFor(() => expect(onPreview).toHaveBeenCalledTimes(2));

    await act(async () => {
      firstRequest.resolve(FIRST_PREVIEW);
      await firstRequest.promise;
    });

    expect(result.current.preview).toBeNull();
    expect(result.current.loading).toBe(true);

    await act(async () => {
      secondRequest.resolve({ ...FIRST_PREVIEW, target_version: "0.64.0" });
      await secondRequest.promise;
    });

    await waitFor(() => expect(result.current.preview?.target_version).toBe("0.64.0"));
  });
});
